package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

const blockForm = `{"sections":[{"content":[{"id":"blockId","value":"process"},{"id":"collection","value":"basic"},{"id":"view","value":"{\"shape\":{\"type\":\"path\",\"d\":\"M0 0H1\"},\"size\":{\"w\":120,\"h\":60},\"textFrame\":{\"x\":0,\"y\":0,\"w\":120,\"h\":60}}"}]}]}`

// consoleReplies are canned public results by method, shaped like sim-api answers.
var consoleReplies = map[string]string{
	"actor/create":                        `{"id":"a1","title":"Demo","formId":42}`,
	"actor/update":                        `{"id":"a1","title":"Renamed","formId":42}`,
	"form/system/get":                     `{"id":7,"title":"Layers","type":"system","form":{}}`,
	"form/system/list":                    `{"list":[{"id":5,"title":"FlowchartBlock","type":"system"},{"id":6,"title":"Process","type":"system","parentId":5},{"id":8,"title":"Events","type":"system"}]}`,
	"form/get":                            `{"id":6,"title":"Process","parentId":5,"updatedAt":10,"form":` + blockForm + `}`,
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
		if r.Header.Get("Authorization") != "Simulator user-jwt" || q.Params["accId"] != "ws" {
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
	want := "actor_create,actor_get,actor_update,edge_create,flowchart_blocks_list,layer_create,layer_get,layer_placements_create,layer_positions_update,smart_form_create,smart_form_files_create,smart_form_files_get,smart_form_files_update,smart_form_release,smart_form_release_get"
	if strings.Join(names, ",") != want {
		t.Fatal("console tools:", names)
	}
	for _, name := range []string{"actor_get", "flowchart_blocks_list", "layer_get", "smart_form_files_get", "smart_form_release_get"} {
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
		{"actor_update", `{"actorId":"a1","title":"Renamed"}`, "actor/update", `"actorId":"a1","fields":["id","title","description","formId","status","ref"],"title":"Renamed"`, `"title":"Renamed"`},
		{"flowchart_blocks_list", `{}`, "form/system/list,form/get", `"formId":6`, `"list":[{"formId":5,"title":"Process","blockId":"process","collection":"basic"}]`},
		{"flowchart_blocks_list", `{"blockIds":["process"]}`, "form/system/list,form/get", `"formId":6`, `"layerSettings":{"width":120,"height":60,"blockId":"process"`},
		{"flowchart_blocks_list", `{"query":"decision"}`, "form/system/list", `"accId":"ws"`, `"list":[]`},
		{"layer_create", `{"title":"Flow"}`, "form/system/get,actor/create", `"data":{},"fields"`, `"id":"a1"`},
		{"edge_create", `{"source":"a1","target":"a2","name":"yes"}`, "edge_type/get,edge/create", `"edgeTypeId":3,"forceDirection":true,"name":"yes","source":"a1","target":"a2"`, `"id":"e1"`},
		{"layer_placements_create", `{"layerId":"L","nodes":[{"actorId":"a1","position":{"x":0,"y":0}}]}`, "graph/layer/actors/manage", `"actorId":"L","items":[{"action":"create","data":{"id":"a1","position":{"x":0,"y":0},"type":"node"}}]`, `"laId":11`},
		{"layer_positions_update", `{"layerId":"L","items":[{"laId":11,"x":150,"y":0}]}`, "graph/layer/actors/positions/update", `"items":[{"laActorId":11,"position":{"x":150,"y":0}}]`, `"updated":1`},
		{"layer_get", `{"layerId":"L"}`, "graph/layer/edges/list,graph/layer/actors/list", `"fields":["id","title","formId"],"limit":200`, `"nodes":[{"laId":11,"actorId":"a1","title":"Demo","formId":5,"position":{"x":0,"y":0},"blockId":"process"}],"edges":[{"laId":21,"edgeId":"e1","laIdSource":11,"laIdTarget":12,"source":"a1","target":"a2","name":"yes"}]`},
		{"smart_form_create", `{"ref":"leave_form","title":"Leave request","sharedWith":"allWorkspaceUsers"}`, "application/create", `"cduVersion":2,"ref":"leave_form","sharedWith":"allWorkspaceUsers"`, `"appId":"app1","ref":"leave_form","developEnvId":1,"productionEnvId":2`},
		{"smart_form_files_get", `{"appId":"app1"}`, "application/env/list,application/struct", `"envId":1,"fields":["id","title","objType","type","sourceHash","children"]`, `{"id":103,"path":"pages/index/config","objType":"file","type":"application/json","sourceHash":"h"}`},
		{"smart_form_files_update", `{"appId":"app1","items":[{"id":103,"title":"config","type":"application/json","source":"{}"}]}`, "app_file/update", `"actorId":"app1","items":[{"id":103,"source":"{}","title":"config","type":"application/json"}]`, `"sourceHash":"h2"`},
		{"smart_form_files_create", `{"appId":"app1","items":[{"folderId":102,"title":"locale","type":"application/json","source":"{}"}]}`, "app_file/create", `"items":[{"folderId":102`, `"id":104`},
		{"smart_form_release", `{"appId":"app1"}`, "application/env/list,release/create", `"sourceEnvId":1,"targetEnvId":2`, `{"appId":"app1","releaseId":9,"releaseNumber":1,"status":"active","ready":true}`},
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
		{"no flowchart catalog", "flowchart_blocks_list", `{}`, map[string]string{"form/system/list": `"result":{"list":[]}`}, "FlowchartBlock system template not found"},
		{"updated actor must be the requested one", "actor_update", `{"actorId":"a9","title":"x"}`, nil, "identity mismatch"},
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
