package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

const blockForm = `{"sections":[{"content":[{"id":"blockId","value":"process"},{"id":"collection","value":"basic"},{"id":"view","value":"{\"shape\":{\"type\":\"path\",\"d\":\"M0 0H1\"},\"size\":{\"w\":200,\"h\":100},\"textFrame\":{\"x\":0,\"y\":0,\"w\":200,\"h\":100}}"}]}]}`

// consoleReplies are canned public results by method, shaped like sim-api answers.
var consoleReplies = map[string]string{
	"actor/create":                        `{"id":"a1","title":"Demo","formId":42}`,
	"actor/update":                        `{"id":"a1","title":"Renamed","formId":42}`,
	"form/system/get":                     `{"id":7,"title":"Layers","type":"system","form":{}}`,
	"form/flowchart-block/search":         `{"list":[{"id":6,"parentId":5,"title":"Process","blockId":"process","collection":"basic","updatedAt":10,"form":` + blockForm + `}],"hasMore":false}`,
	"edge_type/get":                       `{"id":3,"name":"hierarchy"}`,
	"edge/create":                         `{"id":"e1","edgeTypeId":3,"source":"a1","target":"a2","name":"yes"}`,
	"graph/layer/actors/manage":           `{"nodes":[{"laId":11,"actorId":"a1","position":{"x":0,"y":0}}],"edges":[]}`,
	"graph/layer/actors/positions/update": `{}`,
	"graph/layer/actors/list":             `{"list":[{"laId":11,"id":"a1","position":{"x":0,"y":0},"layerSettings":{"blockId":"process"},"actor":{"id":"a1","title":"Demo","formId":5}}]}`,
	"graph/layer/edges/list":              `{"list":[{"laId":21,"id":"e1","laIdSource":11,"laIdTarget":12,"edge":{"id":"e1","edgeTypeId":3,"source":"a1","target":"a2","name":"yes"}}]}`,
	"application/create":                  `{"application":{"id":"app1","ref":"leave_form"},"envs":[{"id":1,"actorId":"app1","title":"develop"},{"id":2,"actorId":"app1","title":"production","readonly":true}]}`,
	"application/env/list":                `{"canModify":true,"envs":[{"id":1,"actorId":"app1","title":"develop","rootFolderId":100},{"id":2,"actorId":"app1","title":"production","readonly":true}]}`,
	"application/struct":                  `{"tree":{"id":100,"objType":"folder","title":"root","children":[{"id":101,"objType":"folder","title":"pages","children":[{"id":102,"objType":"folder","title":"index","children":[{"id":103,"objType":"file","title":"config","type":"application/json","sourceHash":"h"}]}]}]}}`,
	"app_file/update":                     `{"files":[{"id":103,"folderId":102,"objType":"file","title":"config","type":"application/json","source":"{}","sourceHash":"h2"}]}`,
	"app_file/create":                     `{"files":[{"id":104,"folderId":102,"objType":"file","title":"locale","type":"application/json","source":"{}","sourceHash":"h3"}]}`,
	"release/create":                      `{"id":9,"actorId":"app1","envId":2,"sourceEnvId":1,"releaseNumber":1,"status":"active"}`,
	"release/get":                         `{"id":9,"actorId":"app1","envId":2,"sourceEnvId":1,"releaseNumber":1,"status":"archived"}`,
}

type consoleCall struct {
	Method string
	Params map[string]any
}

// consoleServer answers with consoleReplies; an override is a full JSON-RPC result or error member.
func consoleServer(t *testing.T, overrides map[string]string) (func(body string) string, *[]consoleCall) {
	calls := &[]consoleCall{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if r.Header.Get("Authorization") != "Simulator user-jwt" || (q.Params["accId"] != "ws" && q.Method != "batch") {
			t.Errorf("%s sent without the host credential or workspace", q.Method)
		}
		*calls = append(*calls, consoleCall{q.Method, q.Params})
		if member, ok := overrides[q.Method]; ok {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,%s}`, q.ID, member)
			return
		}
		reply, ok := consoleReplies[q.Method]
		if !ok {
			t.Errorf("unexpected method %s", q.Method)
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, reply)
	}))
	t.Cleanup(api.Close)
	c, e := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator user-jwt"})
	if e != nil {
		t.Fatal(e)
	}
	s := New(c)
	id := 0
	send := func(body string) string {
		id++
		reply, e := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,%s}`, id, body))))
		if e != nil {
			t.Fatal(e)
		}
		return string(reply)
	}
	send(`"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}`)
	return send, calls
}

func TestConsoleCatalog(t *testing.T) {
	send, _ := consoleServer(t, nil)
	var list struct {
		Result struct {
			Tools []struct {
				Name        string          `json:"name"`
				Annotations map[string]any  `json:"annotations"`
				Output      json.RawMessage `json:"outputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if e := json.Unmarshal([]byte(send(`"method":"tools/list"`)), &list); e != nil {
		t.Fatal(e)
	}
	var names []string
	reads := map[string]bool{}
	for _, tool := range list.Result.Tools {
		names = append(names, tool.Name)
		reads[tool.Name] = tool.Annotations["readOnlyHint"] == true
		if strings.Contains(string(tool.Output), `"minimum":0`) {
			t.Errorf("%s output schema types open JSON as bytes", tool.Name)
		}
	}
	// The exact console catalog: its own tools plus every actor, form and graph parity tool.
	want := "actor_by_ref_get,actor_create,actor_delete,actor_get,actor_status_update,actor_system_get,actor_update,actors_create,actors_filter,actors_filter_count,actors_search," +
		"edge_create,edge_delete,edge_endpoints_update,edge_exist,edge_get,edge_types_list,edge_update,edges_by_nodes_delete,edges_create,flowchart_blocks_list,flowchart_layout," +
		"form_account_create,form_account_delete,form_accounts_list,form_create,form_delete,form_get,form_linked_get,form_status_update,form_update,forms_count,forms_list,forms_tree_get," +
		"graph_get,graph_list,layer_actors_move,layer_actors_search,layer_clean,layer_compact,layer_create,layer_edges_list,layer_element_exist,layer_get,layer_placements_all," +
		"layer_placements_create,layer_placements_list,layer_placements_manage,layer_positions_update,layer_prune_long_edges,layer_skeleton_get,layer_stats_get,search_all," +
		"smart_form_components,smart_form_create,smart_form_files_create,smart_form_files_get,smart_form_files_update,smart_form_release,smart_form_release_get"
	sort.Strings(names)
	if got := strings.Join(names, ","); got != want || len(names) != 60 {
		t.Fatalf("console catalog has %d tools:\n%s", len(names), got)
	}
	for _, name := range []string{"actor_get", "smart_form_components", "flowchart_blocks_list", "flowchart_layout", "layer_get", "smart_form_files_get", "smart_form_release_get"} {
		if !reads[name] {
			t.Errorf("%s must be read-only", name)
		}
	}
	if reads["actor_create"] || reads["smart_form_release"] {
		t.Error("writes must not be read-only")
	}
}

func TestConsoleRequests(t *testing.T) {
	cases := []struct {
		tool, args string
		methods    string
		wire       string // substring of the last request's params
		out        string // substring of the structured result
	}{
		{"actor_create", `{"formId":5,"title":"Start","color":"#17B26A","data":{"__form__6:blockId":"process"}}`, "actor/create", `"color":"#17B26A","data":{"__form__6:blockId":"process"}`, `"id":"a1"`},
		{"actor_update", `{"actorId":"a1","title":"Renamed"}`, "actor/update", `"actorId":"a1","fields":["id","title","description","formId","status","ref","color"],"title":"Renamed"`, `"title":"Renamed"`},
		{"flowchart_blocks_list", `{"query":" proc ","offset":100}`, "form/flowchart-block/search", `"accId":"ws","limit":100,"offset":100,"query":"proc"`, `"list":[{"formId":5,"title":"Process","blockId":"process","collection":"basic"}],"hasMore":false`},
		{"flowchart_blocks_list", `{"blockIds":["process"]}`, "form/flowchart-block/search", `"blockIds":["process"]`, `"layerSettings":{"width":200,"height":100,"blockId":"process"`},
		{"layer_create", `{"title":"Flow"}`, "form/system/get,actor/create", `"data":{},"fields"`, `"id":"a1"`},
		{"edge_create", `{"source":"a1","target":"a2","name":"yes"}`, "edge_type/get,edge/create", `"edgeTypeId":3,"forceDirection":true,"name":"yes","source":"a1","target":"a2"`, `"id":"e1"`},
		{"layer_placements_create", `{"layerId":"L","nodes":[{"actorId":"a1","position":{"x":0,"y":0}}]}`, "graph/layer/actors/manage", `"actorId":"L","items":[{"action":"create","data":{"id":"a1","position":{"x":0,"y":0},"type":"node"}}]`, `"laId":11`},
		{"layer_placements_create", `{"layerId":"L","nodes":[{"actorId":"a1","color":"#539fdf","position":{"x":10,"y":10},"layerSettings":{"width":200,"height":100,"blockId":"process","shape":{"type":"path","d":"M0 0H1"},"textFrame":{"x":0,"y":0,"w":200,"h":100}}}]}`, "graph/layer/actors/manage",
			`"data":{"areaPicture":{"height":100,"img":"data:image/svg+xml;base64,`, `"laId":11`},
		{"layer_placements_create", `{"layerId":"L","nodes":[{"actorId":"a1","position":{"x":10,"y":10},"layerSettings":{"width":200,"height":100,"blockId":"process","shape":{"type":"path","d":"M0 0H1"},"textFrame":{"x":0,"y":0,"w":200,"h":100}}}]}`, "graph/layer/actors/manage",
			`"position":{"x":25,"y":25},"type":"node"`, `"laId":11`},
		{"flowchart_layout", `{"nodes":[{"key":"a","blockId":"process"},{"key":"b","blockId":"process"}],"edges":[{"source":"a","target":"b"},{"source":"b","target":"a"}]}`, "form/flowchart-block/search", `"blockIds":["process"]`,
			`"key":"b","blockId":"process","formId":5,"data":{"__form__6:blockId":"process"`},
		{"flowchart_layout", `{"nodes":[{"key":"a","blockId":"process"},{"key":"b","blockId":"process"}],"edges":[{"source":"a","target":"b"},{"source":"b","target":"a"}]}`, "form/flowchart-block/search", `"limit":100`,
			`"position":{"x":25,"y":325},"layerSettings":{"width":200,"height":100,"blockId":"process"`},
		{"layer_positions_update", `{"layerId":"L","items":[{"laId":11,"x":150,"y":0}]}`, "graph/layer/actors/positions/update", `"items":[{"laActorId":11,"position":{"x":150,"y":0}}]`, `"updated":1`},
		{"layer_get", `{"layerId":"L"}`, "graph/layer/edges/list,graph/layer/actors/list", `"fields":["id","title","formId"],"limit":200`, `"nodes":[{"laId":11,"actorId":"a1","title":"Demo","formId":5,"position":{"x":0,"y":0},"blockId":"process"}],"edges":[{"laId":21,"edgeId":"e1","laIdSource":11,"laIdTarget":12,"source":"a1","target":"a2","name":"yes"}]`},
		{"smart_form_create", `{"ref":"leave_form","title":"Leave request","sharedWith":"allWorkspaceUsers"}`, "application/create", `"cduVersion":2,"ref":"leave_form","sharedWith":"allWorkspaceUsers"`, `"appId":"app1","ref":"leave_form","developEnvId":1,"productionEnvId":2`},
		{"smart_form_files_get", `{"appId":"app1"}`, "application/env/list,application/struct", `"envId":1,"fields":["id","title","objType","type","sourceHash","children"]`, `{"id":103,"path":"pages/index/config","objType":"file","type":"application/json","sourceHash":"h"}`},
		{"smart_form_files_update", `{"appId":"app1","items":[{"id":103,"title":"config","type":"application/json","source":"{}"}]}`, "app_file/update", `"actorId":"app1","items":[{"id":103,"source":"{}","title":"config","type":"application/json"}]`, `"sourceHash":"h2"`},
		{"smart_form_files_create", `{"appId":"app1","items":[{"folderId":102,"title":"locale","type":"application/json","source":"{}"}]}`, "app_file/create", `"items":[{"folderId":102`, `"id":104`},
		{"smart_form_release", `{"appId":"app1"}`, "application/env/list,release/create", `"sourceEnvId":1,"targetEnvId":2`, `{"appId":"app1","releaseId":9,"releaseNumber":1,"status":"active","ready":true,"marker":"appId: app1"}`},
		{"smart_form_release_get", `{"appId":"app1","releaseId":9}`, "application/env/list,release/get", `"releaseId":9`, `"ready":false`},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			send, calls := consoleServer(t, nil)
			reply := send(`"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":` + tc.args + `}`)
			var r struct {
				Result struct {
					IsError    bool            `json:"isError"`
					Structured json.RawMessage `json:"structuredContent"`
				} `json:"result"`
			}
			if e := json.Unmarshal([]byte(reply), &r); e != nil || r.Result.IsError {
				t.Fatal(reply)
			}
			var methods []string
			for _, c := range *calls {
				methods = append(methods, c.Method)
			}
			if strings.Join(methods, ",") != tc.methods {
				t.Fatal("methods:", methods)
			}
			last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
			if !strings.Contains(string(last), tc.wire) {
				t.Fatalf("request %s lacks %s", last, tc.wire)
			}
			if !strings.Contains(string(r.Result.Structured), tc.out) {
				t.Fatalf("result %s lacks %s", r.Result.Structured, tc.out)
			}
		})
	}
}

func TestConsoleRefusesBadArguments(t *testing.T) {
	send, calls := consoleServer(t, nil)
	for _, args := range []string{
		`"name":"edge_create","arguments":{"source":"a1"}`,
		`"name":"layer_placements_create","arguments":{"layerId":"L"}`,
		`"name":"smart_form_create","arguments":{"ref":"bad ref","title":"x"}`,
		`"name":"actor_create","arguments":{"formId":5,"title":"x","data":[1]}`,
	} {
		if reply := send(`"method":"tools/call","params":{` + args + `}`); !strings.Contains(reply, `"isError":true`) && !strings.Contains(reply, `"error"`) {
			t.Error("accepted", args, reply)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("invalid arguments reached the API:", *calls)
	}
}

func TestConsoleFailures(t *testing.T) {
	cases := []struct {
		name, tool, args string
		overrides        map[string]string
		want             string
	}{
		{"server validation text reaches the agent", "smart_form_files_update", `{"appId":"app1","items":[{"id":103,"title":"config","type":"application/json","source":"{}"}]}`,
			map[string]string{"app_file/update": `"error":{"code":-32602,"message":"invalid params: invalid CDU source in page 'index': unknown component type \"Buton\" at /tree/children/0","data":{"reason":"invalidParams"}}`},
			`reason=invalidParams http=0): invalid params: invalid CDU source in page 'index': unknown component type`},
		{"route issues are included", "smart_form_release", `{"appId":"app1"}`,
			map[string]string{"release/create": `"error":{"code":1,"message":"invalid routes","data":{"reason":"invalidRoutes","reasonParams":{"issues":[{"code":"duplicatePath","pageId":"index"}]}}}`},
			`invalid routes {"issues":[{"code":"duplicatePath","pageId":"index"}]}`},
		{"reverse edge is not reported as the new one", "edge_create", `{"source":"a2","target":"a1","name":"retry"}`, nil, "other endpoints"},
		{"develop release is not ready production", "smart_form_release_get", `{"appId":"app1","releaseId":9}`,
			map[string]string{"release/get": `"result":{"id":9,"actorId":"app1","envId":1,"sourceEnvId":2,"releaseNumber":2,"status":"active"}`}, "not a production release"},
		{"catalog page must be well formed", "flowchart_blocks_list", `{}`, map[string]string{"form/flowchart-block/search": `"result":{"list":[]}`}, "invalid native block catalog page"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			send, _ := consoleServer(t, tc.overrides)
			reply := send(`"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":` + tc.args + `}`)
			var r struct {
				Result struct {
					IsError bool `json:"isError"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"result"`
			}
			if e := json.Unmarshal([]byte(reply), &r); e != nil || !r.Result.IsError || len(r.Result.Content) != 1 || !strings.Contains(r.Result.Content[0].Text, tc.want) {
				t.Fatal(reply)
			}
		})
	}
}

func TestConsoleBatches(t *testing.T) {
	actors := `"result":{"isAtomic":true,"list":[{"id":"0","result":{"id":"a1","title":"Start","formId":5}},{"id":"1","result":{"id":"a2","title":"End","formId":5}}]}`
	edges := `"result":{"isAtomic":true,"list":[{"id":"0","result":{"id":"e1","edgeTypeId":3,"source":"a1","target":"a2","name":"go"}},{"id":"1","result":{"id":"e2","edgeTypeId":3,"source":"a2","target":"a1"}}]}`
	failed := `"result":{"isAtomic":true,"rolledBack":true,"list":[{"id":"0","result":{"id":"a1","formId":5}},{"id":"1","error":{"code":-32602,"message":"invalid params: formId","data":{"reason":"invalidParams"}}}]}`
	for _, tc := range []struct {
		name, tool, args, batch, wire, want string
		fails                               bool
	}{
		{"actors in order", "actors_create", `{"actors":[{"formId":5,"title":"Start","color":"#17B26A","data":{"k":1}},{"formId":5,"title":"End"}]}`, actors,
			`"isAtomic":true,"list":[{"id":"0","method":"actor/create","params":{"accId":"ws","color":"#17B26A","data":{"k":1}`, `"actors":[{"formId":5,"id":"a1","title":"Start"},{"formId":5,"id":"a2","title":"End"}]`, false},
		{"edges keep direction", "edges_create", `{"edges":[{"source":"a1","target":"a2","name":"go"},{"source":"a2","target":"a1"}]}`, edges,
			`"method":"edge/create","params":{"accId":"ws","edgeTypeId":3,"forceDirection":true,"name":"go","source":"a1","target":"a2"}`, `"edges":[{"id":"e1","edgeTypeId":3,"source":"a1","target":"a2","name":"go"},{"id":"e2"`, false},
		{"a failed item fails the call", "actors_create", `{"actors":[{"formId":5,"title":"A"},{"formId":9,"title":"B"}]}`, failed, `"isAtomic":true`, "item 1 failed, nothing was created", true},
		{"reverse edge is not reused", "edges_create", `{"edges":[{"source":"a1","target":"a2"},{"source":"a1","target":"a2"}]}`, edges, `"isAtomic":true`, "edge 1: batch committed but edge/create returned an edge with other endpoints", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			send, calls := consoleServer(t, map[string]string{"batch": tc.batch})
			reply := send(`"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":` + tc.args + `}`)
			last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
			if (*calls)[len(*calls)-1].Method != "batch" || !strings.Contains(string(last), tc.wire) {
				t.Fatalf("batch request %s lacks %s", last, tc.wire)
			}
			if strings.Contains(reply, `"isError":true`) != tc.fails || !strings.Contains(strings.ReplaceAll(reply, `\"`, `"`), tc.want) {
				t.Fatalf("reply %s, want %s", reply, tc.want)
			}
		})
	}
}

func TestSmartFormComponents(t *testing.T) {
	send, calls := consoleServer(t, nil)
	text := func(args string) string {
		var r struct {
			Result struct {
				Content    []struct{ Text string } `json:"content"`
				Structured json.RawMessage         `json:"structuredContent"`
			} `json:"result"`
		}
		_ = json.Unmarshal([]byte(send(`"method":"tools/call","params":{"name":"smart_form_components","arguments":`+args+`}`)), &r)
		if len(r.Result.Structured) > 0 {
			return string(r.Result.Structured)
		}
		if len(r.Result.Content) > 0 {
			return r.Result.Content[0].Text
		}
		return ""
	}
	list := text(`{}`)
	if strings.Count(list, `"category":`) != 97 || strings.Contains(list, `"props"`) || !strings.Contains(list, `"type":"DatePicker"`) {
		t.Fatal("component list:", list[:300])
	}
	detail := text(`{"types":["TextInput","Button"]}`)
	if !strings.Contains(detail, `"label":{"description":"Label for the input","type":"string"}`) || !strings.Contains(detail, `"title":`) || strings.Count(detail, `"category":`) != 2 {
		t.Fatal("component detail:", detail[:300])
	}
	if unknown := text(`{"types":["DateInput"]}`); !strings.Contains(unknown, "unknown CDU component types [DateInput]") {
		t.Fatal(unknown)
	}
	if len(*calls) != 0 {
		t.Fatal("the component reference called the API")
	}
}

func TestConsoleBatchRequiresAtomicBackend(t *testing.T) {
	send, _ := consoleServer(t, map[string]string{"batch": `"result":{"list":[{"id":"0","result":{"id":"a1","formId":5}},{"id":"1","error":{"code":1,"message":"x","data":{"reason":"notFound"}}}]}`})
	reply := send(`"method":"tools/call","params":{"name":"actors_create","arguments":{"actors":[{"formId":5,"title":"A"},{"formId":6,"title":"B"}]}}`)
	if !strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, "did not run the batch atomically") {
		t.Fatal(reply)
	}
}
