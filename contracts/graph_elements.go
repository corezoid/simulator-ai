package contracts

import (
	_ "embed"
	"encoding/json"
	"errors"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// GraphElementsSource pins the supported public exact-selection method.
//
//go:embed graph-elements-openrpc.json
var GraphElementsSource []byte
var elementsOnce sync.Once
var elementsSchema json.RawMessage
var elementsValidator *jsonschema.Schema
var elementsError error

func initializeElements() {
	var source Document
	if elementsError = json.Unmarshal(GraphElementsSource, &source); elementsError != nil {
		return
	}
	if len(source.Methods) != 1 || source.Methods[0].Name != "graph/layer/elements/list" || source.Methods[0].Audience != "public" || source.Methods[0].Scope != "actors.readonly" {
		elementsError = errors.New("unsupported exact graph contract")
		return
	}
	properties := map[string]any{}
	for _, p := range source.Methods[0].Params {
		switch p.Name {
		case "actorId":
			p.Schema["minLength"] = 1
			p.Schema["maxLength"] = 255
			properties[p.Name] = p.Schema
		case "nodes", "edges":
			p.Schema["maxItems"] = 100
			p.Schema["uniqueItems"] = true
			p.Schema["items"] = map[string]any{"type": "integer", "minimum": 1}
			properties[p.Name] = p.Schema
		}
	}
	if len(properties) != 3 {
		elementsError = errors.New("incomplete exact graph contract")
		return
	}
	raw, err := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"actorId", "nodes", "edges"}, "properties": properties})
	if err != nil {
		elementsError = err
		return
	}
	var schema any
	if elementsError = json.Unmarshal(raw, &schema); elementsError != nil {
		return
	}
	compiler := jsonschema.NewCompiler()
	const uri = "https://simulator.company/schemas/mcp/graph-elements"
	if elementsError = compiler.AddResource(uri, schema); elementsError != nil {
		return
	}
	elementsValidator, elementsError = compiler.Compile(uri)
	elementsSchema = raw
}
func GraphElementsInputSchema() (json.RawMessage, error) {
	elementsOnce.Do(initializeElements)
	return append(json.RawMessage(nil), elementsSchema...), elementsError
}
func ValidateGraphElementsInput(raw []byte) error {
	if _, err := GraphElementsInputSchema(); err != nil {
		return err
	}
	var value any
	if json.Unmarshal(raw, &value) != nil || elementsValidator.Validate(value) != nil {
		return errors.New("invalid exact graph input: follow the declared tool schema")
	}
	return nil
}
