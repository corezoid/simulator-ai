package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
)

// The reads are on in the console and in graph-read; the person's own write in the console only,
// never read-only and never destructive; graph_extra holds all three, the plain console none.
func TestLayerCollapseCatalog(t *testing.T) {
	full := consoleTools(t, "console-full").ListTools()
	for name, readOnly := range map[string]bool{"layer_collapse_get": true, "layer_collapse_count": true, "layer_collapse_update": false} {
		tool := full[name]
		if tool == nil {
			t.Fatalf("console-full lacks %s", name)
		}
		a := tool.Tool.Annotations
		if *a.ReadOnlyHint != readOnly || *a.DestructiveHint || !*a.IdempotentHint {
			t.Errorf("%s annotations %+v", name, a)
		}
		if !strings.Contains(strings.Join(toolGroups[name], ","), "graph_extra") {
			t.Errorf("%s not in graph_extra", name)
		}
	}
	core := consoleTools(t, "console").ListTools()
	if core["layer_collapse_get"] != nil || core["layer_collapse_update"] != nil {
		t.Error("the plain console catalog grew the collapse tools")
	}
	read := consoleTools(t, "graph-read").ListTools()
	if read["layer_collapse_get"] == nil || read["layer_collapse_count"] == nil || read["layer_collapse_update"] != nil {
		t.Error("graph-read: reads only")
	}
	if consoleTools(t, "graph-create").ListTools()["layer_collapse_get"] != nil {
		t.Error("graph-create stays the strict create catalog")
	}
}

// With an API key the write is refused before any request: it would fold the API user's view.
func TestLayerCollapseUpdateRefusesAnAPIKey(t *testing.T) {
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console-full", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	tool := New(c).GetTool("layer_collapse_update")
	req := mcp.CallToolRequest{}
	req.Params.Name = "layer_collapse_update"
	req.Params.Arguments = map[string]any{"layerId": "L", "collapse": []any{1}}
	res, err := tool.Handler(context.Background(), req)
	if err != nil || !res.IsError {
		t.Fatal(res, err)
	}
	if text := res.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "API key") {
		t.Fatal(text)
	}
}
