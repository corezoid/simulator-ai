package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type cduComponent struct {
	Type     string                     `json:"type"`
	Category string                     `json:"category"`
	Label    string                     `json:"label"`
	Kind     string                     `json:"kind,omitempty"`
	Props    map[string]json.RawMessage `json:"props,omitempty"`
	Events   []string                   `json:"events,omitempty"`
	Slots    []string                   `json:"slots,omitempty"`
}

type cduCatalog struct {
	SchemaVersion string         `json:"schemaVersion"`
	Components    []cduComponent `json:"components"`
}

type componentsInput struct {
	Types []string `json:"types,omitempty"`
}

const componentsOutput = `{"type":"object","properties":{"schemaVersion":{"type":"string"},"components":{"type":"array","items":{"type":"object","properties":{"type":{"type":"string"},"category":{"type":"string"},"label":{"type":"string"},"kind":{"type":"string"},"props":{"type":"object"},"events":{"type":"array","items":{"type":"string"}},"slots":{"type":"array","items":{"type":"string"}}},"required":["type","category","label"]}}},"required":["schemaVersion","components"]}`

// addSmartFormComponents registers the read-only CDU component reference; it calls no API.
func addSmartFormComponents(s *server.MCPServer) {
	var catalog cduCatalog
	if err := json.Unmarshal(contracts.CDUComponents, &catalog); err != nil {
		panic(err) // pinned build artifact
	}
	byType := map[string]cduComponent{}
	for _, c := range catalog.Components {
		byType[c.Type] = c
	}
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(componentsOutput)), "smart_form_components", "The CDU component types a Smart Form page config may use (schemaVersion 2.0). Without types: every type with its category and label. With types: those types with their props (type, enum, description), events and slots. Use ONLY these types and props: sim-api refuses an unknown type or prop when the page is saved. A field caption is TextInput label; DatePicker and Select have no label prop, so put a Label before them.",
		`{"type":"object","properties":{"types":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"string","minLength":1,"maxLength":64}}},"additionalProperties":false}`, false,
		func(_ context.Context, in componentsInput) (cduCatalog, error) {
			out := cduCatalog{SchemaVersion: catalog.SchemaVersion, Components: []cduComponent{}}
			if len(in.Types) == 0 {
				for _, c := range catalog.Components {
					out.Components = append(out.Components, cduComponent{Type: c.Type, Category: c.Category, Label: c.Label})
				}
				return out, nil
			}
			var unknown []string
			for _, t := range in.Types {
				if c, ok := byType[t]; ok {
					out.Components = append(out.Components, c)
				} else {
					unknown = append(unknown, t)
				}
			}
			if len(unknown) > 0 {
				sort.Strings(unknown)
				return cduCatalog{}, fmt.Errorf("unknown CDU component types %v; call smart_form_components without types for the list", unknown)
			}
			return out, nil
		})
}
