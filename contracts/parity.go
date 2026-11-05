package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Parity is the public OpenRPC projection of the actor, form and graph methods behind the legacy
// parity tools (parity-source.json pins its source). Tool input schemas are its params, so names,
// types and required keys are the backend's own; accId is host-injected and never a tool argument.
//
//go:embed parity-openrpc.json
var Parity []byte

// ParitySource records where Parity was exported from.
//
//go:embed parity-source.json
var ParitySource []byte

var parityOnce sync.Once
var paritySchemas map[string]json.RawMessage
var parityErr error

func loadParity() {
	var d Document
	if parityErr = json.Unmarshal(Parity, &d); parityErr != nil {
		return
	}
	paritySchemas = map[string]json.RawMessage{}
	for _, m := range d.Methods {
		properties, required := map[string]any{}, []string{}
		for _, p := range m.Params {
			if p.Name == "accId" {
				continue
			}
			properties[p.Name] = compact(p.Schema, 0)
			if p.Required {
				required = append(required, p.Name)
			}
		}
		paritySchemas[m.Name], parityErr = json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
		if parityErr != nil {
			return
		}
	}
}

// ParityInputSchema is the tool input schema of one pinned parity method.
func ParityInputSchema(method string) (json.RawMessage, error) {
	parityOnce.Do(loadParity)
	if parityErr != nil {
		return nil, parityErr
	}
	s, ok := paritySchemas[method]
	if !ok {
		return nil, fmt.Errorf("parity method %s is not pinned", method)
	}
	return s, nil
}

// maxParamDescription bounds a parameter description: tool schemas ride in the model context on
// every turn, so a param keeps its first sentence and nested fields keep only their types.
const maxParamDescription = 160

// compact shortens a param schema for the model: the top-level description to its first sentence,
// nested descriptions removed. Types, enums, bounds and required keys stay exact.
func compact(v any, depth int) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			// A string description is an annotation; a "description" that is a schema is a real
			// property (a template field has one) and stays.
			if d, ok := x.(string); ok && k == "description" {
				if depth == 0 {
					out[k] = firstSentence(d)
				}
				continue
			}
			out[k] = compact(x, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = compact(x, depth+1)
		}
		return out
	}
	return v
}

func firstSentence(d string) string {
	d = strings.Join(strings.Fields(d), " ")
	if i := strings.Index(d, ". "); i > 0 {
		d = d[:i+1]
	}
	if r := []rune(d); len(r) > maxParamDescription {
		d = string(r[:maxParamDescription-1]) + "…"
	}
	return d
}
