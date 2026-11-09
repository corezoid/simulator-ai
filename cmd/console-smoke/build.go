package main

import (
	"fmt"
)

type built struct {
	Done    bool   `json:"done"`
	LayerID string `json:"layerId"`
	Nodes   struct {
		Done     int `json:"done"`
		Existing int `json:"existing"`
	} `json:"nodes"`
	Edges struct {
		Done     int `json:"done"`
		Existing int `json:"existing"`
	} `json:"edges"`
	Error string `json:"error"`
}

// build (optional, not part of `make smoke`): graph_build of 300 Process blocks in a three-way tree
// on a new layer, the same call again (nothing built twice), the layer's counts, then the removal of
// the operation.
func build(c *client, stamp string) error {
	const n = 300
	var nodes, edges [][]string // the short tuples: [key, title], [from, to]
	for i := range n {
		nodes = append(nodes, []string{fmt.Sprintf("n%d", i), fmt.Sprintf("Step %d", i)})
		if i > 0 {
			edges = append(edges, []string{fmt.Sprintf("n%d", (i-1)/3), fmt.Sprintf("n%d", i)})
		}
	}
	args := map[string]any{"operationId": "smoke-" + stamp, "newLayer": "AI smoke large graph " + stamp, "kind": "process", "nodes": nodes, "edges": edges}
	var first, again built
	if err := c.tool("graph_build", args, &first); err != nil {
		return err
	}
	if !first.Done || first.Nodes.Done != n || first.Edges.Done != n-1 {
		return fmt.Errorf("graph_build: %+v", first)
	}
	if err := c.tool("graph_build", args, &again); err != nil {
		return err
	}
	if !again.Done || again.LayerID != first.LayerID || again.Nodes.Existing != n || again.Nodes.Done != 0 || again.Edges.Done != 0 {
		return fmt.Errorf("graph_build repeated: %+v", again)
	}
	var stats struct {
		Nodes int `json:"nodes"`
		Edges int `json:"edges"`
	}
	if err := c.tool("layer_stats_get", map[string]any{"actorId": first.LayerID}, &stats); err != nil {
		return err
	}
	if stats.Nodes != n || stats.Edges != n-1 {
		return fmt.Errorf("layer has %d nodes and %d edges after a repeated build", stats.Nodes, stats.Edges)
	}
	var gone struct {
		Found   int `json:"found"`
		Deleted int `json:"deleted"`
	}
	if err := c.tool("graph_build_delete", map[string]any{"layerId": first.LayerID, "operationId": "smoke-" + stamp}, &gone); err != nil {
		return err
	}
	if gone.Found != n || gone.Deleted != n {
		return fmt.Errorf("graph_build_delete: %+v", gone)
	}
	fmt.Println("build ok: layer", first.LayerID)
	return nil
}
