package main

import (
	"encoding/json"
	"fmt"
)

// onePixel is a 1x1 transparent PNG.
const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// extras (console-full): an edge drawn dashed, thick and red on its layer, an actor's picture set
// from an image, and the workspace's Corezoid projects, stages and processes read.
func extras(c *client, stamp string) error {
	var layer, a, b id
	for _, step := range []struct {
		title string
		out   *id
	}{{"AI smoke extras " + stamp, &layer}, {"AI smoke from " + stamp, &a}, {"AI smoke to " + stamp, &b}} {
		if err := c.tool("layer_create", map[string]any{"title": step.title}, step.out); err != nil {
			return err
		}
	}
	var edge id
	if err := c.tool("edge_create", map[string]any{"source": a.ID, "target": b.ID}, &edge); err != nil {
		return err
	}
	var nodes struct {
		Nodes []struct {
			LaID int64 `json:"laId"`
		} `json:"nodes"`
	}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "nodes": []any{map[string]any{"actorId": a.ID, "position": map[string]any{"x": 0, "y": 0}},
		map[string]any{"actorId": b.ID, "position": map[string]any{"x": 300, "y": 0}}}}, &nodes); err != nil {
		return err
	}
	var edges struct {
		Edges []struct {
			LaID int64 `json:"laId"`
		} `json:"edges"`
	}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "edges": []any{map[string]any{"edgeId": edge.ID, "laIdSource": nodes.Nodes[0].LaID, "laIdTarget": nodes.Nodes[1].LaID}}}, &edges); err != nil {
		return err
	}
	var ignored json.RawMessage
	if err := c.tool("layer_edge_settings_save", map[string]any{"actorId": layer.ID, "laEdgeId": edges.Edges[0].LaID, "layerSettings": map[string]any{"lineStyle": "dashed", "width": 3, "color": "#F04438"}}, &ignored); err != nil {
		return err
	}
	var drawn struct {
		List []struct {
			LayerSettings struct {
				LineStyle string `json:"lineStyle"`
				Width     int    `json:"width"`
				Color     string `json:"color"`
			} `json:"layerSettings"`
		} `json:"list"`
	}
	if err := c.tool("layer_edges_list", map[string]any{"actorId": layer.ID}, &drawn); err != nil {
		return err
	}
	if len(drawn.List) != 1 || drawn.List[0].LayerSettings.LineStyle != "dashed" || drawn.List[0].LayerSettings.Width != 3 || drawn.List[0].LayerSettings.Color != "#F04438" {
		return fmt.Errorf("the edge is drawn as %+v", drawn.List)
	}
	var picture struct {
		Picture string `json:"picture"`
	}
	if err := c.tool("actor_picture_upload", map[string]any{"actorId": a.ID, "fileName": "smoke-" + stamp + ".png", "content": onePixel}, &picture); err != nil {
		return err
	}
	var actor struct {
		Picture string `json:"picture"`
	}
	if err := c.tool("actor_get", map[string]any{"actorId": a.ID, "fields": []string{"picture"}}, &actor); err != nil {
		return err
	}
	if picture.Picture == "" || actor.Picture != picture.Picture {
		return fmt.Errorf("actor %s shows picture %q, set %q", a.ID, actor.Picture, picture.Picture)
	}
	if err := corezoidReads(c); err != nil {
		return err
	}
	fmt.Println("OK extras layer", layer.ID, "picture", picture.Picture)
	return nil
}

// corezoidReads walks the first Corezoid project down to its processes; it binds nothing.
func corezoidReads(c *client) error {
	var projects struct {
		Projects []struct {
			ID int64 `json:"id"`
		} `json:"projects"`
	}
	if err := c.tool("corezoid_projects_list", map[string]any{}, &projects); err != nil {
		return err
	}
	if len(projects.Projects) == 0 {
		fmt.Println("SKIP corezoid stages: the workspace has no Corezoid project")
		return nil
	}
	var stages struct {
		Stages []struct {
			ID int64 `json:"id"`
		} `json:"stages"`
	}
	project := projects.Projects[0].ID
	if err := c.tool("corezoid_stages_list", map[string]any{"projectId": project}, &stages); err != nil {
		return err
	}
	if len(stages.Stages) == 0 {
		return fmt.Errorf("corezoid project %d has no stages", project)
	}
	var ignored json.RawMessage
	return c.tool("corezoid_processes_list", map[string]any{"projectId": project, "stageId": stages.Stages[0].ID}, &ignored)
}
