package mcpserver

import (
	"encoding/json"
	"math"
)

// A compactor shrinks a parity read's answer to what a model acts on unless the call sends full
// true: the pictures, shapes and duplicate addresses of placements are most of a large layer's bytes.
type compactor func(json.RawMessage) json.RawMessage

// fullParam is the schema of the full switch every compacted read takes.
// The tool's description says what full adds: every byte of a schema rides in the model context.
const fullParam = `{"type":"boolean"}`

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

// skeleton answers at most skeletonCap nodes and edges with exact positions, the totals and the box
// every node centre lies in, so the model can read a region with x1,y1,x2,y2.
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
	if len(nodes) > skeletonCap || len(edges) > skeletonCap {
		out["truncated"] = true
		nodes, edges = nodes[:min(len(nodes), skeletonCap)], edges[:min(len(edges), skeletonCap)]
	}
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

// withFull adds the full switch to a tool's input schema.
func withFull(schema json.RawMessage) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(doc["properties"], &properties); err != nil {
		return nil, err
	}
	properties["full"] = json.RawMessage(fullParam)
	doc["properties"], _ = json.Marshal(properties)
	return json.Marshal(doc)
}
