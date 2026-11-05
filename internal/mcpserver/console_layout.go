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

type layoutInput struct {
	Nodes []struct {
		Key     string `json:"key"`
		BlockID string `json:"blockId"`
	} `json:"nodes"`
	Edges []struct {
		Source string `json:"source"`
		Target string `json:"target"`
	} `json:"edges"`
}

// layoutNode is everything the agent needs to create and place one planned block.
type layoutNode struct {
	Key           string                        `json:"key"`
	BlockID       string                        `json:"blockId"`
	FormID        int64                         `json:"formId"`
	Color         string                        `json:"color"`
	Data          json.RawMessage               `json:"data"`
	Position      publicapi.GraphPoint          `json:"position"`
	Cell          string                        `json:"cell"`
	LayerSettings publicapi.GraphNativeSettings `json:"layerSettings"`
}

type layout struct {
	Nodes       []layoutNode `json:"nodes"`
	ReturnEdges []string     `json:"returnEdges"`
}

const layoutOutput = `{"type":"object","properties":{"nodes":{"type":"array","items":{"type":"object","properties":{"key":{"type":"string"},"blockId":{"type":"string"},"formId":{"type":"integer"},"color":{"type":"string"},"data":{"type":"object"},"position":` + point + `,"cell":{"type":"string"},"layerSettings":` + nativeLayout + `},"required":["key","blockId","formId","color","data","position","cell","layerSettings"]}},"returnEdges":{"type":"array","items":{"type":"string"}}},"required":["nodes","returnEdges"]}`

// addFlowchartLayout registers the read-only grid layout of a flowchart; it writes nothing.
func addFlowchartLayout(s *server.MCPServer, c *publicapi.Client) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(layoutOutput)), "flowchart_layout", "Lay out a flowchart before creating it: nodes [{key, blockId}] (kinds from flowchart_blocks_list), directed edges [{source, target}] by key. Returns per node formId, color and data for actors_create (all blocks in one call), position and layerSettings for layer_placements_create (all nodes in one call, then edges_create and their placements): ranks top to bottom, three free cells between blocks and ranks, blocks on whole cells, no overlaps. returnEdges: loop-back edges (\"source->target\") kept out of ranking. Writes nothing.",
		`{"type":"object","properties":{"nodes":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"key":{"type":"string","minLength":1,"maxLength":64},"blockId":{"type":"string","minLength":1,"maxLength":128}},"required":["key","blockId"],"additionalProperties":false}},"edges":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"source":{"type":"string","minLength":1,"maxLength":64},"target":{"type":"string","minLength":1,"maxLength":64}},"required":["source","target"],"additionalProperties":false}}},"required":["nodes"],"additionalProperties":false}`, false,
		func(ctx context.Context, in layoutInput) (layout, error) {
			keys, kinds := map[string]bool{}, []string{}
			seen := map[string]bool{}
			for _, n := range in.Nodes {
				if keys[n.Key] {
					return layout{}, fmt.Errorf("duplicate node key %q", n.Key)
				}
				keys[n.Key] = true
				if !seen[n.BlockID] {
					seen[n.BlockID] = true
					kinds = append(kinds, n.BlockID)
				}
			}
			edges := make([]graphplan.Edge, len(in.Edges))
			for i, e := range in.Edges {
				if !keys[e.Source] || !keys[e.Target] || e.Source == e.Target {
					return layout{}, errors.New("every edge needs two different known node keys")
				}
				edges[i] = graphplan.Edge{Key: e.Source + "->" + e.Target, Source: e.Source, Target: e.Target}
			}
			catalog, err := c.ListFlowchartBlocks(ctx, publicapi.FlowchartBlocksInput{BlockIDs: kinds})
			if err != nil {
				return layout{}, err
			}
			defs := map[string]graphplan.Block{}
			for _, b := range graphplan.ConsoleBlocks(catalog, true).List {
				defs[b.BlockID] = b
			}
			out := layout{Nodes: make([]layoutNode, len(in.Nodes)), ReturnEdges: []string{}}
			boxes := make([]graphplan.LayoutNode, len(in.Nodes))
			for i, n := range in.Nodes {
				def, ok := defs[n.BlockID]
				if !ok {
					return layout{}, fmt.Errorf("flowchart block %q is not in the workspace catalog", n.BlockID)
				}
				color := def.Color // the block's own colour, as the editor gives it; else a random palette hue
				if color == "" {
					color = publicapi.RandomActorColor()
				}
				out.Nodes[i] = layoutNode{Key: n.Key, BlockID: n.BlockID, FormID: def.FormID, Color: color, Data: def.Data, LayerSettings: *def.LayerSettings}
				boxes[i] = graphplan.LayoutNode{Key: n.Key, Width: def.LayerSettings.Width, Height: def.LayerSettings.Height}
			}
			positions, returns := graphplan.GridLayout(boxes, edges)
			for i := range out.Nodes {
				out.Nodes[i].Position = positions[i]
				out.Nodes[i].Cell = publicapi.CellOf(graphplan.GridCenter(positions[i], out.Nodes[i].LayerSettings.Width, out.Nodes[i].LayerSettings.Height))
			}
			out.ReturnEdges = append(out.ReturnEdges, returns...)
			return out, nil
		})
}

// placedNode is a created node placement with the Graph Editor cell of its position.
type placedNode struct {
	publicapi.GraphCreatedNode
	Cell string `json:"cell,omitempty"`
}

type consolePlacements struct {
	Nodes   []placedNode                          `json:"nodes"`
	Edges   []publicapi.GraphCreatedPlacementEdge `json:"edges"`
	Moved   int                                   `json:"moved,omitempty"`
	Opened  *publicapi.LayerNodeOpen              `json:"opened,omitempty"`
	Warning string                                `json:"warning,omitempty"`
}

// gridPlacements places flowchart nodes as Graph Editor does: corner on a grid line and the block
// silhouette as areaPicture, so sim-api keeps the position instead of snapping the centre. A node
// given by its cell is placed at the cell's centre; every placed node answers its cell.
func gridPlacements(c *publicapi.Client) func(context.Context, publicapi.PlacementsCreateInput) (consolePlacements, error) {
	return func(ctx context.Context, in publicapi.PlacementsCreateInput) (consolePlacements, error) {
		for i := range in.Nodes {
			if err := in.Nodes[i].ResolveCell(); err != nil {
				return consolePlacements{}, fmt.Errorf("node %d: %w", i, err)
			}
		}
		if err := fillActorColors(ctx, c, in.Nodes); err != nil {
			return consolePlacements{}, err
		}
		for i := range in.Nodes {
			n := &in.Nodes[i]
			if n.LayerSettings == nil {
				continue
			}
			picture, err := graphplan.AreaPicture(*n.LayerSettings, n.Color)
			if err != nil {
				return consolePlacements{}, err
			}
			n.Position = graphplan.GridCenter(n.Position, n.LayerSettings.Width, n.LayerSettings.Height)
			n.AreaPicture = &picture
		}
		placed, err := c.CreatePlacements(ctx, in)
		if err != nil {
			return consolePlacements{}, err
		}
		out := consolePlacements{Nodes: make([]placedNode, len(placed.Nodes)), Edges: placed.Edges, Moved: placed.Moved, Opened: placed.Opened, Warning: placed.Warning}
		for i, n := range placed.Nodes {
			at := in.Nodes[i].Position // manage answers nodes in input order
			if n.Position != nil {
				at = *n.Position
			}
			out.Nodes[i] = placedNode{GraphCreatedNode: n, Cell: publicapi.CellOf(at)}
		}
		return out, nil
	}
}

// fillActorColors gives a flowchart node without a colour its actor's own, read in one batch, so the
// stored silhouette matches the block.
func fillActorColors(ctx context.Context, c *publicapi.Client, nodes []publicapi.NodePlacement) error {
	var missing []int
	var items []map[string]json.RawMessage
	for i, n := range nodes {
		if n.LayerSettings != nil && n.Color == "" {
			id, _ := json.Marshal(n.ActorID)
			missing = append(missing, i)
			items = append(items, map[string]json.RawMessage{"actorId": id, "fields": json.RawMessage(`["id","color"]`)})
		}
	}
	if len(items) == 0 {
		return nil
	}
	results, err := c.PassEach(ctx, "actor/get", items)
	if err != nil {
		return err
	}
	for k, r := range results {
		var actor struct {
			Color string `json:"color"`
		}
		if r.Error == "" && json.Unmarshal(r.Result, &actor) == nil {
			nodes[missing[k]].Color = actor.Color
		}
	}
	return nil
}
