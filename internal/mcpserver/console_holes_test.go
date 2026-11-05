package mcpserver

import (
	"encoding/json"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
)

type pinnedParam struct {
	Type     string
	Required bool
}

// pinnedParams is one pinned method's params by name, accId aside.
func pinnedParams(t *testing.T, method string) map[string]pinnedParam {
	var doc struct {
		Methods []struct {
			Name   string          `json:"name"`
			Params json.RawMessage `json:"params"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(contracts.Parity, &doc); err != nil {
		t.Fatal(err)
	}
	for _, m := range doc.Methods {
		if m.Name != method {
			continue
		}
		var params []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
			Schema   struct {
				Type string `json:"type"`
			} `json:"schema"`
		}
		if err := json.Unmarshal(m.Params, &params); err != nil {
			t.Fatal(err)
		}
		out := map[string]pinnedParam{}
		for _, p := range params {
			if p.Name != "accId" {
				out[p.Name] = pinnedParam{p.Schema.Type, p.Required}
			}
		}
		return out
	}
	t.Fatalf("%s is not pinned", method)
	return nil
}

// The hand-written close schemas name the merge methods' own params with their pinned types, and
// every param the merge requires is the tool's (the closer id aside, which the tool may create).
func TestHoleCloseFollowsThePin(t *testing.T) {
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console-full", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	for tool, method := range map[string]string{"hole_close": "actor/hole/merge", "edge_hole_close": "edge/hole/merge"} {
		var schema struct {
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(s.GetTool(tool).Tool.RawInputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		closer := map[string]string{"hole_close": "actorId", "edge_hole_close": "edgeId"}[tool]
		required := map[string]bool{}
		for _, name := range schema.Required {
			required[name] = true
		}
		for name, p := range pinnedParams(t, method) {
			got, ok := schema.Properties[name]
			if name == "targetLaId" { // the Graph Editor's drag close; a tool call has no dragged placement
				continue
			}
			if !ok || got.Type != p.Type {
				t.Errorf("%s.%s: tool %q, pinned %q", tool, name, got.Type, p.Type)
			}
			if p.Required && name != closer && !required[name] {
				t.Errorf("%s.%s is required by %s", tool, name, method)
			}
		}
	}
}
