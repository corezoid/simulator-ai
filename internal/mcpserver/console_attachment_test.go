package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
)

// attachment_read hands the model what Hermes can pass on: an image as an image block after the
// structured answer and a note; a PDF as its text, never a PDF block; a text file as text only.
func TestAttachmentReadSendsContentBlocks(t *testing.T) {
	var page bytes.Buffer
	if err := jpeg.Encode(&page, image.NewGray(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	files := map[string][2]string{
		"pic":  {"image/png", "\x89PNG\r\n\x1a\nfake"},
		"doc":  {"application/pdf", "%PDF-1.4 no text layer, no pages"},
		"scan": {"application/pdf", fmt.Sprintf("%%PDF-1.4\n2 0 obj\n<< /Subtype /Image /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream\nendobj\n", page.Len(), page.String())},
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
	if len(pic) != 3 || pic[0]["type"] != "text" || !strings.Contains(pic[1]["text"].(string), "follows as an image block") ||
		pic[2]["type"] != "image" || pic[2]["mimeType"] != "image/png" || pic[2]["data"] != base64.StdEncoding.EncodeToString([]byte(files["pic"][1])) {
		t.Fatalf("image: %v", pic)
	}
	doc := read("doc")
	if len(doc) != 2 || doc[1]["type"] != "text" || !strings.Contains(doc[1]["text"].(string), "not available") {
		t.Fatalf("a scan without JPEG pages: %v", doc)
	}
	for _, block := range doc {
		if block["type"] == "resource" {
			t.Fatal("a PDF block Hermes would not pass on:", block)
		}
	}
	if scan := read("scan"); len(scan) != 3 || scan[2]["type"] != "image" || scan[2]["mimeType"] != "image/jpeg" || scan[2]["data"] != base64.StdEncoding.EncodeToString(page.Bytes()) {
		t.Fatalf("a scan's JPEG page: %v", scan)
	}
	if note := read("note"); len(note) != 1 || !strings.Contains(note[0]["text"].(string), `"text":"hello"`) {
		t.Fatalf("text: %v", note)
	}
}

// attachment_read is registered once, with the read schema and the description that names the
// request's attachments.
func TestAttachmentReadRegisteredOnce(t *testing.T) {
	for _, profile := range []string{"console", "console-full"} {
		c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: profile, Authorization: "Bearer user"})
		if err != nil {
			t.Fatal(err)
		}
		tool := New(c).GetTool("attachment_read").Tool
		schema := string(tool.RawInputSchema)
		if !strings.Contains(tool.Description, "Attachments of the request") || strings.Contains(tool.Description, "embedded") || strings.Contains(schema, `"content"`) || !strings.Contains(schema, `"required":["fileName"]`) {
			t.Fatalf("%s: %s / %s", profile, tool.Description, schema)
		}
	}
}

// A tool that panics answers an error and the server lives on (server.WithRecovery).
func TestToolPanicIsAnError(t *testing.T) {
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	s.AddTool(mcp.NewTool("boom"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) { panic("boom") })
	reply, _ := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"boom","arguments":{}}}`)))
	if !strings.Contains(string(reply), "error") || !strings.Contains(string(reply), "boom") {
		t.Fatal(string(reply))
	}
}
