package simulation

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

// fakeAPI answers each JSON-RPC call with reply(method, params) as the result and counts calls.
func fakeAPI(t *testing.T, reply func(method string, params map[string]any) any) (*publicapi.Client, map[string]int) {
	t.Helper()
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls[req.Method]++
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": reply(req.Method, req.Params)})
	}))
	t.Cleanup(srv.Close)
	c, err := publicapi.New(publicapi.Config{Origin: srv.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator test"})
	if err != nil {
		t.Fatal(err)
	}
	return c, calls
}

func TestSnapshotReadsAccountsOfBoundedActors(t *testing.T) {
	prev := maxAccountActors
	t.Cleanup(func() { maxAccountActors = prev })
	maxAccountActors = 3
	c, calls := fakeAPI(t, func(method string, params map[string]any) any {
		var list []any
		if method == "graph/layer/actors/list" && params["offset"] == 0.0 {
			for i := 0; i < 10; i++ {
				list = append(list, map[string]any{"id": fmt.Sprintf("n%d", i), "actor": map[string]any{"title": fmt.Sprint("N", i), "formTitle": "t"}})
			}
		}
		return map[string]any{"list": list}
	})
	g, err := Snapshot(context.Background(), c, "layer", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.actors) != 10 || calls["account_actor/list"] != 3 {
		t.Errorf("actors %d, account reads %d; want 10 actors and 3 reads", len(g.actors), calls["account_actor/list"])
	}
	if note := getString(g.Source, "accounts_not_read"); !strings.HasPrefix(note, "truncated") || !strings.Contains(note, "other 7") {
		t.Errorf("accounts_not_read = %q", note)
	}
}

func TestNameFormTypesReadsEachFormOnceUpToTheCap(t *testing.T) {
	prev := maxFormNames
	t.Cleanup(func() { maxFormNames = prev })
	maxFormNames = 2
	g, err := LoadGraph([]byte("layerId: x\nactors:\n  - {id: a, title: A, formId: 1}\n  - {id: b, title: B, formId: 1}\n" +
		"  - {id: c, title: C, formId: 2}\n  - {id: d, title: D, formId: 3}\nedges: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	c, calls := fakeAPI(t, func(method string, params map[string]any) any {
		return map[string]any{"id": params["formId"], "title": fmt.Sprint("Form ", params["formId"])}
	})
	if n := nameFormTypes(context.Background(), c, g); n != 3 || calls["form/get"] != 2 {
		t.Errorf("named %d actors with %d form reads; want 3 with 2", n, calls["form/get"])
	}
	if g.actorIdx["d"].Type != "3" {
		t.Errorf("a form past the cap is named %q, want its id", g.actorIdx["d"].Type)
	}
}

func TestSnapshotStopsAfterTheLayerPageLimit(t *testing.T) {
	c, calls := fakeAPI(t, func(method string, params map[string]any) any {
		return map[string]any{"list": []any{map[string]any{"id": fmt.Sprint("n", params["offset"]), "actor": map[string]any{"title": "N"}}}}
	})
	_, err := Snapshot(context.Background(), c, "layer", 0)
	if err == nil || !strings.Contains(err.Error(), "more than 20000 node placements") || calls["graph/layer/actors/list"] != 101 {
		t.Errorf("err %v after %d pages; want the page limit after 101", err, calls["graph/layer/actors/list"])
	}
}
