package mcpserver

import (
	"encoding/json"
	"math"
)

// A compactor shrinks a parity read's answer to what a model acts on unless the call sends full
// true: the pictures, shapes and duplicate addresses of placements are most of a large layer's bytes.
type compactor func(json.RawMessage) json.RawMessage

// fullParam is the schema of the full switch every compacted read takes.
// fullLocal is the full switch every compacted read takes (a Local param, never sent); the tool's
// description says what full adds: every byte of a schema rides in the model context.
var fullLocal = map[string]json.RawMessage{"full": json.RawMessage(`{"type":"boolean"}`)}

// compactUnlessFull is the Shape of a compacted read: the backend's answer as is with full true.
func compactUnlessFull(c compactor) func(map[string]json.RawMessage, json.RawMessage) (json.RawMessage, error) {
	return func(local map[string]json.RawMessage, v json.RawMessage) (json.RawMessage, error) {
		if string(local["full"]) == "true" {
			return v, nil
		}
		return c(v), nil
	}
}

// skeletonCap bounds each kind of a compact skeleton; the rest is counted, and box narrows the read.
const skeletonCap = 150

type jsonObj = map[string]json.RawMessage

// placementRows drops from each placement row of list its picture, its shape, its cell twin of the
// position, its timestamp and the actor's copy of the row id (a flowchart block's base64 SVG alone
// is ~1 KB).
func placementRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", compactPlacement)
}

func compactPlacement(row jsonObj) {
	delete(row, "areaPicture")
	delete(row, "cellPosition")
	delete(row, "createdAt")
	var actor jsonObj
	if json.Unmarshal(row["actor"], &actor) == nil && actor != nil && string(actor["id"]) == string(row["id"]) {
		delete(actor, "id") // the row's id is the actor's
		row["actor"], _ = json.Marshal(actor)
	}
	roundPosition(row)
	var settings jsonObj
	if json.Unmarshal(row["layerSettings"], &settings) == nil && settings != nil {
		delete(settings, "shape")
		delete(settings, "textFrame")
		row["layerSettings"], _ = json.Marshal(settings)
	}
}

// edgeRows keeps of each edge row's edge what tells it apart: type, ends and its label.
func edgeRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", func(row jsonObj) {
		var edge jsonObj
		if json.Unmarshal(row["edge"], &edge) != nil || edge == nil {
			return
		}
		keep := jsonObj{}
		for _, k := range []string{"edgeTypeId", "source", "target", "name", "hole"} {
			if x, ok := edge[k]; ok && string(x) != `""` && string(x) != "false" && string(x) != "null" {
				keep[k] = x
			}
		}
		row["edge"], _ = json.Marshal(keep)
	})
}

// skeleton answers at most skeletonCap nodes with exact positions and the edges between them (at
// most skeletonCap; an edge to a node not answered is left out), the backend's totals and the box
// the centres of the nodes read lie in (the whole layer, or the x1,y1,x2,y2 zone asked for), so the
// model can read a region next.
func skeleton(v json.RawMessage) json.RawMessage {
	var doc map[string][]jsonObj
	if json.Unmarshal(v, &doc) != nil {
		return v
	}
	nodes, edges := doc["nodes"], doc["edges"]
	out := map[string]any{"total": map[string]int{"nodes": len(nodes), "edges": len(edges)}}
	var box *[4]float64
	for _, n := range nodes {
		delete(n, "cellPosition")
		roundPosition(n)
		var p struct{ X, Y *float64 }
		if json.Unmarshal(n["position"], &p) != nil || p.X == nil || p.Y == nil {
			continue
		}
		if box == nil {
			box = &[4]float64{*p.X, *p.Y, *p.X, *p.Y}
		}
		box[0], box[1], box[2], box[3] = math.Min(box[0], *p.X), math.Min(box[1], *p.Y), math.Max(box[2], *p.X), math.Max(box[3], *p.Y)
	}
	if box != nil {
		out["box"] = map[string]float64{"x1": box[0], "y1": box[1], "x2": box[2], "y2": box[3]}
	}
	if len(nodes) > skeletonCap {
		nodes = nodes[:skeletonCap]
	}
	kept := map[string]bool{}
	for _, n := range nodes {
		kept[string(n["laId"])] = true
	}
	var between []jsonObj
	for _, e := range edges {
		if kept[string(e["laIdSource"])] && kept[string(e["laIdTarget"])] && len(between) < skeletonCap {
			between = append(between, e)
		}
	}
	if len(nodes) < len(doc["nodes"]) || len(between) < len(edges) {
		out["truncated"] = true
	}
	edges = between
	out["nodes"], out["edges"] = nonNil(nodes), nonNil(edges)
	b, err := json.Marshal(out)
	if err != nil {
		return v
	}
	return b
}

func nonNil(rows []jsonObj) []jsonObj {
	if rows == nil {
		return []jsonObj{}
	}
	return rows
}

// roundPosition writes a row's position in whole graph units (a 50-unit grid needs no fractions).
func roundPosition(row jsonObj) {
	var p jsonObj
	if json.Unmarshal(row["position"], &p) != nil || p == nil {
		return
	}
	for _, k := range []string{"x", "y"} {
		var f float64
		if json.Unmarshal(p[k], &f) == nil && f != math.Trunc(f) {
			p[k], _ = json.Marshal(math.Round(f))
		}
	}
	row["position"], _ = json.Marshal(p)
}

// eachRow applies fn to every object row of the result's key array; anything else passes as is.
func eachRow(v json.RawMessage, key string, fn func(jsonObj)) json.RawMessage {
	var doc jsonObj
	var rows []jsonObj
	if json.Unmarshal(v, &doc) != nil || json.Unmarshal(doc[key], &rows) != nil {
		return v
	}
	for _, r := range rows {
		if r != nil {
			fn(r)
		}
	}
	doc[key], _ = json.Marshal(rows)
	b, err := json.Marshal(doc)
	if err != nil {
		return v
	}
	return b
}
