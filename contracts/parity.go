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

// patchTypes corrects params the sim-api OpenRPC generator types as "object" because the Go field is
// a json.RawMessage patch (rpc_form/form_update.go: omitted keeps, null clears). Their real values:
// tags is a lossless legacy value: stored template bags hold an array (legacy createForm sends one).
var patchTypes = map[string]map[string][]string{
	"form/update": {"ref": {"string", "null"}, "description": {"string", "null"}, "color": {"string", "null"},
		"parentId": {"integer", "null"}, "pictureObject": {"object", "null"}, "verificationRules": {"object", "null"},
		"actorVerification": {"object", "null"}, "tags": {"array", "object", "null"}},
	"form/create": {"tags": {"array", "object"}},
}

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
			if types, ok := patchTypes[m.Name][p.Name]; ok {
				p.Schema["type"] = types
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
