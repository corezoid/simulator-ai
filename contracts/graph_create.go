package contracts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// GraphCreates pins six reviewed public create/recovery descriptors, never update/delete.
//
//go:embed graph-create-openrpc.json
var GraphCreates []byte

var graphCreateOnce sync.Once
var graphCreateSchemas map[string]json.RawMessage
var graphCreateValidators map[string]*jsonschema.Schema
var graphCreateError error

var graphCreateFields = map[string][]string{
	"actor/create":              {"operationId", "formId", "title", "description", "data", "color", "contextLayerId"},
	"actor/operation/get":       {"operationId"},
	"edge/create":               {"operationId", "source", "target", "edgeTypeId", "name", "linkedActorId", "curveStyle", "forceDirection"},
	"edge/operation/get":        {"operationId"},
	"graph/layer/actors/manage": {"operationId", "actorId", "items"},
	"graph/layer/operation/get": {"operationId", "actorId"},
}

func initializeGraphCreate() {
	graphCreateSchemas = map[string]json.RawMessage{}
	graphCreateValidators = map[string]*jsonschema.Schema{}
	var document Document
	if graphCreateError = json.Unmarshal(GraphCreates, &document); graphCreateError != nil {
		return
	}
	for _, method := range document.Methods {
		schema, err := graphCreateInput(method)
		if err != nil {
			graphCreateError = err
			return
		}
		raw, err := json.Marshal(schema)
		if err != nil {
			graphCreateError = err
			return
		}
		compiler := jsonschema.NewCompiler()
		uri := "https://simulator.company/schemas/mcp/create/" + method.Name
		if err = compiler.AddResource(uri, schema); err != nil {
			graphCreateError = err
			return
		}
		validator, err := compiler.Compile(uri)
		if err != nil {
			graphCreateError = err
			return
		}
		graphCreateSchemas[method.Name], graphCreateValidators[method.Name] = raw, validator
	}
}

func graphCreateInput(method Method) (map[string]any, error) {
	fields, ok := graphCreateFields[method.Name]
	if !ok || method.Audience != "public" || method.Scope != "actors.management" {
		return nil, errors.New("unsupported graph creation contract")
	}
	source := map[string]any{}
	for _, p := range method.Params {
		source[p.Name] = p.Schema
	}
	schema := map[string]any{"type": "object", "properties": source}
	if err := retainGraphProperties(schema, fields); err != nil {
		return nil, err
	}
	props := schema["properties"].(map[string]any)
	required := []string{"operationId"}
	for name, value := range props {
		field := value.(map[string]any)
		switch name {
		case "operationId":
			field["pattern"] = `^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`
		case "formId", "edgeTypeId":
			field["minimum"] = 1
		case "actorId", "source", "target", "contextLayerId", "linkedActorId":
			field["minLength"], field["maxLength"] = 1, 255
		case "title":
			field["minLength"], field["maxLength"] = 1, 1000
		case "color":
			field["pattern"] = `^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`
		}
	}
	switch method.Name {
	case "actor/create":
		required = append(required, "formId", "title", "data")
	case "edge/create":
		required = append(required, "source", "target", "edgeTypeId")
	case "graph/layer/operation/get":
		required = append(required, "actorId")
	case "graph/layer/actors/manage":
		required = append(required, "actorId", "items")
		if err := narrowPlacementSchema(props["items"].(map[string]any)); err != nil {
			return nil, err
		}
	}
	schema["required"] = required
	// Normalize []string and integer values for the JSON Schema compiler.
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	err = json.Unmarshal(raw, &normalized)
	return normalized, err
}

func GraphCreateInputSchema(method string) (json.RawMessage, error) {
	graphCreateOnce.Do(initializeGraphCreate)
	if graphCreateError != nil {
		return nil, graphCreateError
	}
	schema, ok := graphCreateSchemas[method]
	if !ok {
		return nil, errors.New("unsupported graph creation method")
	}
	return append(json.RawMessage(nil), schema...), nil
}

func ValidateGraphCreateInput(method string, raw []byte) error {
	if _, err := GraphCreateInputSchema(method); err != nil {
		return err
	}
	if !json.Valid(raw) {
		return errors.New("invalid graph creation input")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || graphCreateValidators[method].Validate(value) != nil {
		return errors.New("invalid graph creation input: follow the declared tool schema")
	}
	return nil
}

// CheckGraphCreateCompatibility prevents a lagging backend silently ignoring receipt keys.
func CheckGraphCreateCompatibility(raw []byte) error {
	var expected, actual Document
	if json.Unmarshal(GraphCreates, &expected) != nil || json.Unmarshal(raw, &actual) != nil {
		return errors.New("invalid public contract")
	}
	methods := map[string]Method{}
	for _, m := range actual.Methods {
		if _, ok := methods[m.Name]; ok {
			return errors.New("duplicate public method")
		}
		methods[m.Name] = m
	}
	for _, want := range expected.Methods {
		got, ok := methods[want.Name]
		if !ok || got.Audience != "public" || got.Scope != want.Scope {
			return errors.New("graph creation backend contract unavailable")
		}
		stripDescriptions(want.Params)
		stripDescriptions(got.Params)
		index := func(ps []Parameter) map[string]Parameter {
			out := map[string]Parameter{}
			for _, p := range ps {
				out[p.Name] = p
			}
			return out
		}
		if len(got.Params) != len(want.Params) || !reflect.DeepEqual(index(want.Params), index(got.Params)) {
			return errors.New("graph creation backend contract mismatch")
		}
	}
	return nil
}
