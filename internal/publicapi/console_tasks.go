package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strings"
)

// Graph export and import run as background tasks of the workspace (legacy exportGraph,
// importGraph, getTaskStatus, uploadGraphFile): a create returns the task at once and task/get
// follows it until it ends. An archive moves over the upload and download routes.

type TaskGetInput struct {
	TaskID int64    `json:"taskId"`
	With   []string `json:"with,omitempty"`
}

type taskWire struct {
	Details struct {
		File struct {
			FileName string `json:"fileName"`
		} `json:"file"`
		FileName string `json:"fileName"`
	} `json:"details"`
}

// GetTask reads one task as task/get answers it and, when it holds an archive (an export's
// details.file, a zip's details.fileName), adds downloadUrl: the download route of that key, which
// serves it without a session.
func (c *Client) GetTask(ctx context.Context, in TaskGetInput) (json.RawMessage, error) {
	if e := consoleOnly(c); e != nil {
		return nil, e
	}
	if in.TaskID < 1 {
		return nil, errors.New("positive taskId required")
	}
	args := map[string]json.RawMessage{}
	args["taskId"], _ = json.Marshal(in.TaskID)
	if len(in.With) > 0 {
		args["with"], _ = json.Marshal(in.With)
	}
	raw, err := c.Pass(ctx, "task/get", args)
	if err != nil {
		return nil, err
	}
	var t taskWire
	_ = json.Unmarshal(raw, &t)
	key := t.Details.File.FileName
	if key == "" {
		key = t.Details.FileName
	}
	if key == "" {
		return raw, nil
	}
	doc := rawObject(raw)
	setJSON(doc, "downloadUrl", strings.TrimRight(c.config.Origin, "/")+"/api/1.0/download/"+downloadPath(key))
	return json.Marshal(doc)
}

// downloadPath escapes each segment of a storage key, keeping its slashes (legacy encodeDownloadPath).
func downloadPath(key string) string {
	segments := strings.Split(key, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// GraphFileInput is legacy uploadGraphFile: a .graph archive's bytes (base64).
type GraphFileInput struct {
	FileName string `json:"fileName,omitempty"`
	Content  string `json:"content"`
}

// GraphFile is the stored archive: fileName is what graph_import_create takes.
type GraphFile struct {
	FileName string `json:"fileName"`
	Title    string `json:"title"`
	Size     int64  `json:"size"`
}

// UploadGraphFile stores a .graph archive, attached to nothing, as application/octet-stream (as the
// web app and legacy send it, so it passes the stand's type allow-list).
func (c *Client) UploadGraphFile(ctx context.Context, in GraphFileInput) (GraphFile, error) {
	if e := consoleOnly(c); e != nil {
		return GraphFile{}, e
	}
	name := orDefault(strings.TrimSpace(in.FileName), "import.graph")
	if strings.ToLower(path.Ext(name)) != ".graph" {
		return GraphFile{}, errors.New("fileName must end in .graph")
	}
	f, err := c.UploadAttachment(ctx, AttachmentUploadInput{FileName: name, Content: in.Content, ContentType: "application/octet-stream"})
	if err != nil {
		return GraphFile{}, err
	}
	return GraphFile{FileName: f.FileName, Title: f.Title, Size: f.Size}, nil
}
