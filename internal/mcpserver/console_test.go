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
	"batch":                               `{"isAtomic":false,"list":[{"id":"0","result":{"id":"a1","color":"#17b26a"}}]}`,
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
		if r.Header.Get("Authorization") != "Simulator user-jwt" || (q.Params["accId"] != "ws" && q.Method != "batch" && q.Method != "route/build") {
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
	c, e := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator user-jwt"})
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
	// The exact console catalog: its own tools plus every actor, form, graph, access, reaction, user,
	// attachment, finance, registry, chart, simulation, Corezoid, graph export/import, guest link and transcript tool.
	want := "access_request_cancel,access_request_confirm,access_request_create,access_requests_list,access_rules_get,access_rules_save,account_actor_tags_count," +
		"account_actor_tags_list,account_actor_triggers_list,account_actor_triggers_set,account_amount_set,account_by_id_get,account_by_ref_create,account_by_ref_get," +
		"account_create,account_delete,account_formula_get,account_formula_preview,account_formula_set,account_get,account_name_create,account_name_get," +
		"account_name_update,account_names_list,account_pair_create,account_pair_unregister,account_status_update,account_tag_add,account_tags_list,account_tags_set," +
		"account_transactions_list,account_triggers_list,account_triggers_set,account_update,accounts_by_ref_list,accounts_count,accounts_list,actor_attach,actor_by_ref_get," +
		"actor_corezoid_processes_list,actor_create,actor_delete,actor_field_triggers_list,actor_field_triggers_set,actor_get,actor_picture_upload,actor_status_update," +
		"actor_system_get,actor_update,actors_create,actors_filter,actors_filter_count,actors_search,agent_get,agents_find,atom_transaction_create,attachment_attach," +
		"attachment_detach,attachment_read,attachment_update,attachment_upload,attachments_by_actor_list,attachments_list,bbcode_tags_get,build_link,chart_create," +
		"chart_update,chat_open,corezoid_processes_list,corezoid_project_bind,corezoid_project_create,corezoid_project_get,corezoid_projects_list,corezoid_stages_list," +
		"counter_by_period_get,counter_get,counter_increment,counter_set,currencies_list,currency_create,currency_get,currency_update,edge_create,edge_delete," +
		"edge_endpoints_update,edge_exist,edge_get,edge_types_list,edge_update,edges_by_nodes_delete,edges_create,flowchart_blocks_list,flowchart_layout," +
		"form_account_create,form_account_delete,form_accounts_list,form_create,form_delete,form_get,form_linked_get,form_status_update,form_update,forms_count," +
		"forms_list,forms_tree_get,graph_export_create,graph_file_upload,graph_get,graph_import_create,graph_list,layer_actors_move,layer_actors_search,layer_clean," +
		"layer_compact,layer_create,layer_edge_settings_save,layer_edges_list,layer_element_exist,layer_get,layer_placements_all,layer_placements_create," +
		"layer_placements_list,layer_placements_manage,layer_positions_update,layer_prune_long_edges,layer_skeleton_get,layer_stats_get,meet_transcription_list," +
		"public_link_create,public_link_get,public_link_revoke,reaction_create,reaction_delete,reaction_update,reactions_count,reactions_list,reactions_mark_read," +
		"search_all,simulation_check,simulation_run,simulation_snapshot_get,skill_get,skills_find,smart_form_components,smart_form_create,smart_form_data_form_create," +
		"smart_form_file_rollback,smart_form_file_version_get,smart_form_file_versions_list,smart_form_files_create,smart_form_files_get,smart_form_files_update," +
		"smart_form_folders_create,smart_form_page_render,smart_form_page_submit,smart_form_release,smart_form_release_diff,smart_form_release_get," +
		"smart_form_release_rollback,smart_form_releases_list,smart_form_trash_list,smart_form_trash_restore,task_get,transaction_by_actor_ref_create," +
		"transaction_by_ref_get,transaction_children_list,transaction_create,transaction_get,transactions_filter,transactions_filter_count,transactions_list," +
		"transfer_by_ref_get,transfer_create,transfer_get,transfers_filter,transfers_filter_count,user_get,users_search,workspaces_list"
	sort.Strings(names)
	if got := strings.Join(names, ","); got != want || len(names) != 179 {
		t.Fatalf("console catalog has %d tools:\n%s", len(names), got)
	}
	for _, name := range []string{"actor_get", "smart_form_components", "flowchart_blocks_list", "flowchart_layout", "layer_get", "smart_form_files_get", "smart_form_release_get", "access_rules_get", "attachment_read", "reactions_list", "users_search", "skills_find", "skill_get", "agents_find", "agent_get", "build_link", "bbcode_tags_get", "simulation_check", "simulation_run", "simulation_snapshot_get"} {
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
		{"layer_placements_create", `{"layerId":"L","nodes":[{"actorId":"a1","position":{"x":10,"y":10},"layerSettings":{"width":200,"height":100,"blockId":"process","shape":{"type":"path","d":"M0 0H1"},"textFrame":{"x":0,"y":0,"w":200,"h":100}}}]}`, "batch,graph/layer/actors/manage",
			`"position":{"x":25,"y":25},"type":"node"`, `"laId":11`},
		{"flowchart_layout", `{"nodes":[{"key":"a","blockId":"process"},{"key":"b","blockId":"process"}],"edges":[{"source":"a","target":"b"},{"source":"b","target":"a"}]}`, "form/flowchart-block/search", `"blockIds":["process"]`,
			`"key":"b","blockId":"process","formId":5,"color":"#`},
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
	send, calls := consoleServer(t, map[string]string{"cdu/catalog/get": `"result":{"schemaVersion":"2.0","list":[{"type":"TextInput","kind":"primitive","category":"Input","label":"Text Input"}]}`})
	reply := send(`"method":"tools/call","params":{"name":"smart_form_components","arguments":{}}`)
	if strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, "TextInput") {
		t.Fatal(reply)
	}
	last := (*calls)[len(*calls)-1]
	if last.Method != "cdu/catalog/get" || last.Params["brief"] != true || last.Params["accId"] != "ws" {
		t.Fatal("cdu/catalog/get request:", last)
	}
	send(`"method":"tools/call","params":{"name":"smart_form_components","arguments":{"types":["DatePicker","Button"]}}`)
	if p := (*calls)[len(*calls)-1].Params; p["brief"] != nil || fmt.Sprint(p["types"]) != "[DatePicker Button]" {
		t.Fatal("a types request must not be brief:", p)
	}
	if r := send(`"method":"tools/call","params":{"name":"smart_form_components","arguments":{"kind":"widget"}}`); !strings.Contains(r, `"isError":true`) && !strings.Contains(r, `"error"`) {
		t.Fatal("an unknown kind reached the API:", r)
	}
}

func TestConsoleBatchRequiresAtomicBackend(t *testing.T) {
	send, _ := consoleServer(t, map[string]string{"batch": `"result":{"list":[{"id":"0","result":{"id":"a1","formId":5}},{"id":"1","error":{"code":1,"message":"x","data":{"reason":"notFound"}}}]}`})
	reply := send(`"method":"tools/call","params":{"name":"actors_create","arguments":{"actors":[{"formId":5,"title":"A"},{"formId":6,"title":"B"}]}}`)
	if !strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, "did not run the batch atomically") {
		t.Fatal(reply)
	}
}

func TestConsoleCoreCatalog(t *testing.T) {
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range New(c).ListTools() {
		names = append(names, name)
		if !consoleCoreTools[name] {
			t.Error("console has", name)
		}
	}
	if len(names) != len(consoleCoreTools) || len(names) != 47 {
		t.Fatalf("console has %d tools, want the %d core ones: %v", len(names), len(consoleCoreTools), names)
	}
}

func inPalette(color string) bool {
	for _, h := range publicapi.ActorHues {
		if color == h {
			return true
		}
	}
	return false
}

func TestConsoleColorsAndHoles(t *testing.T) {
	batch := `"result":{"isAtomic":true,"list":[{"id":"0","result":{"id":"a1","formId":5,"color":"#2970ff"}},{"id":"1","result":{"id":"h1","formId":5,"color":"#17b26a","hole":true}}]}`
	send, calls := consoleServer(t, map[string]string{"batch": batch})
	reply := send(`"method":"tools/call","params":{"name":"actors_create","arguments":{"actors":[{"formId":5,"title":"A"},{"formId":5,"title":"Expected","hole":true,"data":{"city":"Kyiv"}}]}}`)
	if strings.Contains(reply, `"isError":true`) {
		t.Fatal(reply)
	}
	var req struct {
		List []struct {
			Params struct {
				Color string `json:"color"`
				Hole  bool   `json:"hole"`
			} `json:"params"`
		} `json:"list"`
	}
	raw, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	if err := json.Unmarshal(raw, &req); err != nil || len(req.List) != 2 {
		t.Fatal(string(raw))
	}
	for i, item := range req.List {
		if !inPalette(item.Params.Color) {
			t.Errorf("actor %d got colour %q outside the palette", i, item.Params.Color)
		}
	}
	if req.List[0].Params.Hole || !req.List[1].Params.Hole {
		t.Fatal("hole not passed per item:", string(raw))
	}

	send(`"method":"tools/call","params":{"name":"actor_create","arguments":{"formId":5,"title":"One"}}`)
	if c, _ := (*calls)[len(*calls)-1].Params["color"].(string); !inPalette(c) {
		t.Fatal("actor_create default colour:", c)
	}
	send(`"method":"tools/call","params":{"name":"actor_create","arguments":{"formId":5,"title":"One","color":"#000000"}}`)
	if c, _ := (*calls)[len(*calls)-1].Params["color"].(string); c != "#000000" {
		t.Fatal("an explicit colour was replaced:", c)
	}
}

func TestLayerGetShowsHoles(t *testing.T) {
	send, _ := consoleServer(t, map[string]string{
		"graph/layer/actors/list": `"result":{"list":[{"laId":1,"id":"h1","hole":true,"position":{"x":0,"y":0},"actor":{"id":"h1","title":"Expected"}},{"laId":2,"id":"a2","position":{"x":0,"y":200},"closedHole":{"holeActorId":"h0","closingProgress":{"matched":1,"total":2}},"actor":{"id":"a2","title":"Closer"}}]}`,
		"graph/layer/edges/list":  `"result":{"list":[{"laId":9,"id":"e1","laIdSource":2,"laIdTarget":1,"edge":{"id":"e1","edgeTypeId":3,"source":"a2","target":"h1","hole":true},"closedHole":{"holeEdgeId":"e0"}}]}`,
	})
	reply := strings.ReplaceAll(send(`"method":"tools/call","params":{"name":"layer_get","arguments":{"layerId":"L"}}`), `\"`, `"`)
	for _, want := range []string{`"actorId":"h1","title":"Expected","position":{"x":0,"y":0},"hole":true`, `"closedHole":{"holeActorId":"h0","closingProgress":{"matched":1,"total":2}}`, `"target":"h1","hole":true,"closedHole":"e0"`} {
		if !strings.Contains(reply, want) {
			t.Errorf("layer_get lacks %s: %s", want, reply)
		}
	}
}

// The people tools reach their public methods through the MCP layer: a parity tool narrowed to its
// legacy params, and the access tool's object dispatch.
func TestConsolePeopleTools(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{
		"user/list":                        `"result":{"list":[{"id":7,"nick":"Olena"}]}`,
		"access_rule/actor/share":          `"result":{"list":[]}`,
		"access_rule/formTemplate/list":    `"result":{"list":[]}`,
		"access_rule/actor/request/create": `"result":{"invite":{"id":5,"hash":"invite-secret","status":"pending"}}`,
	})
	for _, tc := range []struct{ tool, args, method, sent string }{
		{"users_search", `{"query":"Olena"}`, "user/list", `"query":"Olena"`},
		{"access_rules_save", `{"objType":"actor","objId":"a1","rules":[{"userId":7,"privs":{"view":true,"execute":true}}]}`, "access_rule/actor/share", `"actions":[{"action":"create","privs":{"execute":true,"view":true},"userId":7}]`},
		{"access_rules_get", `{"objType":"formTemplate","objId":"42"}`, "access_rule/formTemplate/list", `"formId":42`},
	} {
		reply := send(fmt.Sprintf(`"method":"tools/call","params":{"name":%q,"arguments":%s}`, tc.tool, tc.args))
		last := (*calls)[len(*calls)-1]
		sent, _ := json.Marshal(last.Params)
		if strings.Contains(reply, `"isError":true`) || last.Method != tc.method || !strings.Contains(string(sent), tc.sent) {
			t.Errorf("%s: %s called %s %s", tc.tool, reply, last.Method, sent)
		}
	}
	// The invite's hash accepts it in legacy: it never reaches the model.
	if reply := send(`"method":"tools/call","params":{"name":"access_request_create","arguments":{"actorId":"a9"}}`); strings.Contains(reply, "invite-secret") || !strings.Contains(reply, `\"id\":5`) {
		t.Error("access_request_create answered", reply)
	}
	if reply := send(`"method":"tools/call","params":{"name":"users_search","arguments":{"query":"x","rootActorId":"a"}}`); !strings.Contains(reply, `"isError":true`) && !strings.Contains(reply, `"error"`) {
		t.Error("users_search took a param outside its legacy set:", reply)
	}
}

// The owner's Console rules ride in the core tools' descriptions, which the model sees every turn.
func TestConsoleRulesInDescriptions(t *testing.T) {
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	tools := New(c).ListTools()
	for name, want := range map[string]string{
		"actor_create":                "else the Events system template, and Default only when neither fits",
		"smart_form_create":           "It creates no data template",
		"smart_form_data_form_create": "only when the user asked to keep what is submitted",
		"smart_form_release":          "offer the user Corezoid processes for its business logic",
		"layer_create":                "never draw ASCII diagrams",
		"reaction_create":             "is actor_attach, not a reaction",
	} {
		if tool := tools[name]; tool == nil || !strings.Contains(tool.Tool.Description, want) {
			t.Errorf("%s lacks %q", name, want)
		}
	}
}

// A repeat of these writes answers what is there and changes nothing; chat_open is not one (a group
// chat is new on every call).
func TestConsoleIdempotentHints(t *testing.T) {
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	tools := New(c).ListTools()
	for name, want := range map[string]bool{"account_create": true, "form_account_create": true, "chat_open": false, "actor_attach": false} {
		hint := tools[name].Tool.Annotations.IdempotentHint
		if hint == nil || *hint != want {
			t.Errorf("%s idempotentHint %v, want %v", name, hint, want)
		}
	}
}
