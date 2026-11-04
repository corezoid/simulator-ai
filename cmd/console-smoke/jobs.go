package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"
)

type task struct {
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	DownloadURL string `json:"downloadUrl"`
	Details     struct {
		// An import's root graph; details.actor is only an actor dataReplace named.
		Graph struct {
			ID string `json:"id"`
		} `json:"graph"`
		File struct {
			FileName string `json:"fileName"`
		} `json:"file"`
	} `json:"details"`
	Error any `json:"error"`
}

// jobs (console-full): a layer exported into a .graph archive, the archive downloaded by the task's
// downloadUrl, uploaded again with graph_file_upload and imported as a copy.
func jobs(c *client, stamp string) error {
	var layer id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke export " + stamp}, &layer); err != nil {
		return err
	}
	var export task
	if err := c.tool("graph_export_create", map[string]any{"title": "AI smoke " + stamp, "actors": []string{layer.ID}, "ops": map[string]any{"attachments": false}}, &export); err != nil {
		return err
	}
	done, err := waitTask(c, export.ID)
	if err != nil {
		return err
	}
	if done.DownloadURL == "" || done.Details.File.FileName == "" {
		return fmt.Errorf("export %d completed without an archive: %+v", export.ID, done)
	}
	archive, err := download(done.DownloadURL)
	if err != nil {
		return err
	}
	var uploaded struct {
		FileName string `json:"fileName"`
	}
	if err = c.tool("graph_file_upload", map[string]any{"fileName": "smoke-" + stamp + ".graph", "content": base64.StdEncoding.EncodeToString(archive)}, &uploaded); err != nil {
		return err
	}
	var imp task
	if err = c.tool("graph_import_create", map[string]any{"fileName": uploaded.FileName, "title": "AI smoke " + stamp,
		"ops": map[string]any{"actorRefStrategy": "replace", "formRefStrategy": "reuse"}}, &imp); err != nil {
		return err
	}
	if done, err = waitTask(c, imp.ID); err != nil {
		return err
	}
	if done.Details.Graph.ID == "" {
		return fmt.Errorf("import %d completed without its root graph: %+v", imp.ID, done)
	}
	fmt.Println("OK jobs export", export.ID, "import", imp.ID, "root", done.Details.Graph.ID)
	return nil
}

// waitTask follows a task until it ends, at most two minutes, and wants it completed.
func waitTask(c *client, taskID int64) (task, error) {
	var t task
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		if err := c.tool("task_get", map[string]any{"taskId": taskID}, &t); err != nil {
			return t, err
		}
		switch t.Status {
		case "completed":
			return t, nil
		case "failed", "canceled":
			return t, fmt.Errorf("task %d ended %s: %v", taskID, t.Status, t.Error)
		}
	}
	return t, fmt.Errorf("task %d is still %s after two minutes (is the control-tasks worker running?)", taskID, t.Status)
}

// maxArchive bounds a downloaded archive; graph_file_upload takes far less.
const maxArchive = 8 << 20

// download reads an archive by its link, retrying while the antivirus still scans it (425).
func download(link string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	for attempt := 0; attempt < 10; attempt++ {
		resp, err := client.Get(link)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxArchive+1))
		resp.Body.Close()
		switch {
		case err != nil:
			return nil, err
		case len(body) > maxArchive:
			return nil, fmt.Errorf("the archive is larger than %d bytes", maxArchive)
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode != http.StatusTooEarly && resp.StatusCode != http.StatusNotFound:
			return nil, fmt.Errorf("download of the archive answered %d", resp.StatusCode)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("the archive was not served within 20 seconds")
}
