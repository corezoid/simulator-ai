package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/corezoid/simulator-ai/contracts"
)

// GraphElementsInput is an exact bounded selection from public graph/layer/elements/list.
// Node/edge laIds use independent namespaces. Workspace and fields are host-controlled.
type GraphElementsInput struct {
	ActorID string  `json:"actorId"`
	Nodes   []int64 `json:"nodes"`
	Edges   []int64 `json:"edges"`
}

// GraphElementSettings projects e_graph.ActorLayerSettings and its native Extra keys.
// Unrelated drawing/HTML/account configuration is deliberately omitted.
type GraphElementSettings struct {
	Width     *float64        `json:"width,omitempty"`
	Height    *float64        `json:"height,omitempty"`
	BlockID   string          `json:"blockId,omitempty"`
	Shape     json.RawMessage `json:"shape,omitempty"`
	TextFrame *GraphFrame     `json:"textFrame,omitempty"`
}

// GraphElementNode projects rpc_graph.layerNodeRes, with metadata-only Actor.
type GraphElementNode struct {
	LaID          int64                 `json:"laId"`
	ID            string                `json:"id"`
	Position      *Position             `json:"position,omitempty"`
	LayerSettings *GraphElementSettings `json:"layerSettings,omitempty"`
	Actor         *Actor                `json:"actor"`
}

// GraphElementEdge projects rpc_graph.layerGraphEdgeRes and graphEdgeRes.
// It excludes user/owner enrichment and arbitrary edge layer settings.
type GraphElementEdge struct {
	LaID       int64             `json:"laId"`
	ID         string            `json:"id"`
	LaIDSource *int64            `json:"laIdSource,omitempty"`
	LaIDTarget *int64            `json:"laIdTarget,omitempty"`
	Edge       *GraphCreatedEdge `json:"edge"`
}
type GraphElements struct {
	Nodes []GraphElementNode `json:"nodes"`
	Edges []GraphElementEdge `json:"edges"`
}
type graphElementsNodeWire struct {
	GraphElementNode
	AccessDenied bool `json:"accessDenied,omitempty"`
}
type graphElementsWire struct {
	Nodes []graphElementsNodeWire `json:"nodes"`
	Edges []GraphElementEdge      `json:"edges"`
}
type graphElementsRequest struct {
	GraphElementsInput
	AccID  string   `json:"accId"`
	Fields []string `json:"fields"`
}

// GetGraphElements returns every exact requested element or a sanitized error.
// This is current readback, never a stable snapshot, write permission or CAS token.
func (c *Client) GetGraphElements(ctx context.Context, p GraphElementsInput) (GraphElements, error) {
	if !c.GraphReadsEnabled() {
		return GraphElements{}, errors.New("graph-read profile is disabled")
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 32<<10 {
		return GraphElements{}, errors.New("invalid graph selection")
	}
	if err = contracts.ValidateGraphElementsInput(raw); err != nil {
		return GraphElements{}, err
	}
	if len(p.Nodes)+len(p.Edges) < 1 || len(p.Nodes)+len(p.Edges) > 100 {
		return GraphElements{}, errors.New("select 1-100 combined placements")
	}
	wire, err := call[graphElementsRequest, graphElementsWire](ctx, c, "graph/layer/elements/list", graphElementsRequest{GraphElementsInput: p, AccID: c.config.Workspace, Fields: actorFields})
	if err != nil {
		return GraphElements{}, err
	}
	unavailable := errors.New("exact graph selection unavailable or inconsistent; no partial metadata returned")
	if wire.Nodes == nil || wire.Edges == nil || len(wire.Nodes) != len(p.Nodes) || len(wire.Edges) != len(p.Edges) {
		return GraphElements{}, unavailable
	}
	nodes := map[int64]bool{}
	edges := map[int64]bool{}
	actors := map[int64]string{}
	for _, id := range p.Nodes {
		nodes[id] = true
	}
	for _, id := range p.Edges {
		edges[id] = true
	}
	result := GraphElements{Nodes: make([]GraphElementNode, 0, len(p.Nodes)), Edges: wire.Edges}
	for _, n := range wire.Nodes {
		if !nodes[n.LaID] || n.AccessDenied || strings.TrimSpace(n.ID) == "" || n.Actor == nil || n.Actor.ID == nil || *n.Actor.ID != n.ID {
			return GraphElements{}, unavailable
		}
		if n.Position != nil && (n.Position.X == nil || n.Position.Y == nil) {
			return GraphElements{}, unavailable
		}
		delete(nodes, n.LaID)
		actors[n.LaID] = n.ID
		result.Nodes = append(result.Nodes, n.GraphElementNode)
	}
	for _, e := range wire.Edges {
		if !edges[e.LaID] || strings.TrimSpace(e.ID) == "" || e.Edge == nil || e.Edge.ID != e.ID || strings.TrimSpace(e.Edge.Source) == "" || strings.TrimSpace(e.Edge.Target) == "" || e.Edge.EdgeTypeID < 1 {
			return GraphElements{}, unavailable
		}
		delete(edges, e.LaID)
		// Unplaced/legacy endpoints can be absent. When present, never manufacture zero IDs.
		if e.LaIDSource != nil && *e.LaIDSource < 1 || e.LaIDTarget != nil && *e.LaIDTarget < 1 {
			return GraphElements{}, unavailable
		}
		if e.LaIDSource != nil {
			if actor, ok := actors[*e.LaIDSource]; ok && actor != e.Edge.Source {
				return GraphElements{}, unavailable
			}
		}
		if e.LaIDTarget != nil {
			if actor, ok := actors[*e.LaIDTarget]; ok && actor != e.Edge.Target {
				return GraphElements{}, unavailable
			}
		}
	}
	return result, nil
}
