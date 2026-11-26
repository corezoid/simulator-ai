package contracts

import (
	_ "embed"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
)

// DashboardSource is the generated projection of the reviewed public dashboard/get descriptor;
// dashboard-source.json pins the sim-api revision it was projected from.
//
//go:embed dashboard-openrpc.json
var DashboardSource []byte

// dashboardToolParams maps each chart_data_get argument to its dashboard/get param. chartId is the
// dashboard actor (chart_create's chartId), named apart from actorIds, which narrows the series
// actors. accId is host-owned; source (the web app's preview of an unsaved configuration) is not
// offered: the tool reads what a saved chart shows.
var dashboardToolParams = map[string]string{
	"chartId": "actorId", "from": "from", "to": "to", "interval": "interval", "timezoneOffset": "timezoneOffset",
	"balanceOnDate": "balanceOnDate", "counterType": "counterType", "actorIds": "actorIds", "with": "with",
}

// dashboardParams is the exact dashboard/get param set this tool was reviewed against.
var dashboardParams = []string{"accId", "actorId", "actorIds", "balanceOnDate", "counterType", "from", "interval", "source", "timezoneOffset", "to", "with"}

// DashboardWith is the closed with set of dashboard/get.
var DashboardWith = []string{"total", "series"}

// DashboardSchemas are the chart_data_get input and output schemas, derived from the projection.
func DashboardSchemas() (json.RawMessage, json.RawMessage, error) {
	var source struct {
		Methods []struct {
			Method
			Result struct {
				Schema map[string]any `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(DashboardSource, &source); err != nil {
		return nil, nil, err
	}
	if len(source.Methods) != 1 {
		return nil, nil, errors.New("unsupported dashboard contract")
	}
	m := source.Methods[0]
	if m.Name != "dashboard/get" || m.Audience != "public" || m.Scope != "actors.readonly" {
		return nil, nil, errors.New("dashboard contract drift")
	}
	params := map[string]map[string]any{}
	for _, p := range m.Params {
		if p.Required != (p.Name == "accId" || p.Name == "actorId") {
			return nil, nil, errors.New("dashboard required parameter drift")
		}
		params[p.Name] = withoutExtensions(p.Schema).(map[string]any)
	}
	if !slices.Equal(slices.Sorted(maps.Keys(params)), dashboardParams) {
		return nil, nil, errors.New("dashboard parameter drift")
	}
	properties := map[string]any{}
	for arg, param := range dashboardToolParams {
		properties[arg] = params[param]
	}
	chart := properties["chartId"].(map[string]any)
	chart["maxLength"], chart["description"] = 255, "Chart (dashboard actor) id: chart_create's chartId, or an actor of the Dashboards template"
	actors := properties["actorIds"].(map[string]any)
	// An empty list would be dropped and read every series: refuse it, as graph-read does elsewhere.
	actors["minItems"], actors["maxItems"], actors["items"] = 1, 100, map[string]any{"type": "string", "minLength": 1, "maxLength": 255}
	with := properties["with"].(map[string]any)
	with["items"], with["uniqueItems"], with["maxItems"] = map[string]any{"type": "string", "enum": DashboardWith}, true, len(DashboardWith)
	input, err := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": []string{"chartId"}, "additionalProperties": false})
	if err != nil {
		return nil, nil, err
	}
	output := withoutExtensions(m.Result.Schema).(map[string]any)
	if props, ok := output["properties"].(map[string]any); !ok || props["data"] == nil || output["type"] != "object" {
		return nil, nil, errors.New("dashboard result drift")
	}
	output["required"] = []string{"data"}
	raw, err := json.Marshal(output)
	return input, raw, err
}

// withoutExtensions copies a schema without its x- keywords (the backend's validator tags): the
// standard keywords already say the same, and a tool schema rides in the model context.
func withoutExtensions(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			if !strings.HasPrefix(k, "x-") {
				out[k] = withoutExtensions(x)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = withoutExtensions(x)
		}
		return out
	}
	return v
}
