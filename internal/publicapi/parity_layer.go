package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// maxLayerPages bounds a whole-layer read: 20 pages of 1000 placements.
const maxLayerPages = 20

// LayerPlacement is one node placement of a whole-layer read (legacy getAllLayerPlacements).
type LayerPlacement struct {
	LaID     int64     `json:"laId"`
	ActorID  string    `json:"actorId"`
	Title    *string   `json:"title,omitempty"`
	FormID   *int64    `json:"formId,omitempty"`
	Position *Position `json:"position,omitempty"`
}

// AllNodePlacements reads every node placement of a layer. A page shorter than the limit is not the
// last (sim-api drops deleted actors after paging), so it reads until an empty page.
func (c *Client) AllNodePlacements(ctx context.Context, layerID string) ([]LayerPlacement, error) {
	if !c.ParityAllowed("graph/layer/actors/list") {
		return nil, errors.New("graph/layer/actors/list is not available in this profile")
	}
	if !validID(layerID) {
		return nil, errors.New("layerId required")
	}
	out := []LayerPlacement{}
	q := layerPageRequest{AccID: c.config.Workspace, ActorID: layerID, Limit: 1000, Fields: []string{"id", "title", "formId"}}
	for page := 0; page < maxLayerPages; page++ {
		nodes, err := call[layerPageRequest, layerNodesWire](ctx, c, "graph/layer/actors/list", q)
		if err != nil {
			return nil, err
		}
		if len(nodes.List) == 0 {
			return out, nil
		}
		for _, n := range nodes.List {
			if n.LaID < 1 || strings.TrimSpace(n.ID) == "" {
				return nil, errors.New("invalid node placement identity")
			}
			p := LayerPlacement{LaID: n.LaID, ActorID: n.ID, Position: n.Position}
			if n.Actor != nil && !n.AccessDenied {
				p.Title, p.FormID = n.Actor.Title, n.Actor.FormID
			}
			out = append(out, p)
		}
		q.Offset += q.Limit
	}
	return nil, fmt.Errorf("layer has more than %d node placements", maxLayerPages*1000)
}

// AllEdgePlacements reads every edge placement of a layer, like AllNodePlacements.
func (c *Client) AllEdgePlacements(ctx context.Context, layerID string) ([]LayerViewEdge, error) {
	if !c.ParityAllowed("graph/layer/edges/list") {
		return nil, errors.New("graph/layer/edges/list is not available in this profile")
	}
	if !validID(layerID) {
		return nil, errors.New("layerId required")
	}
	out := []LayerViewEdge{}
	q := layerPageRequest{AccID: c.config.Workspace, ActorID: layerID, Limit: 1000}
	for page := 0; page < maxLayerPages; page++ {
		edges, err := call[layerPageRequest, layerEdgesWire](ctx, c, "graph/layer/edges/list", q)
		if err != nil {
			return nil, err
		}
		if len(edges.List) == 0 {
			return out, nil
		}
		for _, p := range edges.List {
			if p.LaID < 1 || strings.TrimSpace(p.ID) == "" {
				return nil, errors.New("invalid edge placement identity")
			}
			e := LayerViewEdge{LaID: p.LaID, EdgeID: p.ID, LaIDSource: p.LaIDSource, LaIDTarget: p.LaIDTarget}
			if p.Edge != nil {
				e.Source, e.Target, e.Name = p.Edge.Source, p.Edge.Target, p.Edge.Name
			}
			out = append(out, e)
		}
		q.Offset += q.Limit
	}
	return nil, fmt.Errorf("layer has more than %d edge placements", maxLayerPages*1000)
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
