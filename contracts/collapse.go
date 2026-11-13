package contracts

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// CollapseSource is the generated projection of the reviewed public graph/layer/collapse/*
// descriptors; collapse-source.json pins the sim-api revision it was projected from.
//
//go:embed collapse-openrpc.json
var CollapseSource []byte

// collapseParams is the exact param set of each collapse method this plugin was reviewed against.
var collapseParams = map[string][]string{
	"graph/layer/collapse/children":     {"accId", "actorId", "exclude", "laId", "limit", "maxVisibleChildren", "offset", "q"},
	"graph/layer/collapse/count":        {"accId", "actorId", "condition", "laId"},
	"graph/layer/collapse/resolve":      {"accId", "actorId", "conditional", "fanOpen", "fanShown", "maxVisibleChildren", "roots", "zone"},
	"graph/layer/collapse/state/get":    {"accId", "actorId"},
	"graph/layer/collapse/state/update": {"accId", "actorId", "collapse", "conditional", "expand", "expandAll", "fanClose", "fanOpen", "fanReset"},
}

// collapseStateFields are the state/get result fields the tools read.
var collapseStateFields = []string{"conditional", "fanOpen", "fanShown", "maxVisibleChildren", "roots"}

// layerIDSchema is the layer argument of every collapse tool; actorId is its backend name.
var layerIDSchema = map[string]any{"type": "string", "minLength": 1, "maxLength": 255,
	"description": "Layer actor id (the current layer of the request's context unless the user names another)"}

// CollapseSchemas are the input schemas of layer_collapse_get, layer_collapse_count and
// layer_collapse_update, derived from the projection after checking it is the reviewed contract.
func CollapseSchemas() (get, count, update json.RawMessage, err error) {
	var source struct {
		Methods []struct {
			Method
			Result struct {
				Schema map[string]any `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
	}
	if err = json.Unmarshal(CollapseSource, &source); err != nil {
		return nil, nil, nil, err
	}
	if len(source.Methods) != len(collapseParams) {
		return nil, nil, nil, errors.New("unsupported collapse contract")
	}
	params := map[string]map[string]map[string]any{}
	for _, m := range source.Methods {
		want, ok := collapseParams[m.Name]
		if !ok || m.Audience != "public" || m.Scope != "actors.readonly" {
			return nil, nil, nil, fmt.Errorf("collapse contract drift: %s", m.Name)
		}
		byName := map[string]map[string]any{}
		for _, p := range m.Params {
			byName[p.Name] = withoutExtensions(p.Schema).(map[string]any)
		}
		if !slices.Equal(slices.Sorted(maps.Keys(byName)), want) {
			return nil, nil, nil, fmt.Errorf("collapse parameter drift: %s", m.Name)
		}
		params[m.Name] = byName
		if m.Name == "graph/layer/collapse/state/get" {
			props, _ := m.Result.Schema["properties"].(map[string]any)
			if !slices.Equal(slices.Sorted(maps.Keys(props)), collapseStateFields) {
				return nil, nil, nil, errors.New("collapse state result drift")
			}
		}
	}
	children := params["graph/layer/collapse/children"]
	if children["limit"]["maximum"] != float64(50) || children["q"]["maxLength"] != float64(200) {
		return nil, nil, nil, errors.New("collapse children paging drift")
	}
	get, err = json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"layerId"},
		"properties": map[string]any{
			"layerId": layerIDSchema,
			"laId": map[string]any{"type": "integer", "minimum": 1,
				"description": "A collapsed actor or a wide fan's parent (laId): also list the placements hidden under it, one page"},
			"q":      map[string]any{"type": "string", "maxLength": 200, "description": "With laId of a fan: narrow its hidden children by title"},
			"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "Page size of the hidden list, 10 by default"},
			"offset": map[string]any{"type": "integer", "minimum": 0},
		}})
	if err != nil {
		return nil, nil, nil, err
	}
	c := params["graph/layer/collapse/count"]
	count, err = json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"layerId", "laId", "condition"},
		"properties": map[string]any{"layerId": layerIDSchema, "laId": c["laId"], "condition": c["condition"]}})
	if err != nil {
		return nil, nil, nil, err
	}
	u := params["graph/layer/collapse/state/update"]
	properties := map[string]any{"layerId": layerIDSchema}
	for name, schema := range u {
		if name != "accId" && name != "actorId" {
			// The condition's own words ride in layer_collapse_count; here its types do.
			properties[name] = compact(schema, 0)
		}
	}
	item, ok := properties["conditional"].(map[string]any)["items"].(map[string]any)
	if !ok {
		return nil, nil, nil, errors.New("collapse conditional drift")
	}
	summary, ok := item["properties"].(map[string]any)["summary"].(map[string]any)
	if !ok {
		return nil, nil, nil, errors.New("collapse summary drift")
	}
	summary["description"] = "Words the Graph Editor shows on the fold, e.g. \"Order: Stage = paid\"; omitted: the template title and q"
	update, err = json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"layerId"},
		"properties": properties, "minProperties": 2})
	return get, count, update, err
}
