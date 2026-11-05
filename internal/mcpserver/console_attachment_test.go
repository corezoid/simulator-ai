package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// attachment_read hands the model the file itself: an image as an image block, a PDF as an
// embedded resource, each after the structured answer and a note; a text file stays text only.
func TestAttachmentReadSendsContentBlocks(t *testing.T) {
	files := map[string][2]string{
		"pic":  {"image/png", "\x89PNG\r\n\x1a\nfake"},
		"doc":  {"application/pdf", "%PDF-1.4 no text layer"},
		"note": {"text/plain", "hello"},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := files[strings.TrimPrefix(r.URL.Path, "/api/1.0/download/")]
		w.Header().Set("Content-Type", f[0])
		fmt.Fprint(w, f[1])
	}))
	defer api.Close()
	c, err := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator user-jwt"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	read := func(name string) []map[string]any {
		raw, _ := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"attachment_read","arguments":{"fileName":"`+name+`"}}}`)))
		var reply struct {
			Result struct {
				Content []map[string]any `json:"content"`
				IsError bool             `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.Result.IsError {
			t.Fatal(name, string(raw))
		}
		return reply.Result.Content
	}
	pic := read("pic")
	if len(pic) != 3 || pic[0]["type"] != "text" || !strings.Contains(pic[1]["text"].(string), "follows as a content block") ||
		pic[2]["type"] != "image" || pic[2]["mimeType"] != "image/png" || pic[2]["data"] != base64.StdEncoding.EncodeToString([]byte(files["pic"][1])) {
		t.Fatalf("image: %v", pic)
	}
	doc := read("doc")
	resource, _ := doc[len(doc)-1]["resource"].(map[string]any)
	if doc[len(doc)-1]["type"] != "resource" || resource["mimeType"] != "application/pdf" || resource["uri"] != "simulator://attachment/doc" || resource["blob"] != base64.StdEncoding.EncodeToString([]byte(files["doc"][1])) {
		t.Fatalf("pdf: %v", doc)
	}
	if note := read("note"); len(note) != 1 || !strings.Contains(note[0]["text"].(string), `"text":"hello"`) {
		t.Fatalf("text: %v", note)
	}
}
