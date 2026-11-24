package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// OntologyLayout places the patch's new actors on the layer b reads: keys are the new actors in patch
// order, edges the patch's edges between keys. The console server supplies it (graphplan layouts).
type OntologyLayout func(b *GraphBuild, keys []string, edges [][2]string) (map[string]GraphPoint, error)

// structure is phase a: one atomic batch and how to read its answer back.
type structure struct {
	items []atomicItem
	// created maps a new actor's key to its actor/create item.
	created map[string]int
	// placed are the keys this batch places on the layer: new actors, and actors an earlier call
	// of the operation created without a layer.
	placed map[string]bool
	edges  int
	// later are edge placements between actors that were on the layer already: they may be drawn
	// already, so they are placed after the batch, only if missing (as graph_build does).
	later []laterEdge
}

type laterEdge struct {
	edge     int // item index of its edge/create
	src, dst int64
}

// endRef is how an item names an actor: its id, or "$<item>.id" of its actor/create.
func (s *structure) endRef(a *planActor) string {
	if i, ok := s.created[a.in.Key]; ok {
		return "$" + strconv.Itoa(i) + ".id"
	}
	return a.id
}

// buildStructure lays out phase a's batch: actor/create for each new actor, their placements, edge
// /create for every edge (an existing edge is answered unchanged), edge placements, and
// account_actor/create with ignoreIfExist for every account.
func (c *Client) buildStructure(p *ontologyPlan, b *GraphBuild, at map[string]GraphPoint) (*structure, error) {
	s := &structure{created: map[string]int{}, placed: map[string]bool{}}
	ws := c.workspace()
	for _, a := range p.actors {
		if a.status != "create" {
			continue
		}
		color := a.in.Color
		if color == "" {
			color = RandomActorColor()
		}
		s.created[a.in.Key] = len(s.items)
		s.items = append(s.items, atomicItem{Method: "actor/create", Params: actorCreateRequest{AccID: ws, FormID: a.formID, Title: a.title,
			Description: a.desc, Color: color, Data: a.data, Ref: BuildRef(p.in.OperationID, a.in.Key), AddToRecent: notRecent, Fields: []string{"id"}}})
	}
	// laOf is an actor's placement on the layer: a number, "$<manage>.nodes.<n>.laId", or nil.
	laOf := map[string]any{}
	if b != nil {
		place := layerManageRequest{AccID: ws, ActorID: b.LayerID}
		for _, a := range p.actors {
			la, onLayer := b.laIDs[a.id]
			if _, isNew := s.created[a.in.Key]; isNew || a.status == "exists" && !onLayer {
				point, ok := at[a.in.Key]
				if !ok {
					return nil, fmt.Errorf("no place for %q on the layer", a.in.Key)
				}
				laOf[a.in.Key] = "$" + strconv.Itoa(len(s.items)) + ".nodes." + strconv.Itoa(len(place.Items)) + ".laId"
				s.placed[a.in.Key] = true
				place.Items = append(place.Items, GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: s.endRef(a), Type: "node", Position: &point}})
			} else if onLayer {
				laOf[a.in.Key] = la
			}
		}
		if len(place.Items) > 0 {
			s.items = append(s.items, atomicItem{Method: "graph/layer/actors/manage", Params: place, UseRefs: true})
		}
	}
	var drawn []map[string]any
	for _, e := range p.edges {
		from, to := p.byKey[e.from], p.byKey[e.to]
		edgeItem := len(s.items)
		s.items = append(s.items, atomicItem{Method: "edge/create", UseRefs: true, Params: edgeCreateRequest{AccID: ws, Source: s.endRef(from), Target: s.endRef(to),
			EdgeTypeID: e.typeID, Name: e.name, ForceDirection: true}})
		s.edges++
		src, srcOK := laOf[e.from]
		dst, dstOK := laOf[e.to]
		if !srcOK || !dstOK {
			continue // an end off the layer: the edge exists, the layer does not show it
		}
		if !s.placed[e.from] && !s.placed[e.to] {
			s.later = append(s.later, laterEdge{edge: edgeItem, src: src.(int64), dst: dst.(int64)})
			continue
		}
		drawn = append(drawn, map[string]any{"action": "create", "data": map[string]any{"id": "$" + strconv.Itoa(edgeItem) + ".id", "type": "edge", "laIdSource": src, "laIdTarget": dst}})
	}
	if len(drawn) > 0 {
		s.items = append(s.items, atomicItem{Method: "graph/layer/actors/manage", UseRefs: true, Params: map[string]any{"accId": ws, "actorId": b.LayerID, "items": drawn}})
	}
	for _, a := range p.accounts {
		params := map[string]any{"accId": ws, "actorId": s.endRef(p.byKey[a.actor]), "nameId": a.pair.NameID, "currencyId": a.pair.CurrencyID,
			"accountType": a.kind, "ignoreIfExist": true, "search": true}
		s.items = append(s.items, atomicItem{Method: "account_actor/create", Params: params, UseRefs: true})
	}
	if len(s.items) > ontologyBatchItems {
		return nil, fmt.Errorf("the structure needs %d writes and one atomic batch takes %d: split the patch (actors, edges, accounts) or use graph_build for a large graph", len(s.items), ontologyBatchItems)
	}
	return s, nil
}

// applyStructure runs phase a. Nothing to write (a repeated patch) runs nothing.
func (c *Client) applyStructure(ctx context.Context, p *ontologyPlan, s *structure, b *GraphBuild) error {
	if len(s.items) == 0 {
		return nil
	}
	results, err := c.atomicItems(ctx, s.items)
	if err != nil {
		return c.structureError(p, s, err)
	}
	for key, i := range s.created {
		var got buildCreated
		if json.Unmarshal(results[i], &got) != nil || got.ID == "" {
			return errors.New("the structure batch committed but actor/create returned no id: repeat the same patch (its operationId finds what landed)")
		}
		a := p.byKey[key]
		a.id, a.status = got.ID, "created"
	}
	if len(s.later) == 0 {
		return nil
	}
	ids := make([]string, len(s.later))
	for i, l := range s.later {
		var edge buildCreated
		if json.Unmarshal(results[l.edge], &edge) != nil || edge.ID == "" {
			return errors.New("the structure batch committed but edge/create returned no id")
		}
		ids[i] = edge.ID
	}
	return c.drawEdges(ctx, b, s.later, ids)
}

// drawEdges places the edges between actors already on the layer that the layer does not show.
func (c *Client) drawEdges(ctx context.Context, b *GraphBuild, later []laterEdge, ids []string) error {
	run := &buildRun{b: b}
	drawn, err := run.drawnEdges(ctx)
	if err != nil {
		return fmt.Errorf("the structure is written; reading the layer's edges to draw them failed (repeat the patch): %w", err)
	}
	place := layerManageRequest{AccID: c.workspace(), ActorID: b.LayerID}
	for i, l := range later {
		if !drawn[ids[i]] {
			drawn[ids[i]] = true
			src, dst := l.src, l.dst
			place.Items = append(place.Items, GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: ids[i], Type: "edge", LaIDSource: &src, LaIDTarget: &dst}})
		}
	}
	if len(place.Items) == 0 {
		return nil
	}
	if _, err = call[layerManageRequest, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", place); err != nil {
		return fmt.Errorf("the structure is written; drawing edges on the layer failed (repeat the patch): %w", err)
	}
	return nil
}

// structureError names what the atomic batch refused: nothing of the structure was written.
func (c *Client) structureError(p *ontologyPlan, s *structure, err error) error {
	var item *atomicItemError
	if !errors.As(err, &item) {
		if errors.Is(err, errRolledBack) {
			return fmt.Errorf("the structure batch was rolled back, nothing was written: repeat the patch: %w", err)
		}
		return fmt.Errorf("the structure batch got no answer; repeat the same patch (its operationId finds what landed): %w", err)
	}
	what := fmt.Sprintf("item %d (%s)", item.Index, s.items[item.Index].Method)
	for key, i := range s.created {
		if i == item.Index {
			what = fmt.Sprintf("actor %q", key)
			if refTaken(item.Cause) {
				return fmt.Errorf("actor %q: its ref %s is taken: if this operationId already ran, wait a few seconds and repeat; otherwise use a new operationId. Nothing was written", key, BuildRef(p.in.OperationID, key))
			}
		}
	}
	return fmt.Errorf("%s was refused, so nothing of the structure was written: %w", what, item.Cause)
}

// CheckRoom refuses nodes that overlap each other or what the layer holds (see checkRoom).
func (b *GraphBuild) CheckRoom(nodes []BuildNode) error { return b.checkRoom(nodes) }
