package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Placing nodes where the layer has room. A new node must not land on what another one holds (its
// cell, a flowchart block's box, an open node's box), and two nodes of one call must not overlap.
// On a layer with anisotropy the layer makes room instead: one graph/layer/actors/offset grows the
// new nodes' area from a seam (insertDirective) before they are placed, in the same atomic batch.
// A node placed with expand is born open, as the Graph Editor places a chart or a Smart Form: its
// bag rides the create and, with anisotropy, its neighbours are pushed out of its box.

// PlacementsCreated is graph/layer/actors/manage's answer with what placing did around it.
type PlacementsCreated struct {
	GraphCreatedPlacements
	// Moved is how many placements the make-room moved.
	Moved int `json:"moved,omitempty"`
	// Opened is the open state of a node placed with expand; Covers what its box lies over when the
	// layer does not make room.
	Opened *LayerNodeOpen `json:"opened,omitempty"`
	Covers string         `json:"covers,omitempty"`
	// Warning says what could not be checked.
	Warning string `json:"warning,omitempty"`
}

// newFootprint is what a node about to be placed will hold.
func (p NodePlacement) newFootprint() graphRect {
	if s := p.LayerSettings; s != nil && s.Width > 0 && s.Height > 0 {
		return graphRect{X: p.Position.X - s.Width/2, Y: p.Position.Y - s.Height/2, W: s.Width, H: s.Height}
	}
	return cellOf(p.Position.X, p.Position.Y)
}

// roomPlan is what placing the nodes needs first: the directives, the bag of a node born open.
type roomPlan struct {
	directives []OffsetDirective
	moved      int
	open       *openPatch
	opened     *LayerNodeOpen
	covers     string
	warning    string
}

// planRoom checks the new nodes against each other and the layer, and plans the room they need.
func (c *Client) planRoom(ctx context.Context, in PlacementsCreateInput) (roomPlan, error) {
	boxes := make([]graphRect, len(in.Nodes))
	for i, p := range in.Nodes {
		if !finite(p.Position.X, p.Position.Y) {
			return roomPlan{}, fmt.Errorf("node %d: position out of range", i)
		}
		boxes[i] = p.newFootprint()
		for j := range i {
			if boxes[j].overlaps(boxes[i]) {
				return roomPlan{}, fmt.Errorf("nodes %d and %d overlap (%s): give each its own cells", j, i, CellOf(in.Nodes[i].Position))
			}
		}
	}
	room, err := c.readLayerRoom(ctx, in.LayerID)
	if err != nil {
		return roomPlan{}, err
	}
	var plan roomPlan
	start := room
	if room.Partial {
		plan.warning = fmt.Sprintf("the layer has more than %d placements: only the first were checked for room", maxRoomPages*layerPage)
	}
	if taken := room.conflicts(boxes, 0); len(taken) > 0 {
		if !room.Anisotropy {
			return roomPlan{}, fmt.Errorf("occupied: %s. The layer's anisotropy is off, so nothing makes room: place on free cells (layer_get shows what is where)", cellsOf(taken))
		}
		area := boxes[0]
		for _, b := range boxes[1:] {
			area = area.union(b)
		}
		d := insertDirective(area)
		after := room.after(d, 0)
		if left := after.conflicts(boxes, 0); d == nil || len(left) > 0 {
			return roomPlan{}, fmt.Errorf("occupied: %s, and making room does not clear %s: place elsewhere", cellsOf(taken), areaCells(area))
		}
		plan.directives, room = append(plan.directives, *d), after
	}
	for i, p := range in.Nodes {
		if !p.Expand {
			continue
		}
		if room, err = c.planBornOpen(ctx, p, room, &plan); err != nil {
			return roomPlan{}, fmt.Errorf("node %d: %w", i, err)
		}
	}
	plan.moved = start.movedTo(room)
	return plan, nil
}

// planBornOpen plans the open bag of a node placed with expand and, with anisotropy, its push; it
// answers the layer after the push. Without anisotropy the box lies over its neighbours: covers.
func (c *Client) planBornOpen(ctx context.Context, p NodePlacement, room layerRoom, plan *roomPlan) (layerRoom, error) {
	kind := p.openAs
	if kind == "" {
		actor, err := call[map[string]any, struct {
			FormID int64 `json:"formId"`
		}](ctx, c, "actor/get", map[string]any{"accId": c.config.Workspace, "actorId": p.ActorID, "fields": []string{"id", "formId"}})
		if err != nil {
			return room, fmt.Errorf("actor/get %s: %w", p.ActorID, err)
		}
		forms, err := c.nestedForms(ctx)
		if err != nil {
			return room, err
		}
		kind = nestedKindOf(actor.FormID, openSettings{}, forms)
	}
	at := cellOf(p.Position.X, p.Position.Y)
	n := openNode{X: at.X + gridCell/2, Y: at.Y + gridCell/2}
	patch, d := expandPlan(n, kind, nil, room.Anisotropy)
	n.Settings = n.Settings.with(patch)
	if d != nil {
		plan.directives, room = append(plan.directives, *d), room.after(d, 0)
	}
	plan.open = &patch
	plan.opened = &LayerNodeOpen{ExpandType: patch.ExpandType, Area: areaCells(n.box())}
	if covered := room.conflicts([]graphRect{n.box()}, 0); len(covered) > 0 {
		plan.covers = cellsOf(covered)
	}
	return room, nil
}

// bornOpenItem creates a node placement with its open bag.
type bornOpenItem struct {
	Action string `json:"action"`
	Data   struct {
		ID            string     `json:"id"`
		Type          string     `json:"type"`
		Position      GraphPoint `json:"position"`
		LayerSettings openPatch  `json:"layerSettings"`
	} `json:"data"`
}

type layerManageAnyRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
	Items   []any  `json:"items"`
}

// placeWithRoom sends the make-room directives and the placements in one atomic batch.
func (c *Client) placeWithRoom(ctx context.Context, in PlacementsCreateInput, items []GraphPlacementItem, plan roomPlan) (GraphCreatedPlacements, error) {
	manage := layerManageAnyRequest{AccID: c.config.Workspace, ActorID: in.LayerID, Items: make([]any, len(items))}
	for i, item := range items {
		manage.Items[i] = item
		if plan.open != nil && i < len(in.Nodes) && in.Nodes[i].Expand {
			open := bornOpenItem{Action: "create"}
			open.Data.ID, open.Data.Type, open.Data.Position, open.Data.LayerSettings = item.Data.ID, "node", *item.Data.Position, *plan.open
			manage.Items[i] = open
		}
	}
	if len(plan.directives) == 0 {
		return call[layerManageAnyRequest, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", manage)
	}
	batch := make([]atomicItem, 0, len(plan.directives)+1)
	for _, d := range plan.directives {
		batch = append(batch, atomicItem{Method: "graph/layer/actors/offset", Params: offsetRequest{AccID: c.config.Workspace, ActorID: in.LayerID, OffsetDirective: d}})
	}
	batch = append(batch, atomicItem{Method: "graph/layer/actors/manage", Params: manage})
	results, err := c.atomicItems(ctx, batch)
	if err != nil {
		return GraphCreatedPlacements{}, err
	}
	var r GraphCreatedPlacements
	if err := json.Unmarshal(results[len(results)-1], &r); err != nil {
		return GraphCreatedPlacements{}, fmt.Errorf("graph/layer/actors/manage result: %w", err)
	}
	return r, nil
}

// validBornOpen allows expand on a call's only node, as the Graph Editor opens one node it places.
func (in PlacementsCreateInput) validBornOpen() error {
	for _, p := range in.Nodes {
		if !p.Expand {
			continue
		}
		if len(in.Nodes) != 1 || len(in.Edges) > 0 || p.LayerSettings != nil {
			return errors.New("expand opens a single plain node: place it alone, then its edges")
		}
	}
	return nil
}
