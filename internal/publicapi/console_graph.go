package publicapi

import (
	"context"
	"encoding/json"
	"errors"
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
}

type edgeCreateRequest struct {
	AccID      string `json:"accId"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	EdgeTypeID int64  `json:"edgeTypeId"`
	Name       string `json:"name,omitempty"`
	CurveStyle string `json:"curveStyle,omitempty"`
	// ForceDirection keeps a flowchart loop B->A apart from an existing hierarchy edge A->B.
	ForceDirection bool `json:"forceDirection"`
}

// NodePlacement places an existing actor; LayerSettings is set for native flowchart blocks, whose
// Color paints the AreaPicture the console tool derives from the block shape (never tool input).
type NodePlacement struct {
	ActorID       string               `json:"actorId"`
	Position      GraphPoint           `json:"position"`
	LayerSettings *GraphNativeSettings `json:"layerSettings,omitempty"`
	Color         string               `json:"color,omitempty"`
	AreaPicture   *GraphAreaPicture    `json:"-"`
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

// PositionUpdate moves one node placement (laId) to x/y in graph units.
type PositionUpdate struct {
	LaID int64   `json:"laId"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

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
}

// LayerViewNode is a compact node placement for readback: the laId edges are drawn between.
type LayerViewNode struct {
	LaID     int64     `json:"laId"`
	ActorID  string    `json:"actorId"`
	Title    *string   `json:"title,omitempty"`
	FormID   *int64    `json:"formId,omitempty"`
	Position *Position `json:"position,omitempty"`
	BlockID  string    `json:"blockId,omitempty"`
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
}

// LayerView is one page of node and one page of edge placements; HasMore when either page is full.
type LayerView struct {
	Nodes   []LayerViewNode `json:"nodes"`
	Edges   []LayerViewEdge `json:"edges"`
	HasMore bool            `json:"hasMore"`
}

type layerPageRequest struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	Limit   int64    `json:"limit"`
	Offset  int64    `json:"offset"`
	Fields  []string `json:"fields,omitempty"`
}
type layerNodeWire struct {
	LaID          int64     `json:"laId"`
	ID            string    `json:"id"`
	Position      *Position `json:"position"`
	AccessDenied  bool      `json:"accessDenied"`
	LayerSettings *struct {
		BlockID string `json:"blockId"`
	} `json:"layerSettings"`
	Actor *Actor `json:"actor"`
}
type layerNodesWire struct {
	List []layerNodeWire `json:"list"`
}
type layerEdgesWire struct {
	List []GraphElementEdge `json:"list"`
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
	f, e := call[systemFormRequest, GraphSystemForm](ctx, c, "form/system/get", systemFormRequest{AccID: c.config.Workspace, Title: "Layers"})
	if e != nil {
		return Actor{}, e
	}
	if f.ID < 1 {
		return Actor{}, errors.New("Layers system template not found")
	}
	return c.CreateActor(ctx, ActorCreateInput{FormID: f.ID, Title: in.Title, Description: in.Description})
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
		EdgeTypeID: t.ID, Name: in.Name, CurveStyle: in.CurveStyle, ForceDirection: true})
	if e == nil && (validateGraphEdge(r) != nil || r.Source != in.Source || r.Target != in.Target) {
		return GraphCreatedEdge{}, errors.New("edge/create returned an edge with other endpoints")
	}
	return r, e
}

// CreatePlacements places nodes and/or edges on a layer in one transaction.
func (c *Client) CreatePlacements(ctx context.Context, in PlacementsCreateInput) (GraphCreatedPlacements, error) {
	if e := consoleOnly(c); e != nil {
		return GraphCreatedPlacements{}, e
	}
	n := len(in.Nodes) + len(in.Edges)
	if !validID(in.LayerID) || n < 1 || n > 100 {
		return GraphCreatedPlacements{}, errors.New("layerId and 1..100 node or edge placements required")
	}
	items := make([]GraphPlacementItem, 0, n)
	for _, p := range in.Nodes {
		if !validID(p.ActorID) {
			return GraphCreatedPlacements{}, errors.New("node placement needs actorId")
		}
		pos := p.Position
		items = append(items, GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: p.ActorID, Type: "node", Position: &pos, LayerSettings: p.LayerSettings, AreaPicture: p.AreaPicture}})
	}
	for _, p := range in.Edges {
		if !validID(p.EdgeID) || p.LaIDSource < 1 || p.LaIDTarget < 1 {
			return GraphCreatedPlacements{}, errors.New("edge placement needs edgeId, laIdSource and laIdTarget")
		}
		src, dst := p.LaIDSource, p.LaIDTarget
		items = append(items, GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: p.EdgeID, Type: "edge", LaIDSource: &src, LaIDTarget: &dst}})
	}
	r, e := call[layerManageRequest, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", layerManageRequest{AccID: c.config.Workspace, ActorID: in.LayerID, Items: items})
	if e == nil && (len(r.Nodes) != len(in.Nodes) || len(r.Edges) != len(in.Edges)) {
		return GraphCreatedPlacements{}, errors.New("graph/layer/actors/manage returned a different number of placements")
	}
	return r, e
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
		items[i] = positionItem{LaActorID: p.LaID, Position: GraphPoint{X: p.X, Y: p.Y}}
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
	limit := int64(200)
	if in.Limit != nil {
		limit = *in.Limit
	}
	if !validID(in.LayerID) || limit < 1 || limit > 500 || in.Offset < 0 {
		return LayerView{}, errors.New("layerId required, limit 1..500 and offset >=0")
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
		HasMore: int64(len(nodes.List)) >= limit || int64(len(edges.List)) >= limit}
	for _, n := range nodes.List {
		if n.LaID < 1 || strings.TrimSpace(n.ID) == "" {
			return LayerView{}, errors.New("invalid node placement identity")
		}
		node := LayerViewNode{LaID: n.LaID, ActorID: n.ID, Position: n.Position}
		if n.LayerSettings != nil {
			node.BlockID = n.LayerSettings.BlockID
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
			edge.Source, edge.Target, edge.Name = p.Edge.Source, p.Edge.Target, p.Edge.Name
		}
		if p.LayerSettings != nil && p.LayerSettings.Name != nil && *p.LayerSettings.Name != "" {
			edge.Name = p.LayerSettings.Name
		}
		v.Edges = append(v.Edges, edge)
	}
	return v, nil
}
