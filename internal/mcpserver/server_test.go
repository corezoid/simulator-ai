package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

func TestStdioChild(t *testing.T) {
	if os.Getenv("SIMULATOR_AI_TEST_CHILD") != "1" {
		return
	}
	c, e := publicapi.LoadConfig(os.Getenv)
	if e != nil {
		os.Exit(2)
	}
	client, e := publicapi.New(c)
	if e != nil {
		os.Exit(2)
	}
	if e = server.ServeStdio(New(client), server.WithWorkerPoolSize(1), server.WithQueueSize(32)); e != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestScriptedStdioCatalogAndCalls(t *testing.T) {
	for _, profile := range []string{"read", "delegated-read", "graph-read", "graph-create", "reactions-read"} {
		t.Run(profile, func(t *testing.T) { runScriptedProfile(t, profile) })
	}
}

func runScriptedProfile(t *testing.T, profile string) {
	delegated := profile == "delegated-read"
	key, token, scheme := "synthetic-test-key", "", "ApiKey synthetic-test-key"
	if delegated {
		key, token = "", strings.Repeat("A", 43)
		scheme = "Delegated " + token
	}
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != scheme {
			t.Error("unexpected API request")
		}
		var q struct {
			ID     string `json:"id"`
			Method string `json:"method"`
			Params struct {
				AccID string `json:"accId"`
			} `json:"params"`
		}
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		if q.Params.AccID != "workspace-a" {
			t.Error("workspace not pinned")
		}
		result := `{"id":"actor-a","title":"Fixture actor"}`
		switch q.Method {
		case "actor/get":
		case "form/flowchart-block/search":
			result = `{"list":[{"id":21,"parentId":20,"title":"Process","blockId":"process","updatedAt":1,"form":{}}],"hasMore":false}`
		case "form/system/get":
			result = `{"id":20,"title":"Layers","type":"system","form":{},"editToken":"private-canary"}`
		case "edge_type/list":
			result = `{"list":[{"id":3,"name":"interaction","isTree":false,"isSystem":true,"userId":42}]}`
		case "reaction/stats/get":
			result = `{"comment":2,"view":0}`
		case "graph/layer/stats/get":
			result = `{"nodes":7,"edges":8,"holes":1,"holesClosed":2,"edgeHoles":3,"edgeHolesClosed":4}`
		default:
			t.Error("unexpected method")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, result)
	}))
	defer api.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStdioChild$")
	child.Env = append(os.Environ(), "SIMULATOR_AI_TEST_CHILD=1", "SIMULATOR_AI_ORIGIN="+api.URL, "SIMULATOR_AI_WORKSPACE=workspace-a", "SIMULATOR_AI_API_KEY="+key, "SIMULATOR_AI_PROFILE="+profile, "SIMULATOR_AI_DELEGATED_TOKEN="+token, "SIMULATOR_AI_ACTOR=")
	in, e := child.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	out, e := child.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	child.Stderr = os.Stderr
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		_ = in.Close()
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	reader := bufio.NewReader(out)
	exchange := func(line string) map[string]json.RawMessage {
		t.Helper()
		if _, e := io.WriteString(in, line+"\n"); e != nil {
			t.Fatal(e)
		}
		lineOut, e := reader.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		var reply map[string]json.RawMessage
		if e = json.Unmarshal(lineOut, &reply); e != nil {
			t.Fatalf("non-protocol stdout: %q", lineOut)
		}
		return reply
	}
	init := exchange(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"scripted-client","version":"1"}}}`)
	if init["error"] != nil {
		t.Fatalf("initialize failed %s", init["error"])
	}
	if _, e = io.WriteString(in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); e != nil {
		t.Fatal(e)
	}
	catalog := exchange(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var result struct {
		Tools []struct {
			Name        string `json:"name"`
			Annotations struct {
				ReadOnly    bool `json:"readOnlyHint"`
				Destructive bool `json:"destructiveHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if e = json.Unmarshal(catalog["result"], &result); e != nil {
		t.Fatal(e)
	}
	wanted := []string{"actor_get", "form_get"}
	if !delegated {
		wanted = append(wanted, "layer_actors_list")
	}
	if profile == "graph-read" || profile == "graph-create" {
		wanted = append(wanted, "graph_elements_get", "flowchart_blocks_search", "graph_system_form_get", "graph_edge_types_list", "graph_layer_stats_get")
	}
	if profile == "graph-create" {
		wanted = append(wanted, "graph_flowchart_plan", "graph_actor_create", "graph_actor_operation_get", "graph_edge_create", "graph_edge_operation_get", "graph_placements_create", "graph_placements_operation_get")
	}
	if profile == "reactions-read" {
		wanted = append(wanted, "reaction_stats_get")
	}
	if len(result.Tools) != len(wanted) {
		t.Fatalf("unexpected tools: %s", catalog["result"])
	}
	seen := map[string]bool{}
	for _, tool := range result.Tools {
		seen[tool.Name] = true
		write := profile == "graph-create" && strings.HasSuffix(tool.Name, "_create")
		if tool.Annotations.ReadOnly == write || tool.Annotations.Destructive != write {
			t.Fatal("unsafe annotations")
		}
	}
	for _, name := range wanted {
		if !seen[name] {
			t.Fatal(name)
		}
	}
	good := exchange(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"actor_get","arguments":{"actorId":"actor-a"}}}`)
	if !strings.Contains(string(good["result"]), "Fixture actor") {
		t.Fatalf("missing structured result %s", good["result"])
	}
	for i, args := range []string{`{"actorId":"actor-a","accId":"other-workspace"}`, `{"actorId":"actor-a","method":"actor/update"}`, `{"actorId":"actor-a","apiKey":"other"}`, `{"actorId":12}`, `{"ActorID":"actor-a"}`, `{"actorId":null}`, `null`} {
		bad := exchange(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"actor_get","arguments":%s}}`, 10+i, args))
		if bad["error"] == nil && !strings.Contains(string(bad["result"]), `"isError":true`) {
			t.Fatalf("invalid args accepted: %s", bad["result"])
		}
	}
	for i, args := range []string{`{"layerId":"layer-a","limit":null}`, `{"layerId":"layer-a","offset":null}`, `{"layerId":"layer-a","limit":1.5}`, `{"LayerID":"layer-a"}`} {
		bad := exchange(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"layer_actors_list","arguments":%s}}`, 30+i, args))
		if bad["error"] == nil && !strings.Contains(string(bad["result"]), `"isError":true`) {
			t.Fatalf("invalid layer args accepted: %s", bad["result"])
		}
	}
	unknown := exchange(`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"actor_update","arguments":{"actorId":"actor-a"}}}`)
	if unknown["error"] == nil && !strings.Contains(string(unknown["result"]), `"isError":true`) {
		t.Fatal("write tool accepted")
	}
	if delegated {
		unknown := exchange(`{"jsonrpc":"2.0","id":50,"method":"tools/call","params":{"name":"layer_actors_list","arguments":{"layerId":"layer-a"}}}`)
		if unknown["error"] == nil && !strings.Contains(string(unknown["result"]), `"isError":true`) {
			t.Fatal("layer tool exposed to delegated profile")
		}
	}
	expectedCalls := int32(1)
	if profile == "reactions-read" {
		reply := exchange(`{"jsonrpc":"2.0","id":90,"method":"tools/call","params":{"name":"reaction_stats_get","arguments":{"actorId":"actor-a"}}}`)
		if reply["error"] != nil || !strings.Contains(string(reply["result"]), `"comment":2`) || !strings.Contains(string(reply["result"]), `"view":0`) {
			t.Fatalf("reaction stats failed: %s", reply["result"])
		}
		expectedCalls++
		for i, args := range []string{`{}`, `{"actorId":null}`, `{"actorId":""}`, `{"actorId":"a","accId":"other"}`, `{"actorId":"a","method":"actor/update"}`, `{"actorId":"a","apiKey":"secret"}`} {
			bad := exchange(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"reaction_stats_get","arguments":%s}}`, 100+i, args))
			if bad["error"] == nil && !strings.Contains(string(bad["result"]), `"isError":true`) {
				t.Fatal("invalid reaction arguments accepted")
			}
		}
	} else {
		bad := exchange(`{"jsonrpc":"2.0","id":91,"method":"tools/call","params":{"name":"reaction_stats_get","arguments":{"actorId":"actor-a"}}}`)
		if bad["error"] == nil && !strings.Contains(string(bad["result"]), `"isError":true`) {
			t.Fatal("reaction stats exposed outside profile")
		}
	}
	if profile == "graph-read" || profile == "graph-create" {
		for i, step := range []struct{ name, args string }{
			{"flowchart_blocks_search", `{"blockIds":["process"]}`},
			{"graph_system_form_get", `{"title":"Layers"}`},
			{"graph_edge_types_list", `{}`},
			{"graph_layer_stats_get", `{"actorId":"layer"}`},
		} {
			reply := exchange(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, 60+i, step.name, step.args))
			if reply["error"] != nil || strings.Contains(string(reply["result"]), `"isError":true`) || strings.Contains(string(reply["result"]), "private-canary") {
				t.Fatalf("graph read failed or leaked capability: %s", reply["result"])
			}
		}
		expectedCalls += 4
		for i, args := range []string{`{"blockIds":[]}`, `{"accId":"other"}`, `{"limit":101}`, `{"method":"actor/create"}`, `{"apiKey":"other"}`} {
			bad := exchange(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"flowchart_blocks_search","arguments":%s}}`, 70+i, args))
			if bad["error"] == nil && !strings.Contains(string(bad["result"]), `"isError":true`) {
				t.Fatal("invalid graph arguments accepted")
			}
		}
	}
	if calls.Load() != expectedCalls {
		t.Fatalf("invalid tools reached API; calls=%d", calls.Load())
	}
	if e = in.Close(); e != nil {
		t.Fatal(e)
	}
	if e = child.Wait(); e != nil {
		t.Fatal(e)
	}
}
