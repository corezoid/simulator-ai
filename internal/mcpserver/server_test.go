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
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic-test-key" {
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
		if q.Method != "actor/get" {
			t.Error("unexpected method")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":"actor-a","title":"Fixture actor"}}`, q.ID)
	}))
	defer api.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStdioChild$")
	child.Env = append(os.Environ(), "SIMULATOR_AI_TEST_CHILD=1", "SIMULATOR_AI_ORIGIN="+api.URL, "SIMULATOR_AI_WORKSPACE=workspace-a", "SIMULATOR_AI_API_KEY=synthetic-test-key")
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
	if len(result.Tools) != 3 {
		t.Fatalf("unexpected tools: %s", catalog["result"])
	}
	seen := map[string]bool{}
	for _, tool := range result.Tools {
		seen[tool.Name] = true
		if !tool.Annotations.ReadOnly || tool.Annotations.Destructive {
			t.Fatal("unsafe annotations")
		}
	}
	for _, name := range []string{"actor_get", "form_get", "layer_actors_list"} {
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
	if calls.Load() != 1 {
		t.Fatalf("invalid tools reached API; calls=%d", calls.Load())
	}
	if e = in.Close(); e != nil {
		t.Fatal(e)
	}
	if e = child.Wait(); e != nil {
		t.Fatal(e)
	}
}
