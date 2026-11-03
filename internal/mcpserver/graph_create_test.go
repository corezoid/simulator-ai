package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
)

func TestGraphCreateCatalogAndPublicWorkflow(t *testing.T) {
	posts, preflights := 0, 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/doc/public" {
			preflights++
			_, _ = w.Write(contracts.GraphCreates)
			return
		}
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic" {
			t.Error("unexpected path/auth")
		}
		var q struct {
			ID     string                     `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if string(q.Params["accId"]) != `"workspace-a"` {
			t.Error("workspace not bound")
		}
		posts++
		output := ""
		switch q.Method {
		case "actor/create", "actor/operation/get":
			output = `{"id":"actor-a","formId":20,"title":"Test"}`
		case "edge/create", "edge/operation/get":
			output = `{"id":"edge-a","edgeTypeId":3,"source":"actor-a","target":"actor-b","name":"Yes","userId":42}`
		case "graph/layer/actors/manage", "graph/layer/operation/get":
			output = `{"nodes":[{"laId":11,"actorId":"actor-a","position":{"x":0,"y":0}}],"edges":[{"laId":11,"edgeId":"edge-a","laIdSource":11,"laIdTarget":12}]}`
		default:
			t.Error("unexpected public method")
			w.WriteHeader(501)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, output)
	}))
	defer api.Close()
	c, err := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "graph-create"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	if len(s.ListTools()) != 13 {
		t.Fatalf("unexpected catalog size %d", len(s.ListTools()))
	}
	for name, tool := range s.ListTools() {
		write := strings.HasSuffix(name, "_create")
		raw, _ := json.Marshal(tool.Tool)
		if !strings.Contains(string(raw), fmt.Sprintf(`"readOnlyHint":%t`, !write)) || !strings.Contains(string(raw), fmt.Sprintf(`"destructiveHint":%t`, write)) {
			t.Fatalf("wrong hints %s", name)
		}
	}
	call := func(name, args string) string {
		t.Helper()
		reply := s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, args)))
		raw, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for _, step := range []struct{ name, args string }{
		{"graph_actor_create", `{"operationId":"actor-1","formId":20,"title":"Test","data":{}}`},
		{"graph_actor_operation_get", `{"operationId":"actor-1"}`},
		{"graph_edge_create", `{"operationId":"edge-1","source":"actor-a","target":"actor-b","edgeTypeId":3,"name":"Yes"}`},
		{"graph_edge_operation_get", `{"operationId":"edge-1"}`},
		{"graph_placements_create", `{"operationId":"place-1","actorId":"layer","items":[{"action":"create","data":{"id":"actor-a","type":"node","position":{"x":0,"y":0}}},{"action":"create","data":{"id":"edge-a","type":"edge","laIdSource":11,"laIdTarget":12}}]}`},
		{"graph_placements_operation_get", `{"operationId":"place-1","actorId":"layer"}`},
	} {
		out := call(step.name, step.args)
		if strings.Contains(out, `"error":`) || strings.Contains(out, `"isError":true`) || strings.Contains(out, `"userId"`) {
			t.Fatalf("%s: %s", step.name, out)
		}
	}
	if posts != 6 || preflights != 3 {
		t.Fatalf("posts %d preflight %d", posts, preflights)
	}
	for _, step := range []struct{ name, args string }{
		{"graph_actor_create", `{"operationId":"a","formId":20,"title":"Test","data":{},"accId":"other"}`},
		{"graph_placements_create", `{"operationId":"p","actorId":"layer","items":[{"action":"delete","data":{"id":"a","type":"node"}}]}`},
		{"graph_edge_create", `{"source":"a","target":"b","edgeTypeId":3}`},
		{"actor_update", `{}`},
	} {
		out := call(step.name, step.args)
		if !strings.Contains(out, `"error":`) && !strings.Contains(out, `"isError":true`) {
			t.Fatal("forbidden arguments/tool accepted")
		}
	}
	if posts != 6 || preflights != 3 {
		t.Fatal("forbidden request reached network")
	}
}
