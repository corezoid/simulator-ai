package simulation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// fromWire decodes a fixture written as the public API returns it.
func fromWire[T any](t *testing.T, src string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(src), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReadAccountsFoldsDebitCreditAndSkipsSystem(t *testing.T) {
	g := newGraph()
	a := &Actor{ID: "a", Type: "t", Title: "A", Data: NewOMap()}
	g.addActor(a)
	readAccounts(g, a, fromWire[[]publicapi.SimulationAccount](t, `[
		{"meta": {"accountName": "cash", "currencyName": "UAH"}, "currencyId": 1, "nameId": "n1", "amount": 150.5, "incomeType": "credit", "type": "fact"},
		{"meta": {"accountName": "cash", "currencyName": "UAH"}, "currencyId": 1, "nameId": "n1", "amount": 20.25, "incomeType": "debit", "type": "fact"},
		{"meta": {"accountName": "cash", "currencyName": "UAH"}, "currencyId": 1, "nameId": "n1", "amount": 999, "incomeType": "credit", "type": "plan"},
		{"meta": {"accountName": "Total edges", "currencyName": "count", "isSystem": true}, "currencyId": 2, "nameId": "n2", "amount": 7, "incomeType": "credit", "type": "fact"},
		{"meta": {"accountName": "cash", "currencyName": "USD"}, "currencyId": 3, "nameId": "n1", "amount": 3E-8, "incomeType": "credit", "type": "fact", "formula": "a + b"}
	]`))
	if len(g.accounts) != 2 {
		t.Fatalf("accounts: %d", len(g.accounts))
	}
	if v := g.accIdx[accKey{"a", "cash [UAH]"}].Value; Canon(v) != "130.25" {
		t.Errorf("UAH cash = %s, want 130.25 (credit - debit, exact)", Canon(v))
	}
	if v := g.accIdx[accKey{"a", "cash [USD]"}].Value; Canon(v) != "0.00000003" {
		t.Errorf("USD cash = %s, want 0.00000003", Canon(v))
	}
	meta, _ := a.Data.Get("_accounts")
	if usd, _ := meta.(*OMap).Get("cash [USD]"); getString(usd.(*OMap), "formula") != "a + b" {
		t.Errorf("a formula account must keep its formula in the snapshot: %v", usd)
	}
}

func TestGraphFileRoundTrip(t *testing.T) {
	m, g, _ := caseFiles(t, "sample01")
	_ = m
	b, err := GraphFile(g, "")
	if err != nil {
		t.Fatal(err)
	}
	g2, err := LoadGraph(b)
	if err != nil {
		t.Fatal(err)
	}
	if g2.summary() != g.summary() {
		t.Fatalf("%s != %s", g2.summary(), g.summary())
	}
	for _, a := range g.accounts {
		b := g2.accIdx[accKey{a.ActorID, a.Name}]
		if b == nil || b.Value.Cmp(a.Value) != 0 || b.ValueType != a.ValueType {
			t.Errorf("account %s.%s lost in round trip", a.ActorID, a.Name)
		}
	}
	for _, a := range g.actors {
		if g2.actorIdx[a.ID].Type != a.Type {
			t.Errorf("type of %s lost", a.ID)
		}
	}
}

// The layer endpoints return elements in different orders; a snapshot must not depend
// on which one was read, because graph order drives the random draws.
func TestLayerGraphOrderIsCanonical(t *testing.T) {
	nodes := fromWire[[]publicapi.SimulationNode](t, `[
		{"id": "c", "actor": {"title": "C", "formTitle": "t"}}, {"id": "a", "actor": {"title": "A", "formTitle": "t"}},
		{"id": "b", "actor": {"title": "B", "formTitle": "t"}}, {"id": "a", "actor": {"title": "A", "formTitle": "t"}},
		{"id": "l", "actor": {"title": "L", "formTitle": "Layers"}}, {"id": "x", "accessDenied": true}
	]`)
	edges := fromWire[[]publicapi.SimulationEdge](t, `[
		{"id": "e2", "edge": {"source": "a", "target": "b", "edgeTypeId": 1}}, {"id": "e1", "edge": {"source": "b", "target": "c", "edgeTypeId": 2}},
		{"id": "e2", "edge": {"source": "a", "target": "b", "edgeTypeId": 1}}, {"id": "e0", "edge": {"source": "l", "target": "a", "edgeTypeId": 1}}
	]`)
	g := layerGraph("layer", nodes, edges, map[int64]string{1: "hierarchy", 2: "uses"})
	var ids []string
	for _, a := range g.actors {
		ids = append(ids, a.ID)
	}
	var links []string
	for _, l := range g.links {
		links = append(links, l.ID)
	}
	if got := strings.Join(ids, ","); got != "a,b,c" {
		t.Errorf("actors = %s, want a,b,c", got)
	}
	if got := strings.Join(links, ","); got != "e1,e2" {
		t.Errorf("links = %s, want e1,e2", got)
	}
	if et := g.links[0].EdgeType; et != "uses" {
		t.Errorf("edge type = %s, want the type name uses", et)
	}
	if v, _ := g.Source.Get("actors_not_readable"); fmt.Sprint(v) != "[x]" {
		t.Errorf("actors_not_readable = %v, want [x]", v)
	}
}

func TestUnnamedFormTypesFindsBareFormIDs(t *testing.T) {
	g, err := LoadGraph([]byte(`layerId: x
actors:
  - {id: a, title: A, formId: 101}
  - {id: b, title: B, formId: 101, formName: Shops}
  - {id: c, title: C, formId: 102}
edges: []
`))
	if err != nil {
		t.Fatal(err)
	}
	got := unnamedFormTypes(g)
	if len(got) != 2 || len(got["101"]) != 1 || got["101"][0].ID != "a" || len(got["102"]) != 1 {
		t.Errorf("unnamed = %v, want 101:[a] 102:[c]", got)
	}
}
