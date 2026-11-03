package publicapi

import (
	"context"
	"errors"
	"strings"
)

// LayerStats follows service_graph.LayerStatsValues in public graph/layer/stats/get.
// These are recipient-eligible placement totals, not distinct actor counts.
type LayerStats struct {
	Nodes           int64 `json:"nodes"`
	Edges           int64 `json:"edges"`
	Holes           int64 `json:"holes"`
	HolesClosed     int64 `json:"holesClosed"`
	EdgeHoles       int64 `json:"edgeHoles"`
	EdgeHolesClosed int64 `json:"edgeHolesClosed"`
}

// Pointers distinguish the six required public counters from omitted/null values.
type layerStatsWire struct {
	Nodes           *int64 `json:"nodes"`
	Edges           *int64 `json:"edges"`
	Holes           *int64 `json:"holes"`
	HolesClosed     *int64 `json:"holesClosed"`
	EdgeHoles       *int64 `json:"edgeHoles"`
	EdgeHolesClosed *int64 `json:"edgeHolesClosed"`
}

type layerStatsRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
}

func (c *Client) LayerStatsActorBound() bool {
	return c.config.Profile == "graph-read" && c.config.ActorID != ""
}

func (c *Client) GetLayerStats(ctx context.Context, actorID string) (LayerStats, error) {
	if !c.GraphReadsEnabled() {
		return LayerStats{}, errors.New("graph-read profile is disabled")
	}
	if c.LayerStatsActorBound() {
		if actorID != "" {
			return LayerStats{}, errors.New("actorId is host-bound; omit tool arguments")
		}
		actorID = c.config.ActorID
	}
	if strings.TrimSpace(actorID) == "" || len(actorID) > 255 {
		return LayerStats{}, errors.New("actorId required, maximum 255 bytes")
	}
	w, err := call[layerStatsRequest, layerStatsWire](ctx, c, "graph/layer/stats/get", layerStatsRequest{AccID: c.config.Workspace, ActorID: actorID})
	if err != nil {
		return LayerStats{}, err
	}
	for _, count := range []*int64{w.Nodes, w.Edges, w.Holes, w.HolesClosed, w.EdgeHoles, w.EdgeHolesClosed} {
		if count == nil || *count < 0 {
			return LayerStats{}, errors.New("invalid layer counter result")
		}
	}
	return LayerStats{Nodes: *w.Nodes, Edges: *w.Edges, Holes: *w.Holes, HolesClosed: *w.HolesClosed, EdgeHoles: *w.EdgeHoles, EdgeHolesClosed: *w.EdgeHolesClosed}, nil
}
