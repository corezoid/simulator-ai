package contracts

import (
	_ "embed"
	"encoding/json"
	"errors"
)

// LayerStatsSource is the generated projection of the reviewed public descriptor.
//
//go:embed layer-stats-openrpc.json
var LayerStatsSource []byte

func LayerStatsSchemas() (json.RawMessage, json.RawMessage, error) {
	var source struct {
		Methods []struct {
			Method
			Result struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(LayerStatsSource, &source); err != nil {
		return nil, nil, err
	}
	if len(source.Methods) != 1 {
		return nil, nil, errors.New("unsupported layer stats contract")
	}
	m := source.Methods[0]
	if m.Name != "graph/layer/stats/get" || m.Audience != "public" || m.Scope != "actors.readonly" || len(m.Params) != 2 {
		return nil, nil, errors.New("layer stats contract drift")
	}
	seen := map[string]bool{}
	for _, p := range m.Params {
		if (p.Name != "actorId" && p.Name != "accId") || p.Schema["type"] != "string" || !p.Required || seen[p.Name] {
			return nil, nil, errors.New("layer stats parameter drift")
		}
		seen[p.Name] = true
	}
	var output map[string]any
	if err := json.Unmarshal(m.Result.Schema, &output); err != nil {
		return nil, nil, err
	}
	fields := []string{"nodes", "edges", "holes", "holesClosed", "edgeHoles", "edgeHolesClosed"}
	if err := retainGraphProperties(output, fields); err != nil {
		return nil, nil, err
	}
	props := output["properties"].(map[string]any)
	for _, field := range fields {
		p, ok := props[field].(map[string]any)
		if !ok || p["type"] != "integer" {
			return nil, nil, errors.New("layer stats counter drift")
		}
		p["minimum"] = 0
	}
	output["required"] = fields
	raw, err := json.Marshal(output)
	return json.RawMessage(`{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255}},"required":["actorId"],"additionalProperties":false}`), raw, err
}
