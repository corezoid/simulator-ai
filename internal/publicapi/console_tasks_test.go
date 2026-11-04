package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// task/get answers as it is; a task holding an archive gets its download link added.
func TestGetTaskAddsDownloadURL(t *testing.T) {
	reply := ""
	var sent map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string         `json:"id"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		sent = q.Params
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, reply)
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL + "/", Workspace: "ws", Profile: "console-full", Authorization: "Simulator run"})
	for _, tc := range []struct{ result, want string }{
		{`{"id":3,"status":"started"}`, `{"id":3,"status":"started"}`},
		{`{"id":3,"status":"completed","details":{"file":{"fileName":"2026/11/8c41 e0.graph"}}}`, `"downloadUrl":"` + api.URL + `/api/1.0/download/2026/11/8c41%20e0.graph"`},
		{`{"id":4,"name":"zip","status":"completed","details":{"fileName":"z1.zip","size":9}}`, `"downloadUrl":"` + api.URL + `/api/1.0/download/z1.zip"`},
	} {
		reply = tc.result
		got, err := c.GetTask(context.Background(), TaskGetInput{TaskID: 3})
		if err != nil || !strings.Contains(string(got), tc.want) || sent["taskId"] != float64(3) || sent["accId"] != "ws" {
			t.Errorf("%s: %s %v (sent %v)", tc.result, got, err, sent)
		}
	}
	if _, err := c.GetTask(context.Background(), TaskGetInput{}); err == nil {
		t.Error("taskId 0 accepted")
	}
}

// A .graph archive is uploaded attached to nothing as application/octet-stream.
func TestUploadGraphFile(t *testing.T) {
	var kind, actor string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		_, h, _ := r.FormFile("file")
		kind, actor = h.Header.Get("Content-Type")+" "+h.Filename, r.FormValue("actorId")
		fmt.Fprint(w, `{"id":9,"title":"import.graph","type":"application/octet-stream","size":3,"fileName":"k9.graph"}`)
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator run"})
	f, err := c.UploadGraphFile(context.Background(), GraphFileInput{Content: "AQID"})
	if err != nil || f != (GraphFile{FileName: "k9.graph", Title: "import.graph", Size: 3}) || kind != "application/octet-stream import.graph" || actor != "" {
		t.Fatal(f, err, kind, actor)
	}
	if _, err = c.UploadGraphFile(context.Background(), GraphFileInput{FileName: "notes.zip", Content: "AQID"}); err == nil {
		t.Fatal("a non-.graph file was accepted")
	}
}
