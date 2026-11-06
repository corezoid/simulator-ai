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
	List      []publicapi.LayerPlacement   `json:"list"`
	Count     int                          `json:"count"`
	Summary   *publicapi.PlacementsSummary `json:"summary,omitempty"`
	Truncated bool                         `json:"truncated,omitempty"`
}

type allPlacementsInput struct {
	LayerID string `json:"layerId"`
	Limit   int    `json:"limit"`
	Full    bool   `json:"full"`
}

// allPlacementsOutput is written by hand to keep the graph group within its bound.
const allPlacementsOutput = `{"type":"object","properties":{"count":{"type":"integer"},"list":{"type":"array","items":{"type":"object","properties":{"laId":{"type":"integer"},"actorId":{"type":"string"},"title":{"type":"string"},"formId":{"type":"integer"},"cell":{"type":"string"},"position":{"type":"object"}}}},"summary":{"type":"object","properties":{"forms":{"type":"array"},"box":{"type":"object"}}},"truncated":{"type":"boolean"}}}`

// allPlacementsLimit is the compact layer_placements_all page: past it the summary speaks.
const allPlacementsLimit = 150

// compactAll answers every placement with full; else the summary and the first limit rows by cell.
func compactAll(list []publicapi.LayerPlacement, in allPlacementsInput) allPlacements {
	out := allPlacements{List: list, Count: len(list)}
	if in.Full {
		return out
	}
	limit := allPlacementsLimit
	if in.Limit > 0 {
		limit = in.Limit
	}
	summary := publicapi.SummarizePlacements(list)
	out.Summary = &summary
	if len(list) > limit {
		out.List, out.Truncated = list[:limit], true
	}
	rows := make([]publicapi.LayerPlacement, len(out.List))
	for i, p := range out.List {
		rows[i] = publicapi.CompactPlacement(p)
	}
	out.List = rows
	return out
}

const (
	layerArg = `"layerId":{"type":"string","minLength":1,"maxLength":255}`
	// batchOutput is written by hand: reflection would type each open result as a byte array.
	batchOutput = `{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"index":{"type":"integer"},"result":{"type":"object"},"error":{"type":"string"}},"required":["index"]}},"ok":{"type":"integer"},"failed":{"type":"integer"}},"required":["items","ok","failed"]}`
)

func batchTool[P any](s *server.MCPServer, name, description, schema string, run func(context.Context, P) (batchResults, error)) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(batchOutput)), name, description, schema, true, run)
}

const searchOutput = `{"type":"object","properties":{"users":{"type":"object"},"actors":{"type":"object"},"errors":{"type":"object","additionalProperties":{"type":"string"}}}}`

// addActorEngines registers search_all (legacy searchAll) where the actor reads are.
func addActorEngines(s *server.MCPServer, c *publicapi.Client) {
	if !c.ParityAllowed("actor/search/list") {
		return
	}
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(searchOutput)), "search_all", "Search the workspace for users and/or actors in one call (legacy searchAll): filters users, actors, fullTextActor (actors in text mode). Each part is its own page (limit/offset), with no shared ranking: users from user/list, actors from actor/search/list (fields projects them, default id,title,formId; searchType text (default; pages with offset) or semantic (ranked, ignores offset); formId narrows to one template). A failed part is in errors and the other still answers.",
		`{"type":"object","properties":{"query":{"type":"string","minLength":2,"maxLength":1000},"filters":{"type":"array","minItems":1,"maxItems":3,"uniqueItems":true,"items":{"type":"string","enum":["users","actors","fullTextActor"]}},"searchType":{"type":"string","enum":["text","semantic"]},"formId":{"type":"integer","minimum":1},"fields":{"type":"array","items":{"type":"string","minLength":1}},"limit":{"type":"integer","minimum":1,"maximum":100},"offset":{"type":"integer","minimum":0}},"required":["query","filters"],"additionalProperties":false}`, false,
		func(ctx context.Context, in publicapi.SearchAllInput) (publicapi.SearchAll, error) {
			if in.Fields == nil {
				in.Fields = []string{"id", "title", "formId"}
			}
			return c.SearchAll(ctx, in)
		})
}

// addFormEngines registers form_linked_get (legacy getLinkedForms) where the form reads are.
func addFormEngines(s *server.MCPServer, c *publicapi.Client) {
	if c.ParityAllowed("form/tree/get") {
		addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(linkedFormsOutput)), "form_linked_get", "List one account template's linked templates (legacy getLinkedForms): the template itself first, then with direction children every template nested under it (from its tree, with parentId; truncated when the tree was cut), with parents its ancestor chain, nearest first. A template you cannot view is a stub {id, parentId, accessDenied}.",
			`{"type":"object","properties":{"formId":{"type":"integer","minimum":1},"direction":{"type":"string","enum":["children","parents"]}},"required":["formId","direction"],"additionalProperties":false}`, false,
			func(ctx context.Context, in linkedFormsInput) (linkedForms, error) { return linkedFormsOf(ctx, c, in) })
	}
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
		addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(allPlacementsOutput)), "layer_placements_all", "Read every node placement of a layer (legacy getAllLayerPlacements): count, summary {forms [{formId, count}], box of the centres} and the first limit (default 150) rows {laId, actorId, title, formId, cell}; truncated: more exist (read a region with layer_skeleton_get box). full: every row with position, up to 20000",
			`{"type":"object","properties":{`+layerArg+`,"limit":{"type":"integer","minimum":1,"maximum":20000},"full":{"type":"boolean"}},"required":["layerId"],"additionalProperties":false}`, false,
			func(ctx context.Context, in allPlacementsInput) (allPlacements, error) {
				list, err := c.AllNodePlacements(ctx, in.LayerID)
				return compactAll(list, in), err
			})
	}
	if c.ParityAllowed("graph/layer/actors/positions/update") {
		consoleTool(s, "layer_compact", "Auto-layout a layer into a domain-clustered grid (legacy compactGraphLayout): actors with at least bucketThreshold (default 3) incoming edges head clusters of their linked actors (nodesPerRow, nodeDX/nodeDY), clusters form a super-grid (clustersPerRow), the rest a Misc zone. Moves placements; returns counters. Not for a flowchart from flowchart_layout (blocks would leave the grid and overlap). Changes the layout: confirm with the user first.",
			`{"type":"object","properties":{`+layerArg+`,"bucketThreshold":{"type":"integer","minimum":1},"clustersPerRow":{"type":"integer","minimum":1,"maximum":50},"nodesPerRow":{"type":"integer","minimum":1,"maximum":50},"nodeDX":{"type":"number","minimum":50,"maximum":2000},"nodeDY":{"type":"number","minimum":50,"maximum":2000}},"required":["layerId"],"additionalProperties":false}`,
			true, func(ctx context.Context, in compactInput) (graphplan.CompactStats, error) { return compact(ctx, c, in) })
	}
	if c.ParityAllowed("edge/delete") {
		consoleTool(s, "layer_prune_long_edges", "Delete a layer's edges whose ends are more than maxDistance graph units apart, Manhattan (default 600; legacy pruneLongEdges); preserveParentEdges (default true) keeps edges of an actor with at least bucketThreshold (default 3) incoming edges. Deletes the edges themselves, not just placements. Returns counts, up to 10 examples and failures. Destructive: run dryRun (counts only) first and confirm with the user.",
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
	// Legacy edge placements may lack endpoint laIds: then the ends are the actors' last placements,
	// as legacy measured them.
	last := map[string]int64{}
	for _, n := range nodes {
		last[n.ActorID] = n.LaID
	}
	links := make([]graphplan.LayerLink, 0, len(edges))
	for _, e := range edges {
		l := graphplan.LayerLink{EdgeID: e.EdgeID, Source: e.Source, Target: e.Target, LaIDSource: last[e.Source], LaIDTarget: last[e.Target]}
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
			// Earlier chunks are already deleted: report them, and this chunk as not deleted.
			stats.Deleted -= len(ids) - start
			stats.Errors = append(stats.Errors, fmt.Sprintf("%d edges not deleted: %v", len(ids)-start, err))
			return stats, nil
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

type linkedFormsInput struct {
	FormID    int64  `json:"formId"`
	Direction string `json:"direction"`
}

// linkedForm is one template row of form/tree/get or of form/get parents.
type linkedForm struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parentId,omitempty"`
	Row      json.RawMessage
}

func (f *linkedForm) UnmarshalJSON(b []byte) error {
	var head struct {
		ID       int64  `json:"id"`
		ParentID *int64 `json:"parentId"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return err
	}
	f.ID, f.ParentID, f.Row = head.ID, head.ParentID, append(json.RawMessage(nil), b...)
	return nil
}

// linkedForms starts with the template itself, as legacy getChildrenForms/getParentForms did.
// Truncated passes on form/tree/get's own flag: a very large tree is cut, so children may be missing.
type linkedForms struct {
	List      []json.RawMessage `json:"list"`
	Truncated bool              `json:"truncated,omitempty"`
}

const linkedFormsOutput = `{"type":"object","properties":{"list":{"type":"array","items":{"type":"object"}},"truncated":{"type":"boolean"}},"required":["list"]}`

func linkedFormsOf(ctx context.Context, c *publicapi.Client, in linkedFormsInput) (linkedForms, error) {
	id, _ := json.Marshal(in.FormID)
	out := linkedForms{List: []json.RawMessage{}}
	if in.Direction == "parents" {
		raw, err := c.Pass(ctx, "form/get", map[string]json.RawMessage{"formId": id, "with": json.RawMessage(`["parents"]`)})
		if err != nil {
			return linkedForms{}, err
		}
		var form struct {
			Parents []json.RawMessage `json:"parents"`
		}
		if err = json.Unmarshal(raw, &form); err != nil {
			return linkedForms{}, fmt.Errorf("form/get parents: %w", err)
		}
		var self map[string]json.RawMessage
		if err = json.Unmarshal(raw, &self); err != nil {
			return linkedForms{}, fmt.Errorf("form/get: %w", err)
		}
		delete(self, "parents")
		row, _ := json.Marshal(self)
		out.List = append(append(out.List, row), form.Parents...)
		return out, nil
	}
	raw, err := c.Pass(ctx, "form/tree/get", map[string]json.RawMessage{"formId": id})
	if err != nil {
		return linkedForms{}, err
	}
	var tree struct {
		List      []linkedForm `json:"list"`
		Truncated bool         `json:"truncated"`
	}
	if err = json.Unmarshal(raw, &tree); err != nil {
		return linkedForms{}, fmt.Errorf("form/tree/get: %w", err)
	}
	out.Truncated = tree.Truncated
	for _, f := range tree.List {
		if f.ID == in.FormID {
			out.List = append(out.List, f.Row)
		}
	}
	// Every descendant of formId, in the order the tree lists them.
	under := map[int64]bool{in.FormID: true}
	for changed := true; changed; {
		changed = false
		for _, f := range tree.List {
			if f.ParentID != nil && under[*f.ParentID] && !under[f.ID] {
				under[f.ID], changed = true, true
				out.List = append(out.List, f.Row)
			}
		}
	}
	return out, nil
}
