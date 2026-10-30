package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Method is an OpenRPC method descriptor; schema values are JSON Schema, not domain data.
type Method struct {
	Name     string      `json:"name"`
	Audience string      `json:"x-audience"`
	Scope    string      `json:"x-scope"`
	Params   []Parameter `json:"params"`
}
type Parameter struct {
	Name     string         `json:"name"`
	Required bool           `json:"required,omitempty"`
	Schema   map[string]any `json:"schema"`
}
type Document struct {
	Methods []Method `json:"methods"`
}

var once sync.Once
var schemas map[string]json.RawMessage
var validators map[string]*jsonschema.Schema
var profileError error

func initialize() {
	schemas = make(map[string]json.RawMessage)
	validators = make(map[string]*jsonschema.Schema)
	var d Document
	if profileError = json.Unmarshal(SmartForms, &d); profileError != nil {
		return
	}
	for _, m := range d.Methods {
		properties := map[string]any{}
		required := []string{}
		for _, p := range m.Params {
			if p.Name == "accId" || (m.Name == "application/create" && p.Name != "operationId" && p.Name != "ref" && p.Name != "title" && p.Name != "description") {
				continue
			}
			properties[p.Name] = p.Schema
			if p.Required || p.Name == "operationId" || p.Name == "environmentPreconditions" || p.Name == "expectedSourceEnvironmentHash" || p.Name == "expectedTargetEnvironmentHash" {
				required = append(required, p.Name)
			}
			strengthen(p.Name, p.Schema)
		}
		schema := map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
		if m.Name == "app_file/update" {
			item := properties["items"].(map[string]any)["items"].(map[string]any)
			item["required"] = []string{"id", "title", "type", "ifMatchHash"}
		}
		if m.Name == "app_file/create" {
			item := properties["items"].(map[string]any)["items"].(map[string]any)
			item["required"] = []string{"folderId", "title", "type", "source"}
		}
		b, err := json.Marshal(schema)
		if err != nil {
			profileError = err
			return
		}
		compiler := jsonschema.NewCompiler()
		u := "https://simulator.company/schemas/mcp/" + m.Name
		var normalized any
		if err = json.Unmarshal(b, &normalized); err != nil {
			profileError = err
			return
		}
		if err = compiler.AddResource(u, normalized); err != nil {
			profileError = err
			return
		}
		v, err := compiler.Compile(u)
		if err != nil {
			profileError = err
			return
		}
		schemas[m.Name], validators[m.Name] = b, v
	}
}
func strengthen(name string, s map[string]any) {
	switch s["type"] {
	case "object":
		s["additionalProperties"] = false
		if p, ok := s["properties"].(map[string]any); ok {
			for k, raw := range p {
				strengthen(k, raw.(map[string]any))
			}
		}
	case "array":
		s["minItems"], s["maxItems"] = 1, 100
		if name == "environmentPreconditions" {
			s["maxItems"] = 16
		}
		if i, ok := s["items"].(map[string]any); ok {
			strengthen("", i)
		}
	case "string":
		switch name {
		case "operationId":
			s["pattern"] = `^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`
		case "ifMatchHash", "environmentHash", "expectedSourceEnvironmentHash", "expectedTargetEnvironmentHash":
			s["pattern"] = `^[0-9a-f]{64}$`
		case "ref":
			s["pattern"] = `^[0-9a-zA-Z_-]{5,255}$`
		case "actorId", "title", "type":
			s["minLength"] = 1
		}
	}
}

// InputSchema is stricter than public RPC: fixed workspace, credential-free
// creation, mandatory receipt keys and full-content/source preconditions.
func InputSchema(method string) (json.RawMessage, error) {
	once.Do(initialize)
	if profileError != nil {
		return nil, profileError
	}
	b, ok := schemas[method]
	if !ok {
		return nil, fmt.Errorf("unsupported Smart Form method")
	}
	return append(json.RawMessage(nil), b...), nil
}

// ValidateInput also protects direct Go callers; MCP additionally validates before decoding.
func ValidateInput(method string, b []byte) error {
	if _, err := InputSchema(method); err != nil {
		return err
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("invalid Smart Form input")
	}
	if err := validators[method].Validate(value); err != nil {
		return fmt.Errorf("invalid Smart Form input: follow the declared tool schema")
	}
	return nil
}

// CheckCompatibility checks every pinned parameter schema, audience and scope.
// Descriptive prose can evolve; unknown added methods are harmless. New or changed
// parameters fail closed until reviewed, even if potentially backward compatible.
func CheckCompatibility(data []byte) error {
	var expected, actual Document
	if json.Unmarshal(SmartForms, &expected) != nil || json.Unmarshal(data, &actual) != nil {
		return fmt.Errorf("invalid public contract")
	}
	methods := map[string]Method{}
	for _, m := range actual.Methods {
		if _, exists := methods[m.Name]; exists {
			return fmt.Errorf("duplicate public method")
		}
		methods[m.Name] = m
	}
	for _, want := range expected.Methods {
		got, ok := methods[want.Name]
		if !ok || got.Audience != "public" || got.Scope != want.Scope {
			return fmt.Errorf("Smart Forms backend contract unavailable: %s", want.Name)
		}
		stripDescriptions(want.Params)
		stripDescriptions(got.Params)
		// Compare by name, not declaration order.
		index := func(ps []Parameter) map[string]Parameter {
			out := map[string]Parameter{}
			for _, p := range ps {
				out[p.Name] = p
			}
			return out
		}
		if len(got.Params) != len(want.Params) || !reflect.DeepEqual(index(want.Params), index(got.Params)) {
			return fmt.Errorf("Smart Forms backend contract mismatch: %s", want.Name)
		}
	}
	return nil
}
func stripDescriptions(ps []Parameter) {
	var strip func(map[string]any)
	strip = func(s map[string]any) {
		delete(s, "description")
		for _, v := range s {
			if m, ok := v.(map[string]any); ok {
				strip(m)
			}
			if list, ok := v.([]any); ok {
				for _, x := range list {
					if m, ok := x.(map[string]any); ok {
						strip(m)
					}
				}
			}
		}
	}
	for _, p := range ps {
		strip(p.Schema)
	}
}
