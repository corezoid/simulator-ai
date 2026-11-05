package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// A whole-layer read takes pages of 200 (flowchart placements carry a shape and an SVG, so a page of
// 1000 could near the 4 MiB response cap) and at most 20000 placements.
const (
	layerPage     = 200
	maxLayerPages = 100
)

// LayerPlacement is one node placement of a whole-layer read (legacy getAllLayerPlacements).
type LayerPlacement struct {
	LaID     int64     `json:"laId"`
	ActorID  string    `json:"actorId"`
	Title    *string   `json:"title,omitempty"`
	FormID   *int64    `json:"formId,omitempty"`
	Position *Position `json:"position,omitempty"`
	Cell     string    `json:"cell,omitempty"` // the compact answer's Graph Editor cell, "C, 7"
}

// AllNodePlacements reads every node placement of a layer.
func (c *Client) AllNodePlacements(ctx context.Context, layerID string) ([]LayerPlacement, error) {
	q := layerPageRequest{AccID: c.config.Workspace, ActorID: layerID, Fields: []string{"id", "title", "formId"}}
	nodes, err := allLayerPages[layerNodeWire](ctx, c, "graph/layer/actors/list", "node", q)
	if err != nil {
		return nil, err
	}
	out := make([]LayerPlacement, 0, len(nodes))
	for _, n := range nodes {
		if n.LaID < 1 || strings.TrimSpace(n.ID) == "" {
			return nil, errors.New("invalid node placement identity")
		}
		p := LayerPlacement{LaID: n.LaID, ActorID: n.ID, Position: n.Position}
		if n.Actor != nil && !n.AccessDenied {
			p.Title, p.FormID = n.Actor.Title, n.Actor.FormID
		}
		out = append(out, p)
	}
	return out, nil
}

// AllEdgePlacements reads every edge placement of a layer, like AllNodePlacements.
func (c *Client) AllEdgePlacements(ctx context.Context, layerID string) ([]LayerViewEdge, error) {
	q := layerPageRequest{AccID: c.config.Workspace, ActorID: layerID}
	edges, err := allLayerPages[layerEdgeWire](ctx, c, "graph/layer/edges/list", "edge", q)
	if err != nil {
		return nil, err
	}
	out := make([]LayerViewEdge, 0, len(edges))
	for _, p := range edges {
		if p.LaID < 1 || strings.TrimSpace(p.ID) == "" {
			return nil, errors.New("invalid edge placement identity")
		}
		e := LayerViewEdge{LaID: p.LaID, EdgeID: p.ID, LaIDSource: p.LaIDSource, LaIDTarget: p.LaIDTarget}
		if p.Edge != nil {
			e.Source, e.Target, e.Name = p.Edge.Source, p.Edge.Target, p.Edge.Name
		}
		out = append(out, e)
	}
	return out, nil
}

// allLayerPages reads every page of a layer's node or edge placements. A page shorter than the limit
// is not the last (sim-api drops deleted actors after paging), so it reads until an empty page.
func allLayerPages[T any](ctx context.Context, c *Client, method, kind string, q layerPageRequest) ([]T, error) {
	if !c.ParityAllowed(method) {
		return nil, fmt.Errorf("%s is not available in this profile", method)
	}
	if !validID(q.ActorID) {
		return nil, errors.New("layerId required")
	}
	out := []T{}
	q.Limit = layerPage
	for page := 0; page <= maxLayerPages; page++ {
		rows, err := call[layerPageRequest, listWire[T]](ctx, c, method, q)
		if err != nil {
			return nil, err
		}
		if len(rows.List) == 0 {
			return out, nil
		}
		if page == maxLayerPages {
			break
		}
		out = append(out, rows.List...)
		q.Offset += q.Limit
	}
	return nil, fmt.Errorf("layer has more than %d %s placements", maxLayerPages*layerPage, kind)
}

// listWire is a public method's {list: [...]} page.
type listWire[T any] struct {
	List []T `json:"list"`
}

// HierarchyEdgeType is the workspace's hierarchy edge type id: the relation graph walks follow when
// no other type is given, as legacy getRelatedActors did.
func (c *Client) HierarchyEdgeType(ctx context.Context) (int64, error) {
	if !c.ParityAllowed("graph/list") {
		return 0, errors.New("graph/list is not available in this profile")
	}
	t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.config.Workspace, Name: "hierarchy"})
	if err == nil && t.ID < 1 {
		err = errors.New("hierarchy edge type not found")
	}
	return t.ID, err
}
