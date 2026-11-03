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

func TestGraphPlanUsesOnlyBoundPublicCatalog(t *testing.T) {
	posts := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic" {
			t.Error("unbound request")
		}
		var q struct {
			ID     string `json:"id"`
			Method string `json:"method"`
			Params struct {
				AccID    string   `json:"accId"`
				BlockIDs []string `json:"blockIds"`
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if q.Method != "form/flowchart-block/search" || q.Params.AccID != "workspace-a" || len(q.Params.BlockIDs) != 1 || q.Params.BlockIDs[0] != "process" {
			t.Error("unexpected catalog read")
		}
		view := `{"shape":{"type":"path","d":"M0 0 L200 0 L200 100 Z"},"size":{"w":200,"h":100},"textFrame":{"x":5,"y":5,"w":190,"h":90}}`
		form := map[string]any{"sections": []any{map[string]any{"content": []any{map[string]any{"id": "blockId", "value": "process"}, map[string]any{"id": "view", "value": view}}}}}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": map[string]any{"list": []any{map[string]any{"id": 22, "parentId": 21, "title": "Process", "blockId": "process", "form": form}}, "hasMore": false}})
	}))
	defer api.Close()
	c, err := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "graph-create"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	call := func(args string) string {
		t.Helper()
		out := s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graph_flowchart_plan","arguments":%s}}`, args)))
		raw, _ := json.Marshal(out)
		return string(raw)
	}
	good := `{"nodes":[{"key":"one","blockId":"process","title":"Крок"}],"edges":[]}`
	out := call(good)
	if strings.Contains(out, `"isError":true`) || !strings.Contains(out, "native-flowchart-dag-v1") || posts != 1 {
		t.Fatal(out, posts)
	}
	for _, bad := range []string{strings.Replace(good, `"edges":[]`, `"edges":[],"accId":"other"`, 1), strings.Replace(good, `"key":"one"`, `"key":""`, 1), `{"nodes":[],"edges":[]}`, strings.Replace(good, `"edges":[]`, `"edges":[{"key":"x","source":"one","target":"missing"}]`, 1)} {
		out = call(bad)
		if !strings.Contains(out, `"isError":true`) && !strings.Contains(out, `"error":`) {
			t.Fatal("invalid plan accepted", out)
		}
	}
	if posts != 1 {
		t.Fatal("invalid plan reached network")
	}
	for _, profile := range []string{"read", "graph-read", "smart-forms"} {
		c, err := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: profile})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := New(c).ListTools()["graph_flowchart_plan"]; ok {
			t.Fatal("planner leaked to profile", profile)
		}
	}
}
