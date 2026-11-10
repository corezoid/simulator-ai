package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/corezoid/simulator-ai/internal/graphplan"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// buildInput is graph_build's input, shaped to cost the model the fewest output tokens: keys instead
// of ids, call-level defaults, and nodes and edges as short tuples when they carry nothing else.
type buildInput struct {
	OperationID string `json:"operationId"`
	LayerID     string `json:"layerId"`
	NewLayer    string `json:"newLayer"`
	// FormID, Kind and EdgeType are the defaults of nodes and edges that name none.
	FormID   int64             `json:"formId"`
	Kind     string            `json:"kind"`
	EdgeType string            `json:"edgeType"`
	Layout   string            `json:"layout"`
	Origin   string            `json:"origin"`
	RowCells int               `json:"rowCells"`
	Nodes    []buildNodeIn     `json:"nodes"`
	Edges    []buildEdgeIn     `json:"edges"`
	Existing map[string]string `json:"existing"`
	Use      []string          `json:"use"`
	IDs      []string          `json:"ids"`
}

// buildNodeIn is [key, title], [key, title, cell] or the object.
type buildNodeIn struct {
	Key    string          `json:"key"`
	Title  string          `json:"title"`
	FormID int64           `json:"formId"`
	Kind   string          `json:"kind"`
	Desc   string          `json:"desc"`
	Data   json.RawMessage `json:"data"`
	Color  string          `json:"color"`
	Hole   bool            `json:"hole"`
	Cell   string          `json:"cell"`
}

// buildEdgeIn is [from, to], [from, to, name] or the object.
type buildEdgeIn struct {
	From string `json:"from"`
	To   string `json:"to"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// tuple decodes a JSON array of strings into the fields, in order; false for an object.
func tuple(raw []byte, fields ...*string) (bool, error) {
	if len(raw) == 0 || raw[0] != '[' {
		return false, nil
	}
	var parts []string
	if err := json.Unmarshal(raw, &parts); err != nil {
		return true, err
	}
	for i, p := range parts {
		if i < len(fields) {
			*fields[i] = p
		}
	}
	return true, nil
}

func (n *buildNodeIn) UnmarshalJSON(raw []byte) error {
	if short, err := tuple(raw, &n.Key, &n.Title, &n.Cell); short {
		return err
	}
	type object buildNodeIn
	return json.Unmarshal(raw, (*object)(n))
}

func (e *buildEdgeIn) UnmarshalJSON(raw []byte) error {
	if short, err := tuple(raw, &e.From, &e.To, &e.Name); short {
		return err
	}
	type object buildEdgeIn
	return json.Unmarshal(raw, (*object)(e))
}

const (
	shortTuple  = `{"type":"array","minItems":2,"maxItems":3,"items":{"type":"string"}}`
	buildSchema = `{"type":"object","properties":{"operationId":{"type":"string","pattern":"^[A-Za-z0-9_-]{1,64}$"},"layerId":{"type":"string"},"newLayer":{"type":"string"},"formId":{"type":"integer"},"kind":{"type":"string"},"edgeType":{"type":"string"},"layout":{"enum":["auto","flowchart","layered","grid","given"]},"origin":{"type":"string"},"rowCells":{"type":"integer","minimum":4,"maximum":400},` +
		`"nodes":{"type":"array","maxItems":2000,"items":{"anyOf":[` + shortTuple + `,{"type":"object","properties":{"key":{"type":"string"},"title":{"type":"string"},"formId":{"type":"integer"},"kind":{"type":"string"},"desc":{"type":"string"},"data":{"type":"object"},"color":{"type":"string"},"hole":{"type":"boolean"},"cell":{"type":"string"}},"required":["key","title"],"additionalProperties":false}]}},` +
		`"edges":{"type":"array","maxItems":4000,"items":{"anyOf":[` + shortTuple + `,{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"},"name":{"type":"string"},"type":{"type":"string"}},"required":["from","to"],"additionalProperties":false}]}},` +
		`"existing":{"type":"object","additionalProperties":{"type":"string"}},"use":{"type":"array","maxItems":20,"items":{"type":"string"}},"ids":{"type":"array","maxItems":200,"items":{"type":"string"}}},"required":["operationId"],"additionalProperties":false}`
	// buildOutput names only what every answer has: the rest is described once, in the description.
	buildOutput        = `{"type":"object","properties":{"done":{"type":"boolean"},"layerId":{"type":"string"}},"required":["done","layerId"]}`
	buildDeleteSchema  = `{"type":"object","properties":{"layerId":{"type":"string"},"operationId":{"type":"string"},"dryRun":{"type":"boolean"}},"required":["layerId","operationId"],"additionalProperties":false}`
	buildDescription   = "Build a graph on a layer in ONE call: actors, placements, edges and their placements, by your own keys (no ids to pass around). nodes: [key, title] or {key, title, formId, kind, desc, data, color, hole, cell}; edges: [from, to, name?] or {from, to, name, type}. Top-level formId, kind (a flowchart blockId) and edgeType are the defaults. layerId, or newLayer (a title). layout: auto (children under their parent when the edges have a direction, else a grid), layered, grid, or given ([key, title, cell]); origin: the top-left cell, default the next free place: omit it, sections fill rows 80 cells wide; rowCells: row width, default 60. Edge ends may also be keys of operations listed in use, or existing {key: actorId} on the layer. operationId (yours): repeating the call builds only what is missing, never twice. ids: keys whose actorId to return. Returns done, layerId, nodes and edges {total, done, existing}, area; not done: chunks, error, resume. At most 2000 nodes and 4000 edges."
	buildDeleteDetails = "Delete the actors a graph_build operationId created on a layer, with their placements and edges; a layer it created stays. Destructive: run dryRun (counts only) first and confirm with the user."
)

// addGraphBuild registers graph_build and the removal of what one operation built.
func addGraphBuild(s *server.MCPServer, c *publicapi.Client) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(buildOutput)), "graph_build", buildDescription, buildSchema, true,
		func(ctx context.Context, in buildInput) (publicapi.GraphBuildResult, error) {
			return graphBuild(ctx, c, in)
		})
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(`{"type":"object","properties":{"found":{"type":"integer"},"deleted":{"type":"integer"}}}`)), "graph_build_delete", buildDeleteDetails, buildDeleteSchema, true, c.DeleteGraphBuild)
}

// buildKinds reads the flowchart block kinds the nodes name.
func buildKinds(ctx context.Context, c *publicapi.Client, in buildInput) (map[string]graphplan.Block, error) {
	var kinds []string
	seen := map[string]bool{}
	for _, n := range in.Nodes {
		kind := n.Kind
		if kind == "" && n.FormID == 0 {
			kind = in.Kind
		}
		if kind != "" && !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	defs := map[string]graphplan.Block{}
	if len(kinds) == 0 {
		return defs, nil
	}
	catalog, err := c.ListFlowchartBlocks(ctx, publicapi.FlowchartBlocksInput{BlockIDs: kinds})
	if err != nil {
		return nil, err
	}
	for _, b := range graphplan.ConsoleBlocks(catalog, true).List {
		defs[b.BlockID] = b
	}
	for _, kind := range kinds {
		if _, ok := defs[kind]; !ok {
			return nil, fmt.Errorf("flowchart block %q is not in the workspace catalog (flowchart_blocks_list)", kind)
		}
	}
	return defs, nil
}

// mergeData lays a node's own field values over its block kind's.
func mergeData(base, own json.RawMessage) (json.RawMessage, error) {
	if len(own) == 0 {
		return base, nil
	}
	merged := map[string]json.RawMessage{}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &merged); err != nil {
			return nil, errors.New("block data is not an object")
		}
	}
	extra := map[string]json.RawMessage{}
	if err := json.Unmarshal(own, &extra); err != nil {
		return nil, errors.New("data must be an object of form field values")
	}
	for k, v := range extra {
		merged[k] = v
	}
	return json.Marshal(merged)
}

// buildNodes turns the input nodes into actors with their boxes; positions come from buildLayout.
func buildNodes(in buildInput, defs map[string]graphplan.Block) ([]publicapi.BuildNode, []graphplan.LayoutNode, error) {
	nodes := make([]publicapi.BuildNode, len(in.Nodes))
	boxes := make([]graphplan.LayoutNode, len(in.Nodes))
	for i, n := range in.Nodes {
		node := publicapi.BuildNode{Key: n.Key, Title: n.Title, Description: n.Desc, Color: n.Color, Data: n.Data, Hole: n.Hole, FormID: n.FormID}
		box := graphplan.LayoutNode{Key: n.Key, Width: graphplan.PlainSize, Height: graphplan.PlainSize}
		kind := n.Kind
		if kind == "" && n.FormID == 0 {
			kind = in.Kind
		}
		if kind != "" {
			def := defs[kind]
			data, err := mergeData(def.Data, n.Data)
			if err != nil {
				return nil, nil, fmt.Errorf("node %q: %w", n.Key, err)
			}
			settings := *def.LayerSettings
			node.FormID, node.Data, node.LayerSettings = def.FormID, data, &settings
			if node.Color == "" {
				node.Color = def.Color
			}
			if node.Color == "" {
				node.Color = publicapi.RandomActorColor()
			}
			picture, err := graphplan.AreaPicture(settings, node.Color)
			if err != nil {
				return nil, nil, err
			}
			node.AreaPicture = &picture
			box.Width, box.Height = settings.Width, settings.Height
		} else if node.FormID == 0 {
			node.FormID = in.FormID
		}
		nodes[i], boxes[i] = node, box
	}
	return nodes, boxes, nil
}

// buildLayout gives every node its centre. A layout starts at the origin asked for, else where an
// earlier call of the operation put it (so a repeated call continues the same picture), else on the
// next free place of the layer (FreeOrigin).
func buildLayout(in buildInput, b *publicapi.GraphBuild, nodes []publicapi.BuildNode, boxes []graphplan.LayoutNode) error {
	if in.Layout == "given" {
		for i, n := range in.Nodes {
			at, err := publicapi.CellCenter(n.Cell)
			if err != nil {
				return fmt.Errorf("node %q: layout given needs every node's cell: %w", n.Key, err)
			}
			nodes[i].Position = graphplan.GridCenter(at, boxes[i].Width, boxes[i].Height)
		}
		return nil
	}
	index := map[string]bool{}
	plain := true
	for i, n := range in.Nodes {
		if n.Cell != "" {
			return fmt.Errorf("node %q: a cell is taken only with layout given", n.Key)
		}
		index[n.Key] = true
		plain = plain && nodes[i].LayerSettings == nil
	}
	var edges []graphplan.Edge
	seen := map[[2]string]bool{}
	for _, e := range in.Edges {
		pair := [2]string{e.From, e.To}
		if index[e.From] && index[e.To] && e.From != e.To && !seen[pair] {
			seen[pair] = true
			edges = append(edges, graphplan.Edge{Key: e.From + "->" + e.To, Source: e.From, Target: e.To})
		}
	}
	// Flowchart blocks keep whole ranks, as flowchart_layout draws them, unless layered is asked for.
	opt := graphplan.LayoutOptions{RowWidth: float64(in.RowCells) * graphplan.PlainSize, RankRows: in.Layout == "flowchart" || (!plain && in.Layout != "layered")}
	if plain {
		opt.Gap = graphplan.PlainGap
	}
	var points []publicapi.GraphPoint
	var box graphplan.LayoutBox
	switch {
	case in.Layout == "grid", (in.Layout == "" || in.Layout == "auto") && !graphplan.Layerable(boxes, edges):
		points, box = graphplan.Grid(boxes, opt)
	default:
		points, _, box = graphplan.Layered(boxes, edges, opt)
	}
	corner := b.FreeOrigin(box.W)
	if in.Origin != "" {
		at, err := publicapi.CellCenter(in.Origin)
		if err != nil {
			return fmt.Errorf("origin: %w", err)
		}
		corner = publicapi.GraphPoint{X: at.X - graphplan.PlainSize/2, Y: at.Y - graphplan.PlainSize/2}
	}
	dx, dy := corner.X-box.X, corner.Y-box.Y
	for i, n := range in.Nodes {
		if at, landed := b.Landed(n.Key); landed {
			dx, dy = at.X-points[i].X, at.Y-points[i].Y
			break
		}
	}
	for i := range nodes {
		nodes[i].Position = publicapi.GraphPoint{X: points[i].X + dx, Y: points[i].Y + dy}
	}
	return nil
}

// checkBuildCells checks the cells the input names before anything is read or written: buildLayout
// would refuse the same, but only after a new layer exists.
func checkBuildCells(in buildInput) error {
	if in.Origin != "" {
		if _, err := publicapi.CellCenter(in.Origin); err != nil {
			return fmt.Errorf("origin: %w", err)
		}
	}
	for _, n := range in.Nodes {
		if in.Layout != "given" && n.Cell != "" {
			return fmt.Errorf("node %q: a cell is taken only with layout given", n.Key)
		}
		if in.Layout == "given" {
			if _, err := publicapi.CellCenter(n.Cell); err != nil {
				return fmt.Errorf("node %q: layout given needs every node's cell: %w", n.Key, err)
			}
		}
	}
	return nil
}

func graphBuild(ctx context.Context, c *publicapi.Client, in buildInput) (publicapi.GraphBuildResult, error) {
	if len(in.Nodes) > publicapi.MaxBuildNodes || len(in.Edges) > publicapi.MaxBuildEdges {
		return publicapi.GraphBuildResult{}, fmt.Errorf("at most %d nodes and %d edges per call: split the graph into sections", publicapi.MaxBuildNodes, publicapi.MaxBuildEdges)
	}
	for _, n := range in.Nodes {
		if n.Kind == "" && n.FormID == 0 && in.Kind == "" && in.FormID == 0 {
			return publicapi.GraphBuildResult{}, fmt.Errorf("node %q: formId or kind required, its own or the call's default", n.Key)
		}
	}
	defs, err := buildKinds(ctx, c, in)
	if err != nil {
		return publicapi.GraphBuildResult{}, err
	}
	nodes, boxes, err := buildNodes(in, defs)
	if err != nil {
		return publicapi.GraphBuildResult{}, err
	}
	run := publicapi.GraphBuildInput{Nodes: nodes, Existing: in.Existing, Use: in.Use, IDs: in.IDs, Edges: make([]publicapi.BuildEdge, len(in.Edges))}
	for i, e := range in.Edges {
		edgeType := e.Type
		if edgeType == "" {
			edgeType = in.EdgeType
		}
		run.Edges[i] = publicapi.BuildEdge{From: e.From, To: e.To, Name: e.Name, EdgeType: edgeType}
	}
	if err = run.Validate(); err != nil { // before a new layer is created for a call that cannot run
		return publicapi.GraphBuildResult{}, err
	}
	if err = checkBuildCells(in); err != nil { // likewise: a bad cell or origin must not leave an empty new layer
		return publicapi.GraphBuildResult{}, err
	}
	b, err := c.OpenGraphBuild(ctx, publicapi.GraphBuildTarget{LayerID: in.LayerID, NewLayer: in.NewLayer, OperationID: in.OperationID})
	if err != nil {
		return publicapi.GraphBuildResult{}, err
	}
	if err = buildLayout(in, b, nodes, boxes); err != nil {
		return publicapi.GraphBuildResult{}, err
	}
	return b.Run(ctx, run)
}
