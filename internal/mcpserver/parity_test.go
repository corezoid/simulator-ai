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

type parityCall struct {
	Method string
	Params map[string]json.RawMessage
}

// parityServer answers every method with {"ok":true}, a batch with one result per item, and layer
// pages from the given placements (then an empty page).
func parityServer(t *testing.T, profile string) (func(string) string, *[]parityCall) {
	calls := &[]parityCall{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string                     `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if string(q.Params["accId"]) != `"ws"` && q.Method != "batch" {
			t.Errorf("%s sent without the host workspace", q.Method)
		}
		*calls = append(*calls, parityCall{q.Method, q.Params})
		result := `{"ok":true}`
		switch q.Method {
		case "batch":
			var list []json.RawMessage
			_ = json.Unmarshal(q.Params["list"], &list)
			items := make([]string, len(list))
			for i := range list {
				items[i] = fmt.Sprintf(`{"id":"%d","result":{"id":"e%d"}}`, i, i)
			}
			result = `{"list":[` + strings.Join(items, ",") + `]}`
		case "form/tree/get":
			result = `{"list":[{"id":1},{"id":2,"parentId":1,"title":"Child"},{"id":3,"parentId":2,"title":"Grandchild"},{"id":4,"parentId":1,"accessDenied":true}],"truncated":true}`
		case "form/get":
			result = `{"id":2,"parents":[{"id":1,"title":"Root"}]}`
		case "edge_type/get":
			result = `{"id":3,"name":"hierarchy"}`
		case "graph/layer/actors/list":
			result = `{"list":[]}`
			if string(q.Params["offset"]) == "0" {
				result = `{"list":[{"laId":1,"id":"a1","position":{"x":0,"y":0},"actor":{"id":"a1","title":"Hub"}},{"laId":2,"id":"a2","position":{"x":2000,"y":0},"actor":{"id":"a2","title":"Far"}}]}`
			}
		case "graph/layer/edges/list":
			result = `{"list":[]}`
			if string(q.Params["offset"]) == "0" {
				result = `{"list":[{"laId":9,"id":"e1","laIdSource":1,"laIdTarget":2,"edge":{"id":"e1","edgeTypeId":3,"source":"a1","target":"a2"}}]}`
			}
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, result)
	}))
	t.Cleanup(api.Close)
	cfg := publicapi.Config{Origin: api.URL, Workspace: "ws", Profile: profile, APIKey: "key"}
	if profile == "console" || profile == "console-full" {
		cfg.APIKey, cfg.Authorization = "", "Bearer user"
	}
	c, err := publicapi.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	id := 0
	send := func(body string) string {
		id++
		reply, _ := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,%s}`, id, body))))
		return string(reply)
	}
	send(`"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}`)
	return send, calls
}

func catalog(t *testing.T, send func(string) string) map[string]bool {
	var list struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				Annotations map[string]any `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(send(`"method":"tools/list"`)), &list); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Result.Tools {
		names[tool.Name] = tool.Annotations["readOnlyHint"] == true
	}
	return names
}

// allParityTools is every parity tool list the console registers.
func allParityTools() []parityTool {
	var all []parityTool
	for _, list := range [][]parityTool{actorTools, formTools, graphTools, reactionTools, userTools, attachmentTools, accessRequestTools, accountTools, ledgerTools, accountExtraTools, smartFormTools, corezoidTools} {
		all = append(all, list...)
	}
	return all
}

// destructiveHints reads each tool's destructiveHint from tools/list.
func destructiveHints(t *testing.T, send func(string) string) map[string]bool {
	var list struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				Annotations map[string]any `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(send(`"method":"tools/list"`)), &list); err != nil {
		t.Fatal(err)
	}
	hints := map[string]bool{}
	for _, tool := range list.Result.Tools {
		hints[tool.Name] = tool.Annotations["destructiveHint"] == true
	}
	return hints
}

func TestParityCatalogs(t *testing.T) {
	all := allParityTools()
	for _, tc := range []struct {
		profile string
		want    func(publicapi.ParityMethod, string) bool
	}{
		{"console-full", func(publicapi.ParityMethod, string) bool { return true }},
		{"console", func(_ publicapi.ParityMethod, name string) bool { return consoleCoreTools[name] }},
		{"read", func(m publicapi.ParityMethod, _ string) bool {
			return !m.Write && (m.Domain == "actor" || m.Domain == "form")
		}},
		{"graph-read", func(m publicapi.ParityMethod, name string) bool {
			return !m.Write && (m.Domain == "actor" || m.Domain == "form" || m.Domain == "graph") && name != "edge_types_list" && name != "layer_stats_get"
		}},
		{"graph-create", func(publicapi.ParityMethod, string) bool { return false }},
		{"smart-forms", func(publicapi.ParityMethod, string) bool { return false }},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			send, _ := parityServer(t, tc.profile)
			names := catalog(t, send)
			for _, p := range all {
				m := publicapi.ParityMethods[p.Method]
				_, has := names[p.Name]
				want := tc.want(m, p.Name)
				if (p.Name == "actor_get" || p.Name == "form_get") && tc.profile != "console" {
					has, want = has && names[p.Name] == !m.Write, true // metadata or parity version
				}
				if has != want {
					t.Errorf("%s: has %v, want %v", p.Name, has, want)
				}
				if has && names[p.Name] == m.Write {
					t.Errorf("%s: readOnlyHint does not match the method", p.Name)
				}
			}
			for _, name := range []string{"actors_create", "edges_create", "edges_by_nodes_delete", "layer_compact", "layer_prune_long_edges"} {
				if _, has := names[name]; has != (tc.profile == "console-full" || tc.profile == "console" && consoleCoreTools[name]) {
					t.Errorf("%s in %s: %v", name, tc.profile, has)
				}
			}
			if tc.profile == "console-full" {
				hints := destructiveHints(t, send)
				for _, name := range []string{"account_amount_set", "account_formula_set", "atom_transaction_create", "account_tags_set", "account_triggers_set", "account_actor_triggers_set", "actor_field_triggers_set",
					"smart_form_release_rollback", "smart_form_file_rollback", "smart_form_trash_restore", "corezoid_project_create", "corezoid_project_bind"} {
					if !hints[name] {
						t.Errorf("%s is not hinted destructive", name)
					}
				}
			}
			if _, has := names["layer_placements_all"]; has != (tc.profile == "console-full" || tc.profile == "graph-read") {
				t.Errorf("layer_placements_all in %s: %v", tc.profile, has)
			}
		})
	}
}

func TestParityRequestsCarryArgumentsAndWorkspace(t *testing.T) {
	send, calls := parityServer(t, "console-full")
	all := allParityTools()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	for _, p := range all {
		*calls = nil
		reply := send(`"method":"tools/call","params":{"name":"` + p.Name + `","arguments":{"probe":1}}`)
		if !strings.Contains(reply, `"error"`) && !strings.Contains(reply, `"isError":true`) {
			t.Errorf("%s accepted an undeclared argument: %s", p.Name, reply)
		}
		if len(*calls) != 0 {
			t.Errorf("%s reached the API with invalid arguments", p.Name)
		}
	}
	cases := []struct{ tool, args, method, wire string }{
		{"actor_get", `{"actorId":"a1","fields":["id","data.amount"],"with":["owner"]}`, "actor/get", `"fields":["id","data.amount"]`},
		{"actor_create", `{"formId":5,"title":"T"}`, "actor/create", `"data":{}`},
		{"actor_update", `{"actorId":"a1","data":{"k":1}}`, "actor/update", `"fields":["id","title","description","formId","status","ref","color"]`},
		{"actors_filter", `{"formId":5,"limit":10}`, "actor/filter/list", `"limit":10`},
		{"form_update", `{"formId":5,"title":"F","expectedEditToken":"tok"}`, "form/update", `"expectedEditToken":"tok"`},
		{"form_account_delete", `{"formId":5,"id":7}`, "account_form/delete", `"id":7`},
		{"graph_list", `{"actorId":"a1","type":"children"}`, "graph/list", `"edgeTypeId":3,"fields":["id","title","formId"],"type":"children"`},
		{"graph_list", `{"actorId":"a1","type":"linked","edgeTypeId":9}`, "graph/list", `"edgeTypeId":9`},
		{"layer_clean", `{"actorId":"L"}`, "graph/layer/clean", `"actorId":"L"`},
		{"edge_exist", `{"source":"a","target":"b","edgeTypeId":3,"bidirected":true}`, "edge/by-node/get", `"bidirected":true,"edgeTypeId":3`},
		{"transaction_create", `{"accountId":"r1","amount":10,"ref":"t1"}`, "transaction/create", `"accountId":"r1","amount":10,"ref":"t1"`},
		{"transfer_create", `{"ref":"t2","from":[{"accountId":"d1","amount":5}],"to":[{"accountId":"c1","amount":5}]}`, "transfer/create", `"from":[{"accountId":"d1","amount":5}],"ref":"t2","to":[{"accountId":"c1","amount":5}]`},
		{"atom_transaction_create", `{"transactions":[{"accountId":"d1","amount":1,"ref":"a"},{"accountId":"c1","amount":1,"ref":"b"}]}`, "transaction/atom/create", `"transactions":[{"accountId":"d1","amount":1,"ref":"a"},{"accountId":"c1","amount":1,"ref":"b"}]`},
		{"account_amount_set", `{"id":"r1","amount":7}`, "account_actor/amount/update", `"amount":7,"id":"r1"`},
		{"actor_field_triggers_set", `{"actorId":"a1","dataField":"price","triggers":["t1"]}`, "actor/field/triggers/update", `"actorId":"a1","dataField":"price","triggers":["t1"]`},
		{"account_tags_set", `{"nameId":"n1","currencyId":3,"tags":["t1"]}`, "account/tags/update", `"currencyId":3,"nameId":"n1","tags":["t1"]`},
		{"account_create", `{"actorId":"a1","nameId":"n1","currencyId":3,"counterType":"counter","ignoreIfExist":true}`, "account_actor/create", `"counterType":"counter","currencyId":3,"ignoreIfExist":true`},
		{"counter_increment", `{"formId":5,"actorRef":"car","accountName":"Mileage","currency":"km","incomeType":"debit","amount":12}`, "counter/increment", `"accountName":"Mileage","actorRef":"car","amount":12`},
		{"smart_form_folders_create", `{"actorId":"app1","items":[{"folderId":101,"title":"thanks"}]}`, "app_folder/create", `"actorId":"app1","items":[{"folderId":101,"title":"thanks"}]`},
		{"smart_form_release_rollback", `{"actorId":"app1","releaseId":8}`, "release/rollback", `"actorId":"app1","releaseId":8`},
		{"smart_form_trash_restore", `{"actorId":"app1","items":[{"id":5,"objType":"file"}]}`, "trash/restore", `"items":[{"id":5,"objType":"file"}]`},
		{"smart_form_page_render", `{"ref":"leave_form","envTitle":"develop","page":"index"}`, "page/render", `"envTitle":"develop","page":"index","ref":"leave_form"`},
		{"corezoid_project_bind", `{"actorId":"app1","projectId":4,"envs":[{"envTitle":"develop","stageId":5,"procId":6},{"envTitle":"production","stageId":7,"procId":8}]}`, "corezoid/project/bind", `"envs":[{"envTitle":"develop","stageId":5,"procId":6}`},
		{"layer_edge_settings_save", `{"actorId":"L","laEdgeId":21,"layerSettings":{"lineStyle":"dashed","width":3,"color":"#FF0000"}}`, "graph/layer/edge_settings/save", `"laEdgeId":21,"layerSettings":{"lineStyle":"dashed","width":3,"color":"#FF0000"}`},
	}
	for _, tc := range cases {
		*calls = nil
		reply := send(`"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":` + tc.args + `}`)
		if strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, `\"ok\":true`) && !strings.Contains(reply, `"ok":true`) {
			t.Fatalf("%s: %s", tc.tool, reply)
		}
		last := (*calls)[len(*calls)-1]
		wire, _ := json.Marshal(last.Params)
		if last.Method != tc.method || !strings.Contains(string(wire), tc.wire) {
			t.Errorf("%s sent %s %s", tc.tool, last.Method, wire)
		}
	}
}

func TestParityReadProfileCannotWrite(t *testing.T) {
	send, calls := parityServer(t, "read")
	for _, name := range []string{"actor_create", "actor_delete", "form_delete", "edge_delete", "layer_clean"} {
		reply := send(`"method":"tools/call","params":{"name":"` + name + `","arguments":{}}`)
		if !strings.Contains(reply, `"error"`) && !strings.Contains(reply, `"isError":true`) {
			t.Errorf("read profile ran %s: %s", name, reply)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("a write reached the API from the read profile")
	}
}

func TestParityEngines(t *testing.T) {
	send, calls := parityServer(t, "console-full")
	var reply string
	*calls = nil
	reply = send(`"method":"tools/call","params":{"name":"layer_placements_all","arguments":{"layerId":"L"}}`)
	if !strings.Contains(reply, `\"count\":2`) && !strings.Contains(reply, `"count":2`) || len(*calls) != 2 {
		t.Fatal("layer_placements_all:", reply, len(*calls))
	}

	*calls = nil
	reply = send(`"method":"tools/call","params":{"name":"layer_prune_long_edges","arguments":{"layerId":"L","maxDistance":600}}`)
	if strings.Contains(reply, `"isError":true`) || (*calls)[len(*calls)-1].Method != "batch" || !strings.Contains(string((*calls)[len(*calls)-1].Params["list"]), `"edgeId":"e1"`) {
		t.Fatal("prune:", reply)
	}
	*calls = nil
	send(`"method":"tools/call","params":{"name":"layer_prune_long_edges","arguments":{"layerId":"L","dryRun":true}}`)
	for _, c := range *calls {
		if c.Method == "batch" || c.Method == "edge/delete" {
			t.Fatal("dryRun deleted an edge")
		}
	}
}

func TestParityLinkedForms(t *testing.T) {
	send, calls := parityServer(t, "read")
	children := send(`"method":"tools/call","params":{"name":"form_linked_get","arguments":{"formId":2,"direction":"children"}}`)
	if !strings.Contains(children, "Grandchild") || !strings.Contains(children, `Child`) || strings.Contains(children, "accessDenied") || !strings.Contains(children, `truncated`) {
		t.Fatal("children:", children)
	}
	parents := send(`"method":"tools/call","params":{"name":"form_linked_get","arguments":{"formId":2,"direction":"parents"}}`)
	if !strings.Contains(parents, "Root") || !strings.Contains(parents, `"id":2`) && !strings.Contains(parents, `\"id\":2`) || string((*calls)[len(*calls)-1].Params["with"]) != `["parents"]` {
		t.Fatal("parents:", parents)
	}
	tree := send(`"method":"tools/call","params":{"name":"forms_tree_get","arguments":{"formId":2}}`)
	if !strings.Contains(tree, "accessDenied") {
		t.Fatal("tree:", tree)
	}
}

func TestParitySearchAll(t *testing.T) {
	send, calls := parityServer(t, "read")
	reply := send(`"method":"tools/call","params":{"name":"search_all","arguments":{"query":"acme","filters":["users","fullTextActor"],"limit":5}}`)
	if strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, "users") || !strings.Contains(reply, "actors") {
		t.Fatal(reply)
	}
	last := (*calls)[len(*calls)-1]
	list := string(last.Params["list"])
	if last.Method != "batch" || string(last.Params["isAtomic"]) == "true" || !strings.Contains(list, `"method":"user/list","params":{"accId":"ws","query":"acme","limit":5}`) ||
		!strings.Contains(list, `"method":"actor/search/list","params":{"accId":"ws","query":"acme","searchType":"text","fields":["id","title","formId"],"limit":5}`) {
		t.Fatal("search batch:", list)
	}
	*calls = nil
	for _, args := range []string{`{"query":"a","filters":["users"]}`, `{"query":"acme","filters":[]}`, `{"query":"acme","filters":["forms"]}`} {
		if r := send(`"method":"tools/call","params":{"name":"search_all","arguments":` + args + `}}`); !strings.Contains(r, `"isError":true`) && !strings.Contains(r, `"error"`) {
			t.Error("accepted", args)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("invalid search reached the API")
	}
}

func TestParitySearchAllDefaultsAndErrors(t *testing.T) {
	send, calls := parityServer(t, "read")
	send(`"method":"tools/call","params":{"name":"search_all","arguments":{"query":"acme","filters":["actors"]}}`)
	if list := string((*calls)[len(*calls)-1].Params["list"]); !strings.Contains(list, `"searchType":"text"`) {
		t.Fatal("search_all must search text by default:", list)
	}
}

// strip drops only its path: every other value, a large id or a decimal among them, stays as written.
func TestStripKeepsOtherValues(t *testing.T) {
	got, _ := strip(json.RawMessage(`{"invite":{"id":12345678901234567890,"hash":"secret","amount":1.50},"event":{"id":"e1"}}`), []string{"invite.hash", "missing.key"})
	if string(got) != `{"event":{"id":"e1"},"invite":{"amount":1.50,"id":12345678901234567890}}` {
		t.Fatal(string(got))
	}
}
