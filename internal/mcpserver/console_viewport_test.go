package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// layer_viewport's hand-written schema keeps to the pinned graph/viewport/command params: the same
// names and actions; only cell differs on purpose (the ruler address, sent as sim-api's index).
func TestLayerViewportFollowsThePin(t *testing.T) {
	var doc struct {
		Methods []struct {
			Name   string          `json:"name"`
			Params json.RawMessage `json:"params"` // other methods' schema types may be arrays
		} `json:"methods"`
	}
	if err := json.Unmarshal(contracts.Parity, &doc); err != nil {
		t.Fatal(err)
	}
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	var tool struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(New(c).GetTool("layer_viewport").Tool.RawInputSchema, &tool); err != nil {
		t.Fatal(err)
	}
	for _, m := range doc.Methods {
		if m.Name != "graph/viewport/command" {
			continue
		}
		var params []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
			Schema   struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"schema"`
		}
		if err := json.Unmarshal(m.Params, &params); err != nil {
			t.Fatal(err)
		}
		var pinned, required []string
		for _, p := range params {
			if p.Name == "accId" {
				continue
			}
			pinned = append(pinned, p.Name)
			if p.Required {
				required = append(required, p.Name)
			}
			got := tool.Properties[p.Name]
			if p.Name != "cell" && got.Type != p.Schema.Type || fmt.Sprint(got.Enum) != fmt.Sprint(p.Schema.Enum) {
				t.Errorf("%s: tool %s %v, pinned %s %v", p.Name, got.Type, got.Enum, p.Schema.Type, p.Schema.Enum)
			}
		}
		var props []string
		for name := range tool.Properties {
			props = append(props, name)
		}
		sort.Strings(pinned)
		sort.Strings(props)
		sort.Strings(required)
		sort.Strings(tool.Required)
		if fmt.Sprint(props) != fmt.Sprint(pinned) || fmt.Sprint(tool.Required) != fmt.Sprint(required) {
			t.Fatalf("layer_viewport %v (required %v), pinned %v (required %v)", props, tool.Required, pinned, required)
		}
		return
	}
	t.Fatal("graph/viewport/command is not pinned")
}
