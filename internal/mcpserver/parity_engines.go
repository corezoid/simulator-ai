package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai/internal/graphplan"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Legacy tools that are several calls of public methods: deleteEdgesByNodes, getAllLayerPlacements,
// compactGraphLayout and pruneLongEdges. massLink is the console's atomic edges_create.

type nodeLink struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	EdgeTypeID int64  `json:"edgeTypeId"`
	Bidirected bool   `json:"bidirected,omitempty"`
}
type nodeLinksInput struct {
	Links []nodeLink `json:"links"`
}
type layerInput struct {
	LayerID string `json:"layerId"`
}
type compactInput struct {
	LayerID         string  `json:"layerId"`
	BucketThreshold int     `json:"bucketThreshold,omitempty"`
	ClustersPerRow  int     `json:"clustersPerRow,omitempty"`
	NodesPerRow     int     `json:"nodesPerRow,omitempty"`
	NodeDX          float64 `json:"nodeDX,omitempty"`
	NodeDY          float64 `json:"nodeDY,omitempty"`
}
type pruneInput struct {
	LayerID             string  `json:"layerId"`
	MaxDistance         float64 `json:"maxDistance,omitempty"`
	BucketThreshold     int     `json:"bucketThreshold,omitempty"`
	PreserveParentEdges *bool   `json:"preserveParentEdges,omitempty"`
	DryRun              bool    `json:"dryRun,omitempty"`
}

type batchResults struct {
	Items []publicapi.BatchResult `json:"items"`
	Ok    int                     `json:"ok"`
	Fail  int                     `json:"failed"`
}
type allPlacements struct {
	List  []publicapi.LayerPlacement `json:"list"`
	Count int                        `json:"count"`
}

const (
	layerArg = `"layerId":{"type":"string","minLength":1,"maxLength":255}`
	// batchOutput is written by hand: reflection would type each open result as a byte array.
	batchOutput = `{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"index":{"type":"integer"},"result":{"type":"object"},"error":{"type":"string"}},"required":["index"]}},"ok":{"type":"integer"},"failed":{"type":"integer"}},"required":["items","ok","failed"]}`
)

func batchTool[P any](s *server.MCPServer, name, description, schema string, run func(context.Context, P) (batchResults, error)) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(batchOutput)), name, description, schema, true, run)
}

func addGraphEngines(s *server.MCPServer, c *publicapi.Client) {
	if c.ParityAllowed("edge/by-node/delete") {
		batchTool(s, "edges_by_nodes_delete", "Delete edges by their ends (legacy deleteEdgesByNodes): each link {source, target, edgeTypeId, bidirected?} removes the edge of that type between the two actors (both directions with bidirected) and its placements. Up to 100 links, not atomic: items reports each one. Destructive: confirm with the user first.",
			`{"type":"object","properties":{"links":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"source":{"type":"string","minLength":1,"maxLength":255},"target":{"type":"string","minLength":1,"maxLength":255},"edgeTypeId":{"type":"integer","minimum":1},"bidirected":{"type":"boolean"}},"required":["source","target","edgeTypeId"],"additionalProperties":false}}},"required":["links"],"additionalProperties":false}`,
			func(ctx context.Context, in nodeLinksInput) (batchResults, error) {
				items := make([]map[string]json.RawMessage, len(in.Links))
				for i, l := range in.Links {
					items[i] = rawObject(l)
				}
				return batch(c.PassEach(ctx, "edge/by-node/delete", items))
			})
	}
	if c.ParityAllowed("graph/layer/actors/list") {
		consoleTool(s, "layer_placements_all", "Read every node placement of a layer in one call (legacy getAllLayerPlacements): laId, actorId, title, formId, position; up to 20000 placements. For one page with full actors use layer_placements_list.",
			`{"type":"object","properties":{`+layerArg+`},"required":["layerId"],"additionalProperties":false}`, false,
			func(ctx context.Context, in layerInput) (allPlacements, error) {
				list, err := c.AllNodePlacements(ctx, in.LayerID)
				return allPlacements{List: list, Count: len(list)}, err
			})
	}
	if c.ParityAllowed("graph/layer/actors/positions/update") {
		consoleTool(s, "layer_compact", "Auto-layout a layer into a tight domain-clustered grid (legacy compactGraphLayout): actors with at least bucketThreshold (default 3) incoming edges become cluster headers, their linked actors a grid below them (nodesPerRow, nodeDX/nodeDY), clusters a super-grid (clustersPerRow), the rest a Misc zone. Moves placements with graph/layer/actors/positions/update; returns counters. Changes the layout: confirm with the user first.",
			`{"type":"object","properties":{`+layerArg+`,"bucketThreshold":{"type":"integer","minimum":1},"clustersPerRow":{"type":"integer","minimum":1,"maximum":50},"nodesPerRow":{"type":"integer","minimum":1,"maximum":50},"nodeDX":{"type":"number","minimum":50,"maximum":2000},"nodeDY":{"type":"number","minimum":50,"maximum":2000}},"required":["layerId"],"additionalProperties":false}`,
			true, func(ctx context.Context, in compactInput) (graphplan.CompactStats, error) { return compact(ctx, c, in) })
	}
	if c.ParityAllowed("edge/delete") {
		consoleTool(s, "layer_prune_long_edges", "Delete edges of a layer whose ends are further apart than maxDistance graph units, Manhattan (default 600; legacy pruneLongEdges). preserveParentEdges (default true) keeps edges touching a hierarchy bucket (an actor with at least bucketThreshold, default 3, incoming edges). dryRun only counts. Deletes the edges themselves (edge/delete), not just placements. Destructive: run dryRun first and confirm with the user.",
			`{"type":"object","properties":{`+layerArg+`,"maxDistance":{"type":"number","exclusiveMinimum":0},"bucketThreshold":{"type":"integer","minimum":1},"preserveParentEdges":{"type":"boolean"},"dryRun":{"type":"boolean"}},"required":["layerId"],"additionalProperties":false}`,
			true, func(ctx context.Context, in pruneInput) (graphplan.PruneStats, error) { return prune(ctx, c, in) })
	}
}

func rawObject(v any) map[string]json.RawMessage {
	b, _ := json.Marshal(v)
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(b, &out)
	return out
}

func batch(items []publicapi.BatchResult, err error) (batchResults, error) {
	if err != nil {
		return batchResults{}, err
	}
	out := batchResults{Items: items}
	for _, item := range items {
		if item.Error != "" {
			out.Fail++
		} else {
			out.Ok++
		}
	}
	return out, nil
}

// layerGraph reads a whole layer as the plain nodes and links the layout algorithms take.
func layerGraph(ctx context.Context, c *publicapi.Client, layerID string) ([]graphplan.LayerNode, []graphplan.LayerLink, error) {
	placements, err := c.AllNodePlacements(ctx, layerID)
	if err != nil {
		return nil, nil, err
	}
	edges, err := c.AllEdgePlacements(ctx, layerID)
	if err != nil {
		return nil, nil, err
	}
	nodes := make([]graphplan.LayerNode, 0, len(placements))
	for _, p := range placements {
		n := graphplan.LayerNode{LaID: p.LaID, ActorID: p.ActorID}
		if p.Title != nil {
			n.Title = *p.Title
		}
		if p.Position != nil && p.Position.X != nil && p.Position.Y != nil {
			n.X, n.Y = *p.Position.X, *p.Position.Y
		}
		nodes = append(nodes, n)
	}
	links := make([]graphplan.LayerLink, 0, len(edges))
	for _, e := range edges {
		l := graphplan.LayerLink{EdgeID: e.EdgeID, Source: e.Source, Target: e.Target}
		if e.LaIDSource != nil {
			l.LaIDSource = *e.LaIDSource
		}
		if e.LaIDTarget != nil {
			l.LaIDTarget = *e.LaIDTarget
		}
		links = append(links, l)
	}
	return nodes, links, nil
}

func compact(ctx context.Context, c *publicapi.Client, in compactInput) (graphplan.CompactStats, error) {
	nodes, links, err := layerGraph(ctx, c, in.LayerID)
	if err != nil {
		return graphplan.CompactStats{}, err
	}
	moves, stats := graphplan.CompactLayout(nodes, links, graphplan.CompactOptions{BucketThreshold: in.BucketThreshold, ClustersPerRow: in.ClustersPerRow, NodesPerRow: in.NodesPerRow, NodeDX: in.NodeDX, NodeDY: in.NodeDY})
	if len(moves) > 500 {
		// One call moves all of them, so vacated cells can be reused; sim-api takes at most 500.
		return graphplan.CompactStats{}, fmt.Errorf("layout would move %d placements; at most 500 per call", len(moves))
	}
	if len(moves) == 0 {
		return stats, nil
	}
	type item struct {
		LaActorID int64          `json:"laActorId"`
		Position  map[string]any `json:"position"`
	}
	items := make([]item, len(moves))
	for i, m := range moves {
		items[i] = item{LaActorID: m.LaID, Position: map[string]any{"x": m.X, "y": m.Y}}
	}
	raw, _ := json.Marshal(items)
	layer, _ := json.Marshal(in.LayerID)
	if _, err = c.Pass(ctx, "graph/layer/actors/positions/update", map[string]json.RawMessage{"actorId": layer, "items": raw}); err != nil {
		return graphplan.CompactStats{}, err
	}
	return stats, nil
}

func prune(ctx context.Context, c *publicapi.Client, in pruneInput) (graphplan.PruneStats, error) {
	nodes, links, err := layerGraph(ctx, c, in.LayerID)
	if err != nil {
		return graphplan.PruneStats{}, err
	}
	keep := in.PreserveParentEdges == nil || *in.PreserveParentEdges
	ids, stats := graphplan.PruneLongEdges(nodes, links, graphplan.PruneOptions{MaxDistance: in.MaxDistance, BucketThreshold: in.BucketThreshold, KeepParentEdges: keep})
	stats.DryRun = in.DryRun
	if in.DryRun || len(ids) == 0 {
		return stats, nil
	}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		items := make([]map[string]json.RawMessage, 0, end-start)
		for _, id := range ids[start:end] {
			raw, _ := json.Marshal(id)
			items = append(items, map[string]json.RawMessage{"edgeId": raw})
		}
		results, err := c.PassEach(ctx, "edge/delete", items)
		if err != nil {
			return graphplan.PruneStats{}, err
		}
		for _, r := range results {
			if r.Error != "" {
				stats.Deleted--
				stats.Errors = append(stats.Errors, ids[start+r.Index]+": "+r.Error)
			}
		}
	}
	return stats, nil
}
