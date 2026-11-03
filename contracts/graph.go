package contracts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// GraphReads is generated from reviewed public descriptors, never internal picker APIs.
//
//go:embed graph-openrpc.json
var GraphReads []byte

var graphOnce sync.Once
var graphSchemas map[string]json.RawMessage
var graphOutputs map[string]json.RawMessage
var graphValidators map[string]*jsonschema.Schema
var graphError error

func initializeGraph() {
	graphSchemas, graphOutputs = map[string]json.RawMessage{}, map[string]json.RawMessage{}
	graphValidators = map[string]*jsonschema.Schema{}
	var source struct {
		Methods []struct {
			Method
			Result struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
	}
	if graphError = json.Unmarshal(GraphReads, &source); graphError != nil {
		return
	}
	for _, method := range source.Methods {
		if method.Audience != "public" {
			graphError = errors.New("non-public graph contract")
			return
		}
		properties := map[string]any{}
		required := []string{}
		for _, param := range method.Params {
			if param.Name == "accId" {
				continue
			}
			properties[param.Name] = param.Schema
			if param.Required {
				required = append(required, param.Name)
			}
			switch param.Name {
			case "limit":
				param.Schema["maximum"] = 100
				if method.Name == "edge_type/list" {
					param.Schema["default"] = 50
				}
			case "blockIds":
				param.Schema["minItems"] = 1
				param.Schema["maxItems"] = 100
				// Backend dive,min=1,max=128 lives in x-validation-rules, not item JSON Schema.
				param.Schema["items"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
			case "title":
				param.Schema["enum"] = []string{"Graphs", "Layers", "FlowchartBlock"}
			}
		}
		schema := map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
		raw, err := json.Marshal(schema)
		if err != nil {
			graphError = err
			return
		}
		compiler := jsonschema.NewCompiler()
		uri := "https://simulator.company/schemas/mcp/graph/" + method.Name
		var value any
		if err = json.Unmarshal(raw, &value); err != nil {
			graphError = err
			return
		}
		if err = compiler.AddResource(uri, value); err != nil {
			graphError = err
			return
		}
		validator, err := compiler.Compile(uri)
		if err != nil {
			graphError = err
			return
		}
		output, err := graphOutput(method.Name, method.Result.Schema)
		if err != nil {
			graphError = err
			return
		}
		graphSchemas[method.Name], graphOutputs[method.Name], graphValidators[method.Name] = raw, output, validator
	}
}

func GraphInputSchema(method string) (json.RawMessage, error) {
	graphOnce.Do(initializeGraph)
	if graphError != nil {
		return nil, graphError
	}
	schema, ok := graphSchemas[method]
	if !ok {
		return nil, errors.New("unsupported graph metadata method")
	}
	return append(json.RawMessage(nil), schema...), nil
}

// GraphOutputSchema is the public response schema; clients return only named projections.
func GraphOutputSchema(method string) (json.RawMessage, error) {
	if _, err := GraphInputSchema(method); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), graphOutputs[method]...), nil
}

func ValidateGraphInput(method string, raw []byte) error {
	if _, err := GraphInputSchema(method); err != nil {
		return err
	}
	if !json.Valid(raw) {
		return errors.New("invalid graph metadata input")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return errors.New("invalid graph metadata input")
	}
	if err := graphValidators[method].Validate(value); err != nil {
		return errors.New("invalid graph metadata input: follow the declared tool schema")
	}
	return nil
}
