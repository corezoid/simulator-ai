package main

import (
	"encoding/json"
	"fmt"
)

type block struct {
	FormID        int64           `json:"formId"`
	Title         string          `json:"title"`
	BlockID       string          `json:"blockId"`
	Data          json.RawMessage `json:"data"`
	LayerSettings json.RawMessage `json:"layerSettings"`
}

type blocks struct {
	List []block `json:"list"`
}

type id struct {
	ID string `json:"id"`
}

// graph: layer -> Start / Process / End blocks -> node placements -> two labelled edges -> edge
// placements -> layer_get readback.
func graph(c *client, stamp string) error {
	var layer id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke flowchart " + stamp}, &layer); err != nil {
		return err
	}
	var catalog blocks
	if err := c.tool("flowchart_blocks_list", map[string]any{}, &catalog); err != nil {
		return err
	}
	kinds := map[string]string{}
	for _, b := range catalog.List {
		kinds[b.Title] = b.BlockID
	}
	start, step := kinds["Start / Stop"], kinds["Process"]
	if start == "" || step == "" {
		return fmt.Errorf("workspace catalog has no Start / Stop or Process block")
	}
	var full blocks
	if err := c.tool("flowchart_blocks_list", map[string]any{"blockIds": []string{start, step}}, &full); err != nil {
		return err
	}
	defs := map[string]block{}
	for _, b := range full.List {
		defs[b.BlockID] = b
	}
	plan := []struct{ title, kind, color string }{{"Start", start, "#17B26A"}, {"Fill request", step, "#539fdf"}, {"End", start, "#17B26A"}}
	actors := make([]string, len(plan))
	nodes := make([]map[string]any, len(plan))
	for i, p := range plan {
		var a id
		if err := c.tool("actor_create", map[string]any{"formId": defs[p.kind].FormID, "title": p.title, "color": p.color, "data": defs[p.kind].Data}, &a); err != nil {
			return err
		}
		actors[i] = a.ID
		nodes[i] = map[string]any{"actorId": a.ID, "position": map[string]any{"x": 0, "y": 200 * i}, "layerSettings": defs[p.kind].LayerSettings}
	}
	var placed struct {
		Nodes []struct {
			LaID    int64  `json:"laId"`
			ActorID string `json:"actorId"`
		} `json:"nodes"`
	}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "nodes": nodes}, &placed); err != nil {
		return err
	}
	la := map[string]int64{}
	for _, n := range placed.Nodes {
		la[n.ActorID] = n.LaID
	}
	var edges []map[string]any
	for i, label := range []string{"begin", "done"} {
		var e id
		if err := c.tool("edge_create", map[string]any{"source": actors[i], "target": actors[i+1], "name": label}, &e); err != nil {
			return err
		}
		edges = append(edges, map[string]any{"edgeId": e.ID, "laIdSource": la[actors[i]], "laIdTarget": la[actors[i+1]]})
	}
	var ignored json.RawMessage
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "edges": edges}, &ignored); err != nil {
		return err
	}
	var view struct {
		Nodes []struct {
			BlockID string `json:"blockId"`
		} `json:"nodes"`
		Edges []struct {
			Name string `json:"name"`
		} `json:"edges"`
	}
	if err := c.tool("layer_get", map[string]any{"layerId": layer.ID}, &view); err != nil {
		return err
	}
	if len(view.Nodes) != 3 || len(view.Edges) != 2 {
		return fmt.Errorf("layer readback has %d nodes and %d edges, want 3 and 2", len(view.Nodes), len(view.Edges))
	}
	for _, n := range view.Nodes {
		if n.BlockID == "" {
			return fmt.Errorf("a node placement has no flowchart blockId")
		}
	}
	fmt.Println("OK graph layer", layer.ID)
	return nil
}

// form: Smart Form -> pages/index/config with a field and a submit button -> release -> release_get.
func form(c *client, stamp string) error {
	var created struct {
		AppID string `json:"appId"`
	}
	if err := c.tool("smart_form_create", map[string]any{"ref": "ai_smoke_leave_" + stamp, "title": "Заявка на відпустку (smoke)"}, &created); err != nil {
		return err
	}
	var tree struct {
		Files []struct {
			ID         int64  `json:"id"`
			Path       string `json:"path"`
			Type       string `json:"type"`
			SourceHash string `json:"sourceHash"`
		} `json:"files"`
	}
	if err := c.tool("smart_form_files_get", map[string]any{"appId": created.AppID}, &tree); err != nil {
		return err
	}
	var config map[string]any
	for _, f := range tree.Files {
		if f.Path == "pages/index/config" {
			config = map[string]any{"id": f.ID, "title": "config", "type": f.Type, "ifMatchHash": f.SourceHash}
		}
	}
	if config == nil {
		return fmt.Errorf("new Smart Form has no pages/index/config")
	}
	page, _ := json.Marshal(map[string]any{"schemaVersion": "2.0", "id": "index", "tree": map[string]any{"id": "root", "type": "Stack", "children": []any{
		map[string]any{"id": "days", "type": "TextInput", "props": map[string]any{"label": "Кількість днів", "required": true}},
		map[string]any{"id": "submit", "type": "Button", "props": map[string]any{"title": "Надіслати"}},
	}}})
	config["source"] = string(page)
	var ignored json.RawMessage
	if err := c.tool("smart_form_files_update", map[string]any{"appId": created.AppID, "items": []any{config}}, &ignored); err != nil {
		return err
	}
	var rel, again struct {
		ReleaseID int64  `json:"releaseId"`
		Ready     bool   `json:"ready"`
		Marker    string `json:"marker"`
	}
	if err := c.tool("smart_form_release", map[string]any{"appId": created.AppID}, &rel); err != nil {
		return err
	}
	if err := c.tool("smart_form_release_get", map[string]any{"appId": created.AppID, "releaseId": rel.ReleaseID}, &again); err != nil {
		return err
	}
	if !rel.Ready || !again.Ready || rel.Marker != "smart-form: "+created.AppID {
		return fmt.Errorf("release %d is not ready", rel.ReleaseID)
	}
	fmt.Println("OK form appId", created.AppID, "release", rel.ReleaseID)
	return nil
}
