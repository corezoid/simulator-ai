package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// LayerCreateInput creates a layer: an actor of the workspace's Layers system template.
type LayerCreateInput struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// EdgeCreateInput is the console subset of public edge/create; the edge type is given by name.
type EdgeCreateInput struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	EdgeType   string `json:"edgeType,omitempty"`
	Name       string `json:"name,omitempty"`
	CurveStyle string `json:"curveStyle,omitempty"`
	// Hole creates a placeholder edge; a plain edge to a hole actor is not one.
	Hole bool `json:"hole,omitempty"`
}

type edgeCreateRequest struct {
	AccID      string `json:"accId"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	EdgeTypeID int64  `json:"edgeTypeId"`
	Name       string `json:"name,omitempty"`
	CurveStyle string `json:"curveStyle,omitempty"`
	Hole       bool   `json:"hole,omitempty"`
	// ForceDirection keeps a flowchart loop B->A apart from an existing hierarchy edge A->B.
	ForceDirection bool `json:"forceDirection"`
}

// NodePlacement places an existing actor; LayerSettings is set for native flowchart blocks, whose
// Color paints the AreaPicture the console tool derives from the block shape (never tool input).
type NodePlacement struct {
	ActorID  string     `json:"actorId"`
	Position GraphPoint `json:"position"`
	// Cell names the position as the Graph Editor rulers do ("C, 7"), in place of position.
	Cell          string               `json:"cell,omitempty"`
	LayerSettings *GraphNativeSettings `json:"layerSettings,omitempty"`
	Color         string               `json:"color,omitempty"`
	AreaPicture   *GraphAreaPicture    `json:"-"`
	// Expand places the node open in place (its layer, chart or Smart Form), as the Graph Editor
	// places a chart or a Smart Form.
	Expand bool `json:"expand,omitempty"`
	// openAs is the nested kind when the caller knows it (chart_create), saving the actor read.
	openAs string
}

// EdgePlacement draws an existing edge between two node placements of the same layer.
type EdgePlacement struct {
	EdgeID     string `json:"edgeId"`
	LaIDSource int64  `json:"laIdSource"`
	LaIDTarget int64  `json:"laIdTarget"`
}

type PlacementsCreateInput struct {
	LayerID string          `json:"layerId"`
	Nodes   []NodePlacement `json:"nodes,omitempty"`
	Edges   []EdgePlacement `json:"edges,omitempty"`
}

type layerManageRequest struct {
	AccID   string               `json:"accId"`
	ActorID string               `json:"actorId"`
	Items   []GraphPlacementItem `json:"items"`
}

// PositionUpdate moves one node placement (laId) to x/y in graph units, or to the centre of a cell.
type PositionUpdate struct {
	LaID int64   `json:"laId"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Cell string  `json:"cell,omitempty"`
}

// atCell turns a cell into the position it names; a cell and a position together are refused.
func atCell(cell string, p *GraphPoint) error {
	if cell == "" {
		return nil
	}
	if *p != (GraphPoint{}) {
		return errors.New("send a position or a cell, not both")
	}
	at, err := CellCenter(cell)
	*p = at
	return err
}

// ResolveCell sets the position of a placement given by its cell.
func (p *NodePlacement) ResolveCell() error { return atCell(p.Cell, &p.Position) }

type PositionsUpdateInput struct {
	LayerID string           `json:"layerId"`
	Items   []PositionUpdate `json:"items"`
}

type positionItem struct {
	LaActorID int64      `json:"laActorId"`
	Position  GraphPoint `json:"position"`
}
type positionsRequest struct {
	AccID   string         `json:"accId"`
	ActorID string         `json:"actorId"`
	Items   []positionItem `json:"items"`
}

// PositionsUpdated reports how many placements graph/layer/actors/positions/update moved.
type PositionsUpdated struct {
	Updated int `json:"updated"`
}

type LayerGetInput struct {
	LayerID string `json:"layerId"`
	Limit   *int64 `json:"limit,omitempty"`
	Offset  int64  `json:"offset,omitempty"`
	// Full answers every row as before: positions and edge endpoints' actor ids.
	Full bool `json:"full,omitempty"`
}

// LayerViewNode is a compact node placement for readback: the laId edges are drawn between.
type LayerViewNode struct {
	LaID     int64     `json:"laId"`
	ActorID  string    `json:"actorId"`
	Title    *string   `json:"title,omitempty"`
	FormID   *int64    `json:"formId,omitempty"`
	Position *Position `json:"position,omitempty"`
	Cell     string    `json:"cell,omitempty"` // the position's Graph Editor cell, "C, 7"
	BlockID  string    `json:"blockId,omitempty"`
	// Hole marks a placeholder actor; ClosedHole a placement that closed one.
	Hole       bool        `json:"hole,omitempty"`
	ClosedHole *ClosedHole `json:"closedHole,omitempty"`
	// Open is set while the node is open in place: what it shows and the box it covers.
	Open *LayerNodeOpen `json:"open,omitempty"`
}

// LayerNodeOpen is an open node's state: expandType (layer, chart, script, ...) and the box it
// covers by corner cells.
type LayerNodeOpen struct {
	ExpandType string `json:"expandType"`
	Area       string `json:"area"`
}

// ClosedHole follows graph/layer/actors/list closedHole: the hole a placement closed and, for
// conditions, how many matched.
type ClosedHole struct {
	HoleActorID     string `json:"holeActorId"`
	ClosingProgress *struct {
		Matched int `json:"matched"`
		Total   int `json:"total"`
	} `json:"closingProgress,omitempty"`
}

// LayerViewEdge is a compact edge placement with the edge's own endpoints and label.
type LayerViewEdge struct {
	LaID       int64   `json:"laId"`
	EdgeID     string  `json:"edgeId"`
	LaIDSource *int64  `json:"laIdSource,omitempty"`
	LaIDTarget *int64  `json:"laIdTarget,omitempty"`
	Source     string  `json:"source,omitempty"`
	Target     string  `json:"target,omitempty"`
	Name       *string `json:"name,omitempty"`
	// Hole marks a placeholder edge; ClosedHole is the edge hole this placement closed.
	Hole       bool   `json:"hole,omitempty"`
	ClosedHole string `json:"closedHole,omitempty"`
}

// LayerView is one page of node and one page of edge placements; HasMore when either page is full.
// Anisotropy is the layer's setting: on, a node that grows or lands on a taken area moves the rest.
type LayerView struct {
	Nodes      []LayerViewNode `json:"nodes"`
	Edges      []LayerViewEdge `json:"edges"`
	HasMore    bool            `json:"hasMore"`
	Anisotropy bool            `json:"anisotropy"`
	// Total counts the layer's placements when this page does not hold them all.
	Total *LayerTotals `json:"total,omitempty"`
}

type layerPageRequest struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	Limit   int64    `json:"limit"`
	Offset  int64    `json:"offset"`
	Fields  []string `json:"fields,omitempty"`
	With    []string `json:"with,omitempty"`
}
type layerNodeWire struct {
	LaID          int64                  `json:"laId"`
	ID            string                 `json:"id"`
	Position      *Position              `json:"position"`
	AccessDenied  bool                   `json:"accessDenied"`
	LayerSettings *placementSettingsWire `json:"layerSettings"`
	Actor         *Actor                 `json:"actor"`
	Hole          bool                   `json:"hole"`
	ClosedHole    *ClosedHole            `json:"closedHole"`
}
type layerNodesWire struct {
	List []layerNodeWire `json:"list"`
}

// layerEdgeWire follows graph/layer/edges/list rows, with the hole marks layer_get passes on.
type layerEdgeWire struct {
	LaID          int64                     `json:"laId"`
	ID            string                    `json:"id"`
	LaIDSource    *int64                    `json:"laIdSource"`
	LaIDTarget    *int64                    `json:"laIdTarget"`
	LayerSettings *GraphElementEdgeSettings `json:"layerSettings"`
	Edge          *struct {
		Source string  `json:"source"`
		Target string  `json:"target"`
		Name   *string `json:"name"`
		Hole   bool    `json:"hole"`
	} `json:"edge"`
	ClosedHole *struct {
		HoleEdgeID string `json:"holeEdgeId"`
	} `json:"closedHole"`
}
type layerEdgesWire struct {
	List []layerEdgeWire `json:"list"`
}

type systemFormRequest struct {
	AccID string `json:"accId"`
	Title string `json:"title"`
}
type edgeTypeRequest struct {
	AccID string `json:"accId"`
	Name  string `json:"name"`
}
type edgeTypeWire struct {
	ID int64 `json:"id"`
}

// CreateLayer creates a layer actor; place actors on it with CreatePlacements.
func (c *Client) CreateLayer(ctx context.Context, in LayerCreateInput) (Actor, error) {
	if e := consoleOnly(c); e != nil {
		return Actor{}, e
	}
	formID, e := c.systemFormID(ctx, "Layers")
	if e != nil {
		return Actor{}, e
	}
	return c.CreateActor(ctx, ActorCreateInput{FormID: formID, Title: in.Title, Description: in.Description})
}

// CreateEdge links two actors with an edge of the named type (default hierarchy).
func (c *Client) CreateEdge(ctx context.Context, in EdgeCreateInput) (GraphCreatedEdge, error) {
	if e := consoleOnly(c); e != nil {
		return GraphCreatedEdge{}, e
	}
	if !validID(in.Source) || !validID(in.Target) || utf8.RuneCountInString(in.Name) > 1000 {
		return GraphCreatedEdge{}, errors.New("source and target actor ids required")
	}
	name := in.EdgeType
	if name == "" {
		name = "hierarchy"
	}
	t, e := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.config.Workspace, Name: name})
	if e != nil {
		return GraphCreatedEdge{}, e
	}
	r, e := call[edgeCreateRequest, GraphCreatedEdge](ctx, c, "edge/create", edgeCreateRequest{AccID: c.config.Workspace, Source: in.Source, Target: in.Target,
		EdgeTypeID: t.ID, Name: in.Name, CurveStyle: in.CurveStyle, Hole: in.Hole, ForceDirection: true})
	if e == nil && (validateGraphEdge(r) != nil || r.Source != in.Source || r.Target != in.Target) {
		return GraphCreatedEdge{}, errors.New("edge/create returned an edge with other endpoints")
	}
	return r, e
}

// CreatePlacements places nodes and/or edges on a layer in one transaction. Nodes are checked for
// room first (layer_place.go): an occupied area is refused, or on a layer with anisotropy made room
// for in the same atomic batch.
func (c *Client) CreatePlacements(ctx context.Context, in PlacementsCreateInput) (PlacementsCreated, error) {
	return c.placeNodes(ctx, in, nil)
}

// placeNodes places with the room plan given (made before the nodes' actors existed), or makes one.
func (c *Client) placeNodes(ctx context.Context, in PlacementsCreateInput, given *roomPlan) (PlacementsCreated, error) {
	if e := consoleOnly(c); e != nil {
		return PlacementsCreated{}, e
	}
	n := len(in.Nodes) + len(in.Edges)
	if !validID(in.LayerID) || n < 1 || n > 100 {
		return PlacementsCreated{}, errors.New("layerId and 1..100 node or edge placements required")
	}
	if err := in.validBornOpen(); err != nil {
		return PlacementsCreated{}, err
	}
	items := make([]GraphPlacementItem, 0, n)
	for _, p := range in.Nodes {
		if !validID(p.ActorID) {
			return PlacementsCreated{}, errors.New("node placement needs actorId")
		}
		pos := p.Position
		items = append(items, GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: p.ActorID, Type: "node", Position: &pos, LayerSettings: p.LayerSettings, AreaPicture: p.AreaPicture}})
	}
	for _, p := range in.Edges {
		if !validID(p.EdgeID) || p.LaIDSource < 1 || p.LaIDTarget < 1 {
			return PlacementsCreated{}, errors.New("edge placement needs edgeId, laIdSource and laIdTarget")
		}
		src, dst := p.LaIDSource, p.LaIDTarget
		items = append(items, GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: p.EdgeID, Type: "edge", LaIDSource: &src, LaIDTarget: &dst}})
	}
	var plan roomPlan
	if given != nil {
		plan = *given
	} else if len(in.Nodes) > 0 {
		var err error
		if plan, err = c.planRoom(ctx, in); err != nil {
			return PlacementsCreated{}, err
		}
	}
	var r GraphCreatedPlacements
	var e error
	if len(plan.directives) == 0 && plan.open == nil {
		r, e = call[layerManageRequest, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", layerManageRequest{AccID: c.config.Workspace, ActorID: in.LayerID, Items: items})
	} else {
		r, e = c.placeWithRoom(ctx, in, items, plan)
	}
	if e == nil && (len(r.Nodes) != len(in.Nodes) || len(r.Edges) != len(in.Edges)) {
		e = errors.New("graph/layer/actors/manage returned a different number of placements")
	}
	if e != nil {
		return PlacementsCreated{}, e
	}
	return PlacementsCreated{GraphCreatedPlacements: r, Moved: plan.moved, Opened: plan.opened, Covers: plan.covers, Warning: plan.warning}, nil
}

// UpdatePositions moves node placements on a layer; all or none are applied.
func (c *Client) UpdatePositions(ctx context.Context, in PositionsUpdateInput) (PositionsUpdated, error) {
	if e := consoleOnly(c); e != nil {
		return PositionsUpdated{}, e
	}
	if !validID(in.LayerID) || len(in.Items) < 1 || len(in.Items) > 100 {
		return PositionsUpdated{}, errors.New("layerId and 1..100 positions required")
	}
	items := make([]positionItem, len(in.Items))
	for i, p := range in.Items {
		if p.LaID < 1 {
			return PositionsUpdated{}, errors.New("positive laId required")
		}
		at := GraphPoint{X: p.X, Y: p.Y}
		if err := atCell(p.Cell, &at); err != nil {
			return PositionsUpdated{}, fmt.Errorf("item %d: %w", i, err)
		}
		items[i] = positionItem{LaActorID: p.LaID, Position: at}
	}
	_, e := call[positionsRequest, json.RawMessage](ctx, c, "graph/layer/actors/positions/update", positionsRequest{AccID: c.config.Workspace, ActorID: in.LayerID, Items: items})
	if e != nil {
		return PositionsUpdated{}, e
	}
	return PositionsUpdated{Updated: len(items)}, nil
}

// GetLayer reads one page of node placements and one page of edge placements of a layer.
func (c *Client) GetLayer(ctx context.Context, in LayerGetInput) (LayerView, error) {
	if e := consoleOnly(c); e != nil {
		return LayerView{}, e
	}
	limit := int64(layerGetCompactLimit)
	if in.Full {
		limit = layerGetFullLimit
	}
	if in.Limit != nil {
		limit = *in.Limit
	}
	if !validID(in.LayerID) || limit < 1 || limit > 500 || in.Offset < 0 {
		return LayerView{}, errors.New("layerId required, limit 1..500 and offset >=0")
	}
	aniso, e := c.layerAnisotropy(ctx, in.LayerID)
	if e != nil {
		return LayerView{}, e
	}
	q := layerPageRequest{AccID: c.config.Workspace, ActorID: in.LayerID, Limit: limit, Offset: in.Offset}
	edges, e := call[layerPageRequest, layerEdgesWire](ctx, c, "graph/layer/edges/list", q)
	if e != nil {
		return LayerView{}, e
	}
	q.Fields = []string{"id", "title", "formId"}
	nodes, e := call[layerPageRequest, layerNodesWire](ctx, c, "graph/layer/actors/list", q)
	if e != nil {
		return LayerView{}, e
	}
	v := LayerView{Nodes: make([]LayerViewNode, 0, len(nodes.List)), Edges: make([]LayerViewEdge, 0, len(edges.List)),
		HasMore: int64(len(nodes.List)) >= limit || int64(len(edges.List)) >= limit, Anisotropy: aniso}
	for _, n := range nodes.List {
		if n.LaID < 1 || strings.TrimSpace(n.ID) == "" {
			return LayerView{}, errors.New("invalid node placement identity")
		}
		node := LayerViewNode{LaID: n.LaID, ActorID: n.ID, Position: n.Position, Hole: n.Hole, ClosedHole: n.ClosedHole}
		if n.Position != nil && n.Position.X != nil && n.Position.Y != nil {
			node.Cell = CellOf(GraphPoint{X: *n.Position.X, Y: *n.Position.Y})
		}
		if s := n.LayerSettings; s != nil {
			node.BlockID = s.BlockID
			if p := node.Position; s.Expand && s.BlockID == "" && p != nil && p.X != nil && p.Y != nil {
				open := openNode{X: *p.X, Y: *p.Y, SizeGrade: s.SizeGrade, Settings: s.openSettings}
				node.Open = &LayerNodeOpen{ExpandType: s.ExpandType, Area: areaCells(open.box())}
			}
		}
		if n.Actor != nil && !n.AccessDenied {
			node.Title, node.FormID = n.Actor.Title, n.Actor.FormID
		}
		v.Nodes = append(v.Nodes, node)
	}
	for _, p := range edges.List {
		if p.LaID < 1 || strings.TrimSpace(p.ID) == "" {
			return LayerView{}, errors.New("invalid edge placement identity")
		}
		edge := LayerViewEdge{LaID: p.LaID, EdgeID: p.ID, LaIDSource: p.LaIDSource, LaIDTarget: p.LaIDTarget}
		if p.Edge != nil {
			edge.Source, edge.Target, edge.Name, edge.Hole = p.Edge.Source, p.Edge.Target, p.Edge.Name, p.Edge.Hole
		}
		if p.ClosedHole != nil {
			edge.ClosedHole = p.ClosedHole.HoleEdgeID
		}
		if p.LayerSettings != nil && p.LayerSettings.Name != nil && *p.LayerSettings.Name != "" {
			edge.Name = p.LayerSettings.Name
		}
		v.Edges = append(v.Edges, edge)
	}
	if !in.Full {
		compactLayerView(&v)
	}
	if v.HasMore {
		if v.Total, e = c.layerTotals(ctx, in.LayerID); e != nil {
			return LayerView{}, e
		}
	}
	return v, nil
}
