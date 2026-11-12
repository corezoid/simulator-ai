package publicapi

import (
	"context"
	"math"
	"sort"
	"time"
)

// Large layers made the graph reads the biggest tool outputs of a Console dialog: every one rides in
// the model context of each later step. The reads answer compact by default — what a model acts
// on, a page of it and a summary of the rest — and as before with full.

// Default page sizes per kind of layer_get: compact rows are a third of a full row, and a model
// that needs one actor of a large layer searches for it rather than paging.
const (
	layerGetCompactLimit = 100
	layerGetFullLimit    = 200
)

// LayerTotals counts a layer's node and edge placements when layer_get answers only a page.
type LayerTotals struct {
	Nodes int64 `json:"nodes"`
	Edges int64 `json:"edges"`
}

// compactLayerView keeps of each row what the next tool takes: nodes by laId, actorId and cell (the
// address tools and users use; position comes with full), edges by laId and their node laIds, a
// label only when there is one. Nodes and edges page apart, so an edge whose node is not on this
// page keeps its source/target actor ids; the others' are their nodes' actorIds.
func compactLayerView(v *LayerView) {
	onPage := map[int64]bool{}
	for i := range v.Nodes {
		v.Nodes[i].Position = nil
		onPage[v.Nodes[i].LaID] = true
	}
	for i := range v.Edges {
		e := &v.Edges[i]
		if e.LaIDSource != nil && e.LaIDTarget != nil && onPage[*e.LaIDSource] && onPage[*e.LaIDTarget] {
			e.Source, e.Target = "", ""
		}
		if e.Name != nil && *e.Name == "" {
			e.Name = nil
		}
	}
}

// totalsTimeout bounds the layer counters a page waits for: they are a courtesy, not the answer.
const totalsTimeout = 2 * time.Second

// layerTotals reads the layer's counters for a page that did not hold everything — a separate
// request from the page, as list and count are. It is a courtesy, not the answer: late or failed,
// the page goes without a total.
func (c *Client) layerTotals(ctx context.Context, layerID string) *LayerTotals {
	ctx, cancel := context.WithTimeout(ctx, totalsTimeout)
	defer cancel()
	w, err := call[layerStatsRequest, layerStatsWire](ctx, c, "graph/layer/stats/get", layerStatsRequest{AccID: c.workspace(), ActorID: layerID})
	if err != nil || w.Nodes == nil || w.Edges == nil {
		return nil
	}
	return &LayerTotals{Nodes: *w.Nodes, Edges: *w.Edges}
}

// PlacementsSummary sums up every node placement of a layer: how many, of which templates, and the
// box they cover, so a model sees a 10 000-node layer in a few lines.
type PlacementsSummary struct {
	Forms []FormCount `json:"forms"`
	Box   *GraphBox   `json:"box,omitempty"`
}

// FormCount is how many placements show actors of one template (formId 0: not visible to you).
type FormCount struct {
	FormID int64 `json:"formId"`
	Count  int   `json:"count"`
}

// GraphBox is a rectangle in graph units, x1,y1 top left.
type GraphBox struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

// maxSummaryForms bounds the per-template counts of a summary: the rest are counted under formId -1.
const maxSummaryForms = 30

// SummarizePlacements counts placements by template (most first) and the box their centres span.
func SummarizePlacements(list []LayerPlacement) PlacementsSummary {
	counts := map[int64]int{}
	var box *GraphBox
	for _, p := range list {
		var form int64
		if p.FormID != nil {
			form = *p.FormID
		}
		counts[form]++
		if p.Position == nil || p.Position.X == nil || p.Position.Y == nil {
			continue
		}
		x, y := *p.Position.X, *p.Position.Y
		if box == nil {
			box = &GraphBox{X1: x, Y1: y, X2: x, Y2: y}
		}
		box.X1, box.Y1, box.X2, box.Y2 = math.Min(box.X1, x), math.Min(box.Y1, y), math.Max(box.X2, x), math.Max(box.Y2, y)
	}
	out := PlacementsSummary{Box: box, Forms: make([]FormCount, 0, len(counts))}
	for f, n := range counts {
		out.Forms = append(out.Forms, FormCount{FormID: f, Count: n})
	}
	sort.Slice(out.Forms, func(i, j int) bool {
		a, b := out.Forms[i], out.Forms[j]
		return a.Count > b.Count || a.Count == b.Count && a.FormID < b.FormID
	})
	if len(out.Forms) > maxSummaryForms {
		rest := FormCount{FormID: -1}
		for _, f := range out.Forms[maxSummaryForms:] {
			rest.Count += f.Count
		}
		out.Forms = append(out.Forms[:maxSummaryForms], rest)
	}
	return out
}

// CompactPlacement is a node placement by its cell instead of its exact position.
func CompactPlacement(p LayerPlacement) LayerPlacement {
	if p.Position != nil && p.Position.X != nil && p.Position.Y != nil {
		p.Cell = CellOf(GraphPoint{X: *p.Position.X, Y: *p.Position.Y})
	}
	p.Position = nil
	return p
}
