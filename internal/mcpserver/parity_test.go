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
	if profile == "console" {
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

func TestParityCatalogs(t *testing.T) {
	all := append(append(append([]parityTool{}, actorTools...), formTools...), graphTools...)
	for _, tc := range []struct {
		profile string
		want    func(publicapi.ParityMethod, string) bool
	}{
		{"console", func(publicapi.ParityMethod, string) bool { return true }},
		{"read", func(m publicapi.ParityMethod, _ string) bool { return !m.Write && m.Domain != "graph" }},
		{"graph-read", func(m publicapi.ParityMethod, name string) bool {
			return !m.Write && name != "edge_types_list" && name != "layer_stats_get"
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
				if p.Name == "actor_get" || p.Name == "form_get" {
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
				if _, has := names[name]; has != (tc.profile == "console") {
					t.Errorf("%s in %s: %v", name, tc.profile, has)
				}
			}
			if _, has := names["layer_placements_all"]; has != (tc.profile == "console" || tc.profile == "graph-read") {
				t.Errorf("layer_placements_all in %s: %v", tc.profile, has)
			}
		})
	}
}

func TestParityRequestsCarryArgumentsAndWorkspace(t *testing.T) {
	send, calls := parityServer(t, "console")
	all := append(append(append([]parityTool{}, actorTools...), formTools...), graphTools...)
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
		{"graph_list", `{"actorId":"a1","type":"children"}`, "graph/list", `"edgeTypeId":3,"type":"children"`},
		{"graph_list", `{"actorId":"a1","type":"linked","edgeTypeId":9}`, "graph/list", `"edgeTypeId":9`},
		{"layer_clean", `{"actorId":"L"}`, "graph/layer/clean", `"actorId":"L"`},
		{"edge_exist", `{"source":"a","target":"b","edgeTypeId":3,"bidirected":true}`, "edge/by-node/get", `"bidirected":true,"edgeTypeId":3`},
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
	send, calls := parityServer(t, "console")
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
