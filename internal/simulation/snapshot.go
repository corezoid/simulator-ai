package simulation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"time"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"gopkg.in/yaml.v3"
)

// systemForms are graph-of-graph placements, not domain actors.
var systemForms = map[string]bool{"Layers": true, "Graphs": true}

// unreadable says the caller may not read an actor's accounts: the layer lists actors it cannot view.
func unreadable(err error) bool {
	var e *publicapi.APIError
	if !errors.As(err, &e) {
		return false
	}
	switch e.Reason {
	case "accessDenied", "notFound", "noFoundOrNoAccess":
		return true
	}
	return false
}

func jsonNum(n json.Number) any {
	if n == "" {
		return nil
	}
	if r, ok := parseNum(n.String()); ok {
		return r
	}
	return n.String()
}

// Snapshot reads a layer with its actors, links and account values. With period > 0 the
// values are the turnover of the last period instead of the balance.
func Snapshot(ctx context.Context, c *publicapi.Client, layerID string, period time.Duration) (*Graph, error) {
	nodes, err := c.SimulationNodes(ctx, layerID)
	if err != nil {
		return nil, fmt.Errorf("read layer nodes: %w", err)
	}
	edges, err := c.SimulationEdges(ctx, layerID)
	if err != nil {
		return nil, fmt.Errorf("read layer edges: %w", err)
	}
	types, err := c.EdgeTypeNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("read edge types: %w", err)
	}
	g := layerGraph(layerID, nodes, edges, types)
	g.Source.Set("workspace", c.Workspace())
	var from, to int64
	if period > 0 {
		g.Source.Set("period_seconds", ratInt(int64(period.Seconds())))
		now := time.Now()
		from, to = now.Add(-period).UnixMilli(), now.UnixMilli()
	}
	var skipped []any
	for _, a := range append([]*Actor(nil), g.actors...) {
		rows, err := c.SimulationAccounts(ctx, a.ID, from, to)
		if err != nil {
			if unreadable(err) {
				skipped = append(skipped, a.Title)
				continue
			}
			return nil, fmt.Errorf("read accounts of %s: %w", a.Title, err)
		}
		readAccounts(g, a, rows)
	}
	if len(skipped) > 0 {
		g.Source.Set("accounts_not_readable", skipped)
	}
	return g, nil
}

// layerGraph builds the snapshot graph from the layer's nodes and edges.
func layerGraph(layerID string, nodes []publicapi.SimulationNode, edges []publicapi.SimulationEdge, edgeTypes map[int64]string) *Graph {
	// Graph order drives the random draws, and the layer endpoints return elements in
	// different orders, so a snapshot lists actors and links sorted by id (model format §Graph).
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	g := newGraph()
	g.Source.Set("kind", "simulator")
	g.Source.Set("layer", layerID)
	g.Source.Set("taken_at", ratInt(time.Now().Unix()))
	var hidden []any
	for _, n := range nodes {
		if n.AccessDenied || n.Actor == nil {
			hidden = append(hidden, n.ID) // a placement the caller may not view carries no actor
			continue
		}
		if g.actorIdx[n.ID] != nil || systemForms[n.Actor.FormTitle] {
			continue // one actor placed twice is one actor
		}
		g.addActor(nodeActor(n))
	}
	if len(hidden) > 0 {
		g.Source.Set("actors_not_readable", hidden)
	}
	for _, e := range edges {
		// an edge placed twice on the layer is one link; edges to system actors are dropped
		if e.Edge == nil || g.linkIdx[e.ID] != nil || g.actorIdx[e.Edge.Source] == nil || g.actorIdx[e.Edge.Target] == nil {
			continue
		}
		et := edgeTypes[e.Edge.EdgeTypeID]
		if et == "" {
			et = hierarchy
		}
		g.addLink(&Link{ID: e.ID, Source: e.Edge.Source, Target: e.Edge.Target, EdgeType: et, Mediator: e.Edge.LinkedActorID})
	}
	return g
}

// nodeActor is the simulation actor of a readable node placement.
func nodeActor(n publicapi.SimulationNode) *Actor {
	var raw map[string]any
	d := json.NewDecoder(bytes.NewReader(n.Actor.Data))
	d.UseNumber()
	_ = d.Decode(&raw) // absent or null data is an empty map
	data, _ := fromJSON(anyMap(raw)).(*OMap)
	if data == nil {
		data = NewOMap()
	}
	data.Set("_form_id", jsonNum(n.Actor.FormID))
	if n.Position != nil {
		pos := NewOMap()
		pos.Set("x", jsonNum(n.Position.X))
		pos.Set("y", jsonNum(n.Position.Y))
		data.Set("_position", pos)
	}
	typ := n.Actor.FormTitle
	if typ == "" {
		typ = n.Actor.FormID.String()
	}
	return &Actor{ID: n.ID, Type: typ, Title: n.Actor.Title, Data: data, OriginID: n.ID}
}

func anyMap(m map[string]any) any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// readAccounts folds Simulator's debit/credit pair per (account name, currency) into one value.
func readAccounts(g *Graph, a *Actor, rows []publicapi.SimulationAccount) {
	type pair struct {
		name, currency, nameID, currencyID, formula string
		value                                       *big.Rat
	}
	var order []string
	pairs := map[string]*pair{}
	names := map[string]int{}
	for _, r := range rows {
		if r.Meta.IsSystem || (r.Type != "" && r.Type != "fact") {
			continue
		}
		key := r.NameID + "|" + r.CurrencyID.String()
		p := pairs[key]
		if p == nil {
			name := r.Meta.AccountName
			if name == "" {
				name = r.NameID
			}
			p = &pair{name: name, currency: r.Meta.CurrencyName, nameID: r.NameID, currencyID: r.CurrencyID.String(), formula: r.Formula, value: new(big.Rat)}
			pairs[key] = p
			order = append(order, key)
			names[name]++
		}
		amt, _ := parseNum(r.Amount.String())
		if amt == nil {
			amt = new(big.Rat)
		}
		if r.IncomeType == "debit" {
			p.value.Sub(p.value, amt)
		} else {
			p.value.Add(p.value, amt)
		}
	}
	meta := NewOMap()
	for _, key := range order {
		p := pairs[key]
		vt := p.currency
		if vt == "" {
			vt = "value"
		}
		if _, ok := g.valueTypes[vt]; !ok {
			g.valueTypes[vt] = ValueType{Name: vt, Scale: 8} // Simulator stores Numeric(100,8)
		}
		name := p.name
		if names[p.name] > 1 {
			name = fmt.Sprintf("%s [%s]", p.name, p.currency)
		}
		q, _ := g.valueTypes[vt].quantize(p.value)
		g.addAccount(&Account{ActorID: a.ID, Name: name, ValueType: vt, Value: q})
		m := NewOMap()
		m.Set("nameId", p.nameID)
		m.Set("currencyId", p.currencyID)
		m.Set("currency", p.currency)
		if p.formula != "" {
			m.Set("formula", p.formula) // the value is computed from other accounts; the run does not recompute it
		}
		meta.Set(name, m)
	}
	if meta.Len() > 0 {
		a.Data.Set("_accounts", meta)
	}
}

// GraphFile renders g in the plugin's layer YAML with sim: sections (spec §1).
func GraphFile(g *Graph, layerID string) ([]byte, error) {
	type pos struct {
		X any `yaml:"x"`
		Y any `yaml:"y"`
	}
	var actors []map[string]any
	for _, a := range g.actors {
		public, internal := map[string]any{}, map[string]any{}
		for _, k := range a.Data.Keys() {
			v := toYAML(a.Data.m[k])
			if len(k) > 0 && k[0] == '_' {
				internal[k] = v
			} else {
				public[k] = v
			}
		}
		entry := map[string]any{"id": a.ID, "title": a.Title, "formName": a.Type,
			"sim": map[string]any{"type": a.Type, "origin_id": nilIfEmpty(a.OriginID), "created_by": nilIfEmpty(a.CreatedBy), "state": internal}}
		if f, ok := a.Data.Get("_form_id"); ok && f != nil {
			entry["formId"] = toYAML(f)
		}
		if len(public) > 0 {
			entry["data"] = public
		}
		p := pos{0, 0}
		if pm, ok := a.Data.m["_position"].(*OMap); ok {
			x, _ := pm.Get("x")
			y, _ := pm.Get("y")
			p = pos{toYAML(x), toYAML(y)}
		}
		entry["position"] = p
		actors = append(actors, entry)
	}
	var edges []map[string]any
	for _, l := range g.links {
		edges = append(edges, map[string]any{"source": l.Source, "target": l.Target,
			"sim": map[string]any{"id": l.ID, "edgeType": l.EdgeType, "mediator": nilIfEmpty(l.Mediator)}})
	}
	vts := map[string]any{}
	for name, vt := range g.valueTypes {
		kind := "decimal"
		if vt.Integer {
			kind = "integer"
		}
		spec := map[string]any{"kind": kind, "scale": vt.Scale, "conserved": vt.Conserved, "min": nil, "max": nil}
		if vt.Min != nil {
			spec["min"] = numString(vt.Min)
		}
		if vt.Max != nil {
			spec["max"] = numString(vt.Max)
		}
		vts[name] = spec
	}
	var accounts []map[string]any
	for _, a := range g.accounts {
		accounts = append(accounts, map[string]any{"actor_id": a.ActorID, "name": a.Name, "value_type": a.ValueType, "value": numString(a.Value)})
	}
	if layerID == "" {
		layerID = getString(g.Source, "layer")
	}
	return yaml.Marshal(map[string]any{"layerId": layerID, "actors": actors, "edges": edges,
		"sim": map[string]any{"format": "sim-morrow/1", "source": toYAML(g.Source), "valueTypes": vts, "accounts": accounts}})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// unnamedFormTypes returns, per form id, the actors whose type is a bare form id: a
// pullGraphFile output has formId but no formName, so its types are numbers.
func unnamedFormTypes(g *Graph) map[string][]*Actor {
	out := map[string][]*Actor{}
	for _, a := range g.actors {
		if f, ok := a.Data.Get("_form_id"); ok && f != nil && a.Type != "" && a.Type == show(f) {
			out[a.Type] = append(out[a.Type], a)
		}
	}
	return out
}

// nameFormTypes replaces bare form-id types with form titles read from Simulator
// (public form/get). Forms that cannot be read keep their id. Returns how many actors
// got a title.
func nameFormTypes(ctx context.Context, c *publicapi.Client, g *Graph) int {
	n := 0
	for id, actors := range unnamedFormTypes(g) {
		formID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			continue
		}
		form, err := c.GetForm(ctx, formID)
		if err != nil || form.Title == nil || *form.Title == "" {
			continue
		}
		for _, a := range actors {
			a.Type = *form.Title
			n++
		}
	}
	return n
}
