package main

import (
	"encoding/json"
	"fmt"
	"math"
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

// graph: layer -> flowchart_layout of Start, Decision, two Process branches and End -> blocks ->
// node placements -> labelled edges -> edge placements -> layer_get, checking that every block
// stays where the layout put it: corners on grid lines (25 + 50k), so it fills whole cells.
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
	for _, title := range []string{"Start / Stop", "Process", "Decision"} {
		if kinds[title] == "" {
			return fmt.Errorf("workspace catalog has no %s block", title)
		}
	}
	plan := []struct{ key, title, kind, color string }{
		{"start", "Start", kinds["Start / Stop"], "#17B26A"}, {"check", "Approved?", kinds["Decision"], "#F79009"},
		{"yes", "Book leave", kinds["Process"], "#539fdf"}, {"no", "Notify employee", kinds["Process"], "#F04438"},
		{"end", "End", kinds["Start / Stop"], "#17B26A"}}
	links := []struct{ source, target, name string }{{"start", "check", "request"}, {"check", "yes", "yes"}, {"check", "no", "no"}, {"yes", "end", ""}, {"no", "end", ""}}
	var nodesIn, edgesIn []map[string]any
	for _, p := range plan {
		nodesIn = append(nodesIn, map[string]any{"key": p.key, "blockId": p.kind})
	}
	for _, l := range links {
		edgesIn = append(edgesIn, map[string]any{"source": l.source, "target": l.target})
	}
	var laid struct {
		Nodes []struct {
			FormID        int64           `json:"formId"`
			Data          json.RawMessage `json:"data"`
			Position      struct{ X, Y float64 }
			LayerSettings struct {
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"layerSettings"`
		} `json:"nodes"`
	}
	var raw struct {
		Nodes []struct {
			LayerSettings json.RawMessage `json:"layerSettings"`
		} `json:"nodes"`
	}
	if err := c.tool("flowchart_layout", map[string]any{"nodes": nodesIn, "edges": edgesIn}, &laid); err != nil {
		return err
	}
	if err := c.reuse(&raw); err != nil {
		return err
	}
	var batch []map[string]any
	for i, p := range plan {
		batch = append(batch, map[string]any{"formId": laid.Nodes[i].FormID, "title": p.title, "color": p.color, "data": laid.Nodes[i].Data})
	}
	var created struct {
		Actors []id `json:"actors"`
	}
	if err := c.tool("actors_create", map[string]any{"actors": batch}, &created); err != nil {
		return err
	}
	if len(created.Actors) != len(plan) {
		return fmt.Errorf("actors_create returned %d actors for %d", len(created.Actors), len(plan))
	}
	actors, nodes := map[string]string{}, []map[string]any{}
	for i, p := range plan {
		n := laid.Nodes[i]
		actors[p.key] = created.Actors[i].ID
		nodes = append(nodes, map[string]any{"actorId": created.Actors[i].ID, "position": map[string]any{"x": n.Position.X, "y": n.Position.Y}, "layerSettings": raw.Nodes[i].LayerSettings, "color": p.color})
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
	var wanted []map[string]any
	for _, l := range links {
		wanted = append(wanted, map[string]any{"source": actors[l.source], "target": actors[l.target], "name": l.name})
	}
	var linked struct {
		Edges []id `json:"edges"`
	}
	if err := c.tool("edges_create", map[string]any{"edges": wanted}, &linked); err != nil {
		return err
	}
	var edges []map[string]any
	for i, l := range links {
		edges = append(edges, map[string]any{"edgeId": linked.Edges[i].ID, "laIdSource": la[actors[l.source]], "laIdTarget": la[actors[l.target]]})
	}
	var ignored json.RawMessage
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "edges": edges}, &ignored); err != nil {
		return err
	}
	var view struct {
		Nodes []struct {
			ActorID  string `json:"actorId"`
			BlockID  string `json:"blockId"`
			Position struct{ X, Y float64 }
		} `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	if err := c.tool("layer_get", map[string]any{"layerId": layer.ID}, &view); err != nil {
		return err
	}
	if len(view.Nodes) != len(plan) || len(view.Edges) != len(links) {
		return fmt.Errorf("layer readback has %d nodes and %d edges, want %d and %d", len(view.Nodes), len(view.Edges), len(plan), len(links))
	}
	want := map[string]int{}
	for i, p := range plan {
		want[actors[p.key]] = i
	}
	onLine := func(v float64) bool { return math.Mod(math.Abs(v-25), 50) == 0 }
	for _, n := range view.Nodes {
		l := laid.Nodes[want[n.ActorID]]
		if n.BlockID == "" || n.Position != l.Position {
			return fmt.Errorf("block %s stored at %v, laid out at %v", n.ActorID, n.Position, l.Position)
		}
		w, h := l.LayerSettings.Width, l.LayerSettings.Height
		if !onLine(n.Position.X-w/2) || !onLine(n.Position.Y-h/2) || !onLine(n.Position.X+w/2) || !onLine(n.Position.Y+h/2) {
			return fmt.Errorf("block %s at %v (%vx%v) is off the grid", n.ActorID, n.Position, w, h)
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
	// The leave request of acceptance scenario 2, built only from smart_form_components types.
	var kinds struct {
		Components []struct {
			Type string `json:"type"`
		} `json:"components"`
	}
	if err := c.tool("smart_form_components", map[string]any{"types": []string{"Stack", "Label", "DatePicker", "TextInput", "Button"}}, &kinds); err != nil {
		return err
	}
	page, _ := json.Marshal(map[string]any{"schemaVersion": "2.0", "id": "index", "tree": map[string]any{"id": "root", "type": "Stack", "children": []any{
		map[string]any{"id": "startLabel", "type": "Label", "props": map[string]any{"title": "Дата початку"}},
		map[string]any{"id": "start", "type": "DatePicker", "props": map[string]any{"mode": "single"}},
		map[string]any{"id": "endLabel", "type": "Label", "props": map[string]any{"title": "Дата кінця"}},
		map[string]any{"id": "end", "type": "DatePicker", "props": map[string]any{"mode": "single"}},
		map[string]any{"id": "comment", "type": "TextInput", "props": map[string]any{"label": "Коментар"}},
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
	if !rel.Ready || !again.Ready || rel.Marker != "appId: "+created.AppID {
		return fmt.Errorf("release %d is not ready", rel.ReleaseID)
	}
	fmt.Println("OK form appId", created.AppID, "release", rel.ReleaseID)
	return nil
}
