package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
)

// The fixture simulates response loss AFTER a committed create; the client must recover,
// never retry automatically. Backend functional receipt tests own actual transaction proof.
func TestGraphCreateRecoveryAndPreflight(t *testing.T) {
	posts, preflights := 0, 0
	stale, lost, wrong := false, true, false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/doc/public" {
			preflights++
			if r.Header.Get("Authorization") != "" {
				t.Error("credential on public schema read")
			}
			if stale {
				fmt.Fprint(w, `{"methods":[]}`)
			} else {
				_, _ = w.Write(contracts.GraphCreates)
			}
			return
		}
		posts++
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic-graph" {
			t.Error("unexpected transport")
		}
		q := readSmart(t, r)
		if q.Method != "actor/create" && q.Method != "actor/operation/get" {
			t.Error("unexpected method")
		}
		if q.Method == "actor/create" && lost {
			w.WriteHeader(503)
			return
		}
		formID := 20
		if wrong {
			formID = 21
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":"created-actor","formId":%d,"title":"Test","data":{"secret":"canary"},"owner":{"apiKey":"canary"}}}`, q.ID, formID)
	}))
	defer api.Close()
	c, err := New(Config{Origin: api.URL, Workspace: "workspace-a", APIKey: "synthetic-graph", Profile: "graph-create"})
	if err != nil {
		t.Fatal(err)
	}
	input := GraphActorInput{Operation: Operation{OperationID: "actor-1"}, FormID: 20, Title: "Test", Data: json.RawMessage(`{}`)}
	_, err = c.CreateGraphActor(context.Background(), input)
	var uncertain *WriteError
	if !errors.As(err, &uncertain) || !strings.Contains(err.Error(), "preserve operationId") || posts != 1 || preflights != 1 {
		t.Fatalf("loss wasn't preserved: %v posts=%d preflight=%d", err, posts, preflights)
	}
	actor, err := c.GetGraphActorOperation(context.Background(), input.Operation)
	if err != nil || actor.ID == nil || *actor.ID != "created-actor" || posts != 2 || preflights != 1 {
		t.Fatalf("recovery: %+v %v", actor, err)
	}
	raw, _ := json.Marshal(actor)
	if strings.Contains(string(raw), "canary") {
		t.Fatal("unprojected actor data leaked")
	}
	stale = true
	lost = false
	if _, err = c.CreateGraphActor(context.Background(), input); err == nil || posts != 2 || preflights != 2 {
		t.Fatal("write reached stale backend")
	}
	stale = false
	wrong = true
	if _, err = c.CreateGraphActor(context.Background(), input); !errors.As(err, &uncertain) {
		t.Fatal("malformed success lost uncertainty")
	}
	before := posts
	beforePreflight := preflights
	input.OperationID = ""
	if _, err = c.CreateGraphActor(context.Background(), input); err == nil || posts != before || preflights != beforePreflight {
		t.Fatal("invalid input reached network")
	}
	for _, profile := range []string{"read", "graph-read", "smart-forms", "delegated-read"} {
		c.config.Profile = profile
		input.OperationID = "actor-1"
		if _, err = c.CreateGraphActor(context.Background(), input); err == nil || posts != before || preflights != beforePreflight {
			t.Fatal("profile fence bypassed")
		}
	}
}

func TestGraphPlacementBindingPreservesRepeatedActors(t *testing.T) {
	a, b := int64(11), int64(12)
	p := GraphPlacementsInput{Items: []GraphPlacementItem{
		{Action: "create", Data: GraphPlacementData{Type: "node", ID: "same-actor"}},
		{Action: "create", Data: GraphPlacementData{Type: "node", ID: "same-actor"}},
		{Action: "create", Data: GraphPlacementData{Type: "edge", ID: "edge", LaIDSource: &a, LaIDTarget: &b}},
	}}
	r := GraphCreatedPlacements{Nodes: []GraphCreatedNode{
		{LaID: 12, ActorID: "same-actor", Position: &GraphPoint{X: 50, Y: 100}},
		{LaID: 11, ActorID: "same-actor", Position: &GraphPoint{}},
	}, Edges: []GraphCreatedPlacementEdge{{LaID: 11, EdgeID: "edge", LaIDSource: &a, LaIDTarget: &b}}}
	if err := validateGraphPlacements(r); err != nil {
		t.Fatal(err)
	}
	if err := bindGraphPlacements(p, r); err != nil {
		t.Fatal(err)
	}
	r.Edges[0].LaIDSource = &b
	if bindGraphPlacements(p, r) == nil {
		t.Fatal("wrong edge anchor accepted")
	}
	r.Edges[0].LaIDSource = &a
	r.Nodes[1].LaID = 12
	if validateGraphPlacements(r) == nil {
		t.Fatal("duplicate placement id accepted")
	}
	r.Nodes[1].LaID = 11
	r.Edges = append(r.Edges, r.Edges[0])
	if validateGraphPlacements(r) == nil {
		t.Fatal("duplicate edge placement id accepted")
	}
	var position GraphPoint
	for _, raw := range []string{`{}`, `{"x":1}`, `{"x":null,"y":0}`} {
		if json.Unmarshal([]byte(raw), &position) == nil {
			t.Fatal("missing position became zero")
		}
	}
	if err := json.Unmarshal([]byte(`{"x":0,"y":0}`), &position); err != nil {
		t.Fatal(err)
	}
}

func TestGraphEdgeResolvedDirection(t *testing.T) {
	p := GraphEdgeInput{Source: "a", Target: "b"}
	r := GraphCreatedEdge{Source: "b", Target: "a"}
	if !sameEdgeEnds(p, r) {
		t.Fatal("historical reverse resolution rejected")
	}
	p.ForceDirection = true
	if sameEdgeEnds(p, r) {
		t.Fatal("forced direction ignored")
	}
	r.Target = "foreign"
	if sameEdgeEnds(p, r) {
		t.Fatal("foreign endpoint accepted")
	}
}
