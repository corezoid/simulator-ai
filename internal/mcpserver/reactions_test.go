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

func TestReactionStatsActorBoundCatalog(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q struct {
			ID     string `json:"id"`
			Params struct {
				AccID       string `json:"accId"`
				RootActorID string `json:"rootActorId"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if q.Params.AccID != "workspace" || q.Params.RootActorID != "host-actor" {
			t.Error("actor/workspace binding escaped")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"comment":2}}`, q.ID)
	}))
	defer backend.Close()
	client, err := publicapi.New(publicapi.Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "reactions-read", ActorID: "host-actor"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(client)
	catalog := s.ListTools()
	if len(catalog) != 1 {
		t.Fatal("workspace tools exposed in bound profile")
	}
	if _, ok := catalog["reaction_stats_get"]; !ok {
		t.Fatal("bound stats tool missing")
	}
	call := func(args string) string {
		t.Helper()
		reply := s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"reaction_stats_get","arguments":%s}}`, args)))
		raw, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if out := call(`{}`); !strings.Contains(out, `"comment":2`) {
		t.Fatal(out)
	}
	for _, args := range []string{`{"actorId":"other"}`, `{"actorId":"host-actor"}`, `{"accId":"other"}`, `{"rootActorId":"other"}`, `null`} {
		out := call(args)
		if !strings.Contains(out, `"isError":true`) && !strings.Contains(out, `"error":`) {
			t.Fatal("bound arguments accepted", out)
		}
	}
	if calls != 1 {
		t.Fatal("rejected arguments reached backend")
	}
}
