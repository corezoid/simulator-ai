package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parity drives the actor, form and graph parity tools: a template with its schema and edit token,
// two actors (update, status, by ref, search, filter), a labelled edge (get, update) placed on a new
// layer, the layer reads, the graph traversals, then deletes of the edge and one actor. Search and
// filter results come from replicas, so only their success is checked, not their rows.
func parity(c *client, stamp string) error {
	var ignored json.RawMessage
	var form struct {
		ID int64 `json:"id"`
	}
	if err := c.tool("form_create", map[string]any{"title": "AI smoke template " + stamp, "isTemplate": true}, &form); err != nil {
		return err
	}
	var withEditor struct {
		ID        int64           `json:"id"`
		Form      json.RawMessage `json:"form"`
		EditToken string          `json:"editToken"`
	}
	if err := c.tool("form_get", map[string]any{"formId": form.ID, "with": []string{"editor"}}, &withEditor); err != nil {
		return err
	}
	if withEditor.ID != form.ID || len(withEditor.Form) == 0 || withEditor.EditToken == "" {
		return fmt.Errorf("form_get returned no body or edit token for %d", form.ID)
	}
	if err := c.tool("form_update", map[string]any{"formId": form.ID, "description": "updated by smoke", "expectedEditToken": withEditor.EditToken}, &ignored); err != nil {
		return err
	}
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"forms_list", map[string]any{"query": stamp, "limit": 5}},
		{"forms_count", map[string]any{"query": stamp}},
		{"form_accounts_list", map[string]any{"formId": form.ID}},
		{"forms_tree_get", map[string]any{"formId": form.ID}},
		{"form_linked_get", map[string]any{"formId": form.ID, "direction": "children"}},
		{"form_linked_get", map[string]any{"formId": form.ID, "direction": "parents"}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	actors := make([]string, 2)
	for i := range actors {
		var a id
		if err := c.tool("actor_create", map[string]any{"formId": form.ID, "title": fmt.Sprintf("Smoke actor %d %s", i+1, stamp), "ref": fmt.Sprintf("smoke-%s-%d", stamp, i+1)}, &a); err != nil {
			return err
		}
		actors[i] = a.ID
	}
	var updated struct {
		Title string `json:"title"`
	}
	if err := c.tool("actor_update", map[string]any{"actorId": actors[0], "title": "Smoke actor renamed " + stamp}, &updated); err != nil {
		return err
	}
	if !strings.HasPrefix(updated.Title, "Smoke actor renamed") {
		return fmt.Errorf("actor_update returned title %q", updated.Title)
	}
	var byRef id
	if err := c.tool("actor_by_ref_get", map[string]any{"formId": form.ID, "ref": "smoke-" + stamp + "-2", "fields": []string{"id", "title"}}, &byRef); err != nil {
		return err
	}
	if byRef.ID != actors[1] {
		return fmt.Errorf("actor_by_ref_get found %s, want %s", byRef.ID, actors[1])
	}
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"actor_status_update", map[string]any{"actorId": actors[1], "status": "verified"}},
		{"actor_get", map[string]any{"actorId": actors[0], "fields": []string{"id", "title", "data", "status"}, "with": []string{"owner"}}},
		{"actors_search", map[string]any{"query": "Smoke actor", "formId": form.ID, "limit": 5}},
		{"actors_filter", map[string]any{"formId": form.ID, "limit": 5}},
		{"actors_filter_count", map[string]any{"formId": form.ID}},
		{"search_all", map[string]any{"query": "Smoke actor", "filters": []string{"users", "actors"}, "limit": 3}},
		{"actor_corezoid_processes_list", map[string]any{"actorId": actors[0]}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	var layer, edge id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke parity " + stamp}, &layer); err != nil {
		return err
	}
	if err := c.tool("edge_create", map[string]any{"source": actors[0], "target": actors[1], "name": "first"}, &edge); err != nil {
		return err
	}
	var gotEdge struct {
		Name       string `json:"name"`
		EdgeTypeID int64  `json:"edgeTypeId"`
	}
	if err := c.tool("edge_update", map[string]any{"edgeId": edge.ID, "name": "renamed"}, &ignored); err != nil {
		return err
	}
	if err := c.tool("edge_get", map[string]any{"edgeId": edge.ID}, &gotEdge); err != nil {
		return err
	}
	if gotEdge.Name != "renamed" {
		return fmt.Errorf("edge_get name %q after edge_update", gotEdge.Name)
	}
	var placed struct {
		Nodes []struct {
			LaID int64 `json:"laId"`
		} `json:"nodes"`
	}
	nodes := []map[string]any{{"actorId": actors[0], "position": map[string]any{"x": 0, "y": 0}}, {"actorId": actors[1], "position": map[string]any{"x": 0, "y": 200}}}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "nodes": nodes}, &placed); err != nil {
		return err
	}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "edges": []map[string]any{{"edgeId": edge.ID, "laIdSource": placed.Nodes[0].LaID, "laIdTarget": placed.Nodes[1].LaID}}}, &ignored); err != nil {
		return err
	}
	var all struct {
		Count int `json:"count"`
	}
	var systemTypes, found struct {
		List []struct {
			ID       any  `json:"id"`
			IsSystem bool `json:"isSystem"`
		} `json:"list"`
	}
	var pruned struct {
		Scanned int `json:"scanned"`
		Deleted int `json:"deleted"`
	}
	var exists struct {
		Exist bool `json:"exist"`
	}
	for _, step := range []struct {
		tool string
		args map[string]any
		out  any
	}{
		{"layer_placements_list", map[string]any{"actorId": layer.ID, "limit": 10, "fields": []string{"id", "title"}}, &ignored},
		{"layer_edges_list", map[string]any{"actorId": layer.ID}, &ignored},
		{"layer_skeleton_get", map[string]any{"actorId": layer.ID}, &ignored},
		{"layer_stats_get", map[string]any{"actorId": layer.ID}, &ignored},
		{"layer_actors_search", map[string]any{"actorId": layer.ID, "query": "Smoke"}, &ignored},
		{"layer_element_exist", map[string]any{"actorId": layer.ID, "id": actors[0], "type": "node"}, &exists},
		{"layer_placements_all", map[string]any{"layerId": layer.ID}, &all},
		{"graph_list", map[string]any{"actorId": actors[0], "type": "children"}, &ignored},
		{"graph_get", map[string]any{"actorId": actors[0]}, &ignored},
		{"edge_types_list", map[string]any{"limit": 5, "isSystem": true}, &systemTypes},
		{"edge_exist", map[string]any{"source": actors[0], "target": actors[1], "edgeTypeId": gotEdge.EdgeTypeID}, &found},
		{"layer_prune_long_edges", map[string]any{"layerId": layer.ID, "maxDistance": 100, "preserveParentEdges": false, "dryRun": true}, &pruned},
		{"layer_compact", map[string]any{"layerId": layer.ID}, &ignored},
	} {
		if err := c.tool(step.tool, step.args, step.out); err != nil {
			return err
		}
	}
	for _, t := range systemTypes.List {
		if !t.IsSystem {
			return fmt.Errorf("edge_types_list isSystem returned a workspace edge type")
		}
	}
	if len(found.List) != 1 {
		return fmt.Errorf("edge_exist found %d edges between the linked actors, want 1", len(found.List))
	}
	if all.Count != 2 {
		return fmt.Errorf("layer_placements_all counted %d placements, want 2", all.Count)
	}
	if pruned.Scanned != 1 || pruned.Deleted != 1 {
		return fmt.Errorf("prune dryRun scanned %d and would delete %d of the 200-unit edge, want 1 and 1", pruned.Scanned, pruned.Deleted)
	}
	if err := c.tool("edge_delete", map[string]any{"edgeId": edge.ID}, &ignored); err != nil {
		return err
	}
	if err := c.tool("actor_delete", map[string]any{"actorId": actors[1]}, &ignored); err != nil {
		return err
	}
	fmt.Println("OK parity form", form.ID, "layer", layer.ID)
	return nil
}
