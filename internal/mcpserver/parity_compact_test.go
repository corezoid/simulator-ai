package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

func TestPlacementRowsDropPicturesAndShapes(t *testing.T) {
	in := `{"list":[{"laId":1,"id":"a","position":{"x":12.6,"y":50},"cellPosition":{"x":"A","y":2},"areaPicture":{"img":"data:image/svg+xml;base64,AAAA"},` +
		`"layerSettings":{"blockId":"process","width":200,"shape":{"type":"primitive"},"textFrame":{"x":1}},"createdAt":1,"actor":{"id":"a","title":"A"}}],"hasMore":true}`
	got := string(placementRows(json.RawMessage(in)))
	for _, gone := range []string{"areaPicture", "cellPosition", "shape", "textFrame", "12.6", "createdAt"} {
		if strings.Contains(got, gone) {
			t.Errorf("kept %s: %s", gone, got)
		}
	}
	for _, kept := range []string{`"position":{"x":13,"y":50}`, `"blockId":"process"`, `"width":200`, `"actor":{"title":"A"}`, `"id":"a"`, `"hasMore":true`} {
		if !strings.Contains(got, kept) {
			t.Errorf("lost %s: %s", kept, got)
		}
	}
	if string(placementRows(json.RawMessage(`[1]`))) != `[1]` {
		t.Error("a non-object answer changed")
	}
}

func TestEdgeRowsKeepTypeEndsAndLabel(t *testing.T) {
	in := `{"list":[{"laId":5,"id":"e","laIdSource":1,"laIdTarget":2,"edge":{"id":"e","edgeTypeId":1,"source":"a","target":"b","userId":1,"name":"","formField":false,"createdAt":1,"updatedAt":1,"pinned":false}},` +
		`{"laId":6,"id":"f","edge":{"edgeTypeId":2,"source":"b","target":"c","name":"yes","hole":true}}]}`
	got := string(edgeRows(json.RawMessage(in)))
	if !strings.Contains(got, `"edge":{"edgeTypeId":1,"source":"a","target":"b"}`) || !strings.Contains(got, `"edge":{"edgeTypeId":2,"hole":true,"name":"yes","source":"b","target":"c"}`) {
		t.Fatal(got)
	}
}

func TestSkeletonCapsWithTotalsAndBox(t *testing.T) {
	var nodes, edges []string
	for i := 0; i < skeletonCap+5; i++ {
		nodes = append(nodes, `{"laId":1,"id":"n","position":{"x":`+itoa(i*10)+`,"y":-`+itoa(i)+`},"cellPosition":{"x":"A","y":1}}`)
		edges = append(edges, `{"laId":2,"id":"e","laIdSource":1,"laIdTarget":1}`)
	}
	got := skeleton(json.RawMessage(`{"nodes":[` + strings.Join(nodes, ",") + `],"edges":[` + strings.Join(edges, ",") + `]}`))
	var out struct {
		Nodes, Edges []map[string]any
		Total        map[string]int
		Box          map[string]float64
		Truncated    bool
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != skeletonCap || len(out.Edges) != skeletonCap || !out.Truncated || out.Total["nodes"] != skeletonCap+5 ||
		out.Box["x2"] != float64((skeletonCap+4)*10) || out.Box["y1"] != -float64(skeletonCap+4) || out.Nodes[0]["cellPosition"] != nil {
		t.Fatalf("%+v %v %v", out.Total, out.Box, out.Truncated)
	}
	small := string(skeleton(json.RawMessage(`{"nodes":[],"edges":null}`)))
	if small != `{"edges":[],"nodes":[],"total":{"edges":0,"nodes":0}}` {
		t.Fatal(small)
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestCompactAllSummarizesAndCaps(t *testing.T) {
	x, y, f1, f2 := 75.0, -30.0, int64(7), int64(9)
	list := []publicapi.LayerPlacement{
		{LaID: 1, ActorID: "a", FormID: &f1, Position: &publicapi.Position{X: &x, Y: &y}},
		{LaID: 2, ActorID: "b", FormID: &f2},
		{LaID: 3, ActorID: "c", FormID: &f1},
	}
	out := compactAll(list, allPlacementsInput{Limit: 2})
	if out.Count != 3 || !out.Truncated || len(out.List) != 2 || out.List[0].Position != nil || out.List[0].Cell != "C, -1" ||
		out.Summary == nil || out.Summary.Forms[0] != (publicapi.FormCount{FormID: 7, Count: 2}) || *out.Summary.Box != (publicapi.GraphBox{X1: 75, Y1: -30, X2: 75, Y2: -30}) {
		t.Fatalf("%+v %+v", out, out.Summary)
	}
	if list[0].Position == nil {
		t.Fatal("compacting changed the caller's rows")
	}
	if full := compactAll(list, allPlacementsInput{Full: true}); len(full.List) != 3 || full.Summary != nil || full.List[0].Position == nil {
		t.Fatalf("%+v", full)
	}
}
