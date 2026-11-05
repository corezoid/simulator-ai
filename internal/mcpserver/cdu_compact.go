package mcpserver

import (
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/cduschema"
)

// smart_form_components answers a narrowed catalog (types or kind) compactly: each component's
// props, events, state and api one line each, in a TypeScript-like notation with $refs resolved
// and enums inlined, instead of the JSON Schemas with their whole $defs closures. Stack and Form
// alone were 60 KB of schema, which the Console agent never saw past a spilled preview; the
// line for Stack's gap is `size?: 'none'|…|'11xl' | {{expr}} — Gap size between items`. schema
// true returns the backend's JSON Schemas unchanged.

// catalogNotation explains the compact lines once per answer.
const catalogNotation = "TypeScript-like: key?: optional, 'a'|'b' enum, {{expr}} a {{ }} expression string, A[] array, Name a value of enums[Name]; HTML, aria-* and on* attributes are left out. schema:true returns the JSON Schemas."

type compactType struct {
	Type     string            `json:"type"`
	Kind     string            `json:"kind,omitempty"`
	Category string            `json:"category,omitempty"`
	Label    string            `json:"label,omitempty"`
	Props    []string          `json:"props,omitempty"`
	Events   []string          `json:"events,omitempty"`
	State    []string          `json:"state,omitempty"`
	API      []string          `json:"api,omitempty"`
	Enums    map[string]string `json:"enums,omitempty"`
}

type catalogType struct {
	Type     string         `json:"type"`
	Kind     string         `json:"kind,omitempty"`
	Category string         `json:"category,omitempty"`
	Label    string         `json:"label,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Events   map[string]any `json:"events,omitempty"`
	State    map[string]any `json:"state,omitempty"`
	API      map[string]any `json:"api,omitempty"`
}

// compactCatalogUnlessSchema is smart_form_components' Shape: compact unless the call sent schema true.
func compactCatalogUnlessSchema(local map[string]json.RawMessage, v json.RawMessage) (json.RawMessage, error) {
	if string(local["schema"]) == "true" {
		return v, nil
	}
	return compactCatalog(v)
}

// compactCatalog rewrites a cdu/catalog/get answer; a brief one (no schemas) is returned as it came.
func compactCatalog(v json.RawMessage) (json.RawMessage, error) {
	var doc struct {
		SchemaVersion string        `json:"schemaVersion"`
		List          []catalogType `json:"list"`
		Unknown       []string      `json:"unknown,omitempty"`
	}
	if json.Unmarshal(v, &doc) != nil {
		return v, nil
	}
	schemas := false
	out := make([]compactType, len(doc.List))
	for i, t := range doc.List {
		schemas = schemas || t.Props != nil || t.Events != nil || t.State != nil || t.API != nil
		out[i] = compactOne(t)
	}
	if !schemas {
		return v, nil
	}
	return json.Marshal(struct {
		SchemaVersion string        `json:"schemaVersion"`
		Notation      string        `json:"notation"`
		List          []compactType `json:"list"`
		Unknown       []string      `json:"unknown,omitempty"`
	}{doc.SchemaVersion, catalogNotation, out, doc.Unknown})
}

func compactOne(t catalogType) compactType {
	defs, _ := t.Props["$defs"].(map[string]any)
	r := cduschema.New(defs)
	out := compactType{Type: t.Type, Kind: t.Kind, Category: t.Category, Label: t.Label}
	if t.Props != nil {
		out.Props = r.KeyLines(t.Props, true)
	}
	for _, name := range cduschema.SortedKeys(t.Events) {
		ev, _ := t.Events[name].(map[string]any)
		line := name
		if props, _ := ev["properties"].(map[string]any); len(props) > 0 {
			line += " $event " + r.Object(ev)
		}
		out.Events = append(out.Events, line+cduschema.Describe(ev))
	}
	if t.State != nil {
		out.State = r.KeyLines(t.State, false)
	}
	for _, name := range cduschema.SortedKeys(t.API) {
		m, _ := t.API[name].(map[string]any)
		args := ""
		if a, _ := m["args"].(map[string]any); a != nil {
			if props, _ := a["properties"].(map[string]any); len(props) > 0 {
				args = r.Object(a)
			}
		}
		out.API = append(out.API, name+"("+args+")"+cduschema.Describe(m))
	}
	if named := r.Named(); len(named) > 0 {
		out.Enums = named
	}
	return out
}
