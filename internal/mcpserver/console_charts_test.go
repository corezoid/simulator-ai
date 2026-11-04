package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	chartLayer = "79f6f553-fad8-4ed0-93f2-84c9113b0558"
	chartActor = "0751e107-d31a-4abe-be6a-1aac8f9a450c"
	chartName  = "5e0b9c27-41d8-4a3f-8c6e-92f1a7d04b13"
)

// sent is a call's params with JSON string escapes undone, so a nested source string reads plainly.
func sent(c consoleCall) string {
	b, _ := json.Marshal(c.Params)
	return strings.ReplaceAll(string(b), `\"`, `"`)
}

func TestChartCreate(t *testing.T) {
	placed := map[string]string{"graph/layer/actors/manage": `"result":{"nodes":[{"laId":31,"actorId":"a1"}],"edges":[]}`}
	send, calls := consoleServer(t, placed)
	reply := callTool(send, "chart_create", `{"layerId":"`+chartLayer+`","title":"Revenue","chartType":"bar","position":{"x":100,"y":50},"accounts":[{"actorId":"`+chartActor+`","nameId":"`+chartName+`","currencyId":1}]}`)
	if strings.Contains(reply, `"isError":true`) || methodsOf(*calls) != "form/system/get,actor/create,graph/layer/actors/manage" || !strings.Contains(reply, `"chartId":"a1","laId":31`) {
		t.Fatal(methodsOf(*calls), reply)
	}
	if (*calls)[0].Params["title"] != "Dashboards" || !strings.Contains(sent((*calls)[1]), `"data":{"source":"{"defaultAccount":null,"accounts":[{"actorId":"`+chartActor) ||
		!strings.Contains(sent((*calls)[1]), `"sourceType":"manual"`) || !strings.Contains(sent((*calls)[1]), `"formId":7`) {
		t.Fatal("chart actor:", sent((*calls)[1]))
	}
	if want := `"items":[{"action":"create","data":{"id":"a1","layerSettings":{"expand":true,"expandType":"chart","height":450,"width":650},"position":{"x":100,"y":50},"type":"node"}}]`; !strings.Contains(sent((*calls)[2]), want) {
		t.Fatal("placement:", sent((*calls)[2]))
	}

	// An actorFilter chart first creates its ActorFilters actor, titled by the chart.
	send, calls = consoleServer(t, placed)
	reply = callTool(send, "chart_create", `{"layerId":"`+chartLayer+`","title":"Top customers","sourceFormId":19,"accountNameId":"`+chartName+`","currencyId":2,"top":5}`)
	if strings.Contains(reply, `"isError":true`) || methodsOf(*calls) != "form/system/get,form/system/get,actor/create,actor/create,graph/layer/actors/manage" || !strings.Contains(reply, `"filterActorId":"a1"`) {
		t.Fatal(methodsOf(*calls), reply)
	}
	if (*calls)[1].Params["title"] != "ActorFilters" || !strings.Contains(sent((*calls)[2]), `"filter":"{"accountNameId":"`+chartName+`","currencyId":2,"formId":19,"selectedForms":[19]}"`) ||
		!strings.Contains(sent((*calls)[2]), `"title":"Top customers"`) {
		t.Fatal("filter actor:", sent((*calls)[2]))
	}
	if !strings.Contains(sent((*calls)[3]), `"dynamicSource":{"id":"a1","title":"Top customers","top":"5"`) {
		t.Fatal("chart source:", sent((*calls)[3]))
	}
}

func TestChartCreateRefusals(t *testing.T) {
	send, calls := consoleServer(t, nil)
	for _, args := range []string{
		`{"layerId":"` + chartLayer + `","title":"x"}`,
		`{"layerId":"` + chartLayer + `","title":"x","sourceFormId":19,"accountNameId":"` + chartName + `"}`,
		`{"layerId":"` + chartLayer + `","title":"x","top":5,"accounts":[{"actorId":"` + chartActor + `","nameId":"` + chartName + `","currencyId":1}]}`,
		`{"layerId":"` + chartLayer + `","title":"x","chartType":"line","range":"allTime","filterActorId":"` + chartActor + `"}`,
		`{"layerId":"` + chartLayer + `","title":"x","chartType":"kpiCard","filterActorId":"` + chartActor + `"}`,
	} {
		if reply := callTool(send, "chart_create", args); !strings.Contains(reply, `"isError":true`) && !strings.Contains(reply, `"error"`) {
			t.Error("accepted", args)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("invalid arguments reached the API:", methodsOf(*calls))
	}
	// A chart actor that fails on a reused filter leaves nothing behind to report.
	send, _ = consoleServer(t, map[string]string{"actor/get": `"result":{"id":"` + chartActor + `","title":"Top","formId":15,"data":{"filter":"{}"}}`,
		"actor/create": `"error":{"code":1,"message":"denied","data":{"reason":"accessDenied"}}`})
	if reply := callTool(send, "chart_create", `{"layerId":"`+chartLayer+`","title":"x","filterActorId":"`+chartActor+`"}`); !strings.Contains(reply, `"isError":true`) || strings.Contains(reply, "unused") {
		t.Error("a reused filter reported as left behind:", reply)
	}
}

func TestChartUpdate(t *testing.T) {
	source, _ := json.Marshal(`{"chartType":"bar","sourceType":"manual","yAxisLabel":"UAH","accounts":[{"actorId":"` + chartActor + `","nameId":"` + chartName + `","currencyId":1}]}`)
	chart := `"result":{"id":"` + chartActor + `","title":"Revenue","formId":7,"data":{"source":` + string(source) + `}}`
	send, calls := consoleServer(t, map[string]string{
		"actor/get":                       chart,
		"graph/layer/elements/list":       `"result":{"nodes":[{"laId":31,"id":"` + chartActor + `"}],"edges":[]}`,
		"graph/layer/actor_settings/save": `"result":{}`,
	})
	reply := callTool(send, "chart_update", `{"chartId":"`+chartActor+`","range":"lastWeek","layerId":"`+chartLayer+`","laId":31,"width":800,"height":500}`)
	if strings.Contains(reply, `"isError":true`) || methodsOf(*calls) != "actor/get,form/system/get,graph/layer/elements/list,actor/update,graph/layer/actor_settings/save" {
		t.Fatal(methodsOf(*calls), reply)
	}
	if update := sent((*calls)[3]); !strings.Contains(update, `"range":"lastWeek"`) || !strings.Contains(update, `"yAxisLabel":"UAH"`) || !strings.Contains(update, `"actorId":"`+chartActor+`"`) {
		t.Fatal("source update:", update)
	}
	if save := sent((*calls)[4]); !strings.Contains(save, `"actorId":"`+chartLayer+`","laActorId":31,"layerSettings":{"expand":true,"expandType":"chart","height":500,"width":800}`) {
		t.Fatal("placement settings:", save)
	}
	if !strings.Contains(reply, `"sourceType":"manual"`) || !strings.Contains(reply, `"range":"lastWeek"`) || !strings.Contains(reply, `"laId":31`) {
		t.Fatal(reply)
	}

	// Another actor's placement, or an actor that is not a chart, is refused before any write.
	for _, tc := range []struct{ name, method, result, want string }{
		{"another placement", "graph/layer/elements/list", `"result":{"nodes":[{"laId":31,"id":"other"}]}`, "is not a placement of the chart"},
		{"not a chart", "actor/get", `"result":{"id":"` + chartActor + `","formId":42,"data":{}}`, "is not a chart"},
	} {
		send, calls = consoleServer(t, map[string]string{"actor/get": chart, tc.method: tc.result})
		reply = callTool(send, "chart_update", `{"chartId":"`+chartActor+`","range":"lastWeek","layerId":"`+chartLayer+`","laId":31}`)
		if !strings.Contains(reply, tc.want) || strings.Contains(methodsOf(*calls), "update") || strings.Contains(methodsOf(*calls), "save") {
			t.Errorf("%s: %s after %s", tc.name, reply, methodsOf(*calls))
		}
	}
}
