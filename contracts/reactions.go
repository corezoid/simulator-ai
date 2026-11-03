package contracts

import (
	_ "embed"
	"encoding/json"
	"errors"
)

// ReactionStatsSource pins the named public endpoint; never private/legacy API.
//
//go:embed reactions-openrpc.json
var ReactionStatsSource []byte

// ReactionStatsInputSchema projects the pinned rootActorId to legacy actorId.
// AccID stays host-injected. The tighter nonempty 255-byte client limit is explicit.
func ReactionStatsInputSchema() (json.RawMessage, error) {
	var source struct {
		Methods []struct {
			Method
			Result struct {
				Schema struct {
					Type                 string `json:"type"`
					AdditionalProperties struct {
						Type string `json:"type"`
					} `json:"additionalProperties"`
				} `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(ReactionStatsSource, &source); err != nil {
		return nil, err
	}
	if len(source.Methods) != 1 {
		return nil, errors.New("unsupported reaction stats contract")
	}
	method := source.Methods[0]
	if method.Name != "reaction/stats/get" || method.Audience != "public" || method.Scope != "actors.readonly" || method.Result.Schema.Type != "object" || method.Result.Schema.AdditionalProperties.Type != "integer" || len(method.Params) != 2 {
		return nil, errors.New("reaction stats contract drift")
	}
	seen := map[string]bool{}
	for _, p := range method.Params {
		if (p.Name != "rootActorId" && p.Name != "accId") || p.Schema["type"] != "string" || !p.Required || seen[p.Name] {
			return nil, errors.New("reaction stats parameter drift")
		}
		seen[p.Name] = true
	}
	return json.RawMessage(`{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255}},"required":["actorId"],"additionalProperties":false}`), nil
}

const ReactionStatsOutputSchema = `{"type":"object","additionalProperties":{"type":"integer","minimum":0},"propertyNames":{"minLength":1}}`
