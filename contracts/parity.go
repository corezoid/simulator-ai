package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
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
			properties[p.Name] = p.Schema
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
