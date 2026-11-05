package publicapi

import (
	"context"
	"strings"
	"testing"
)

// expandBackend is a layer with one node to open (laId 7 at B, 4) and a neighbour (laId 8 at K, 4);
// anisotropy is the layer's setting, node the stored placement row.
func expandBackend(t *testing.T, anisotropy, node string) (*Client, *[]rpcCall) {
	batch := `{"isAtomic":true,"list":[{"id":"0","result":{}},{"id":"1","result":{}}]}`
	if anisotropy == "false" && !strings.Contains(node, "expand") {
		batch = `{"isAtomic":true,"list":[{"id":"0","result":{}}]}` // a quiet open sends no offset
	}
	return rpcBackend(t, func(method string) string {
		switch method {
		case "graph/layer/elements/list":
			return `{"nodes":[` + node + `]}`
		case "actor/get":
			return `{"id":"L","formId":7,"data":{"anisotropy":` + anisotropy + `}}`
		case "graph/layer/actors/list":
			return `{"list":[` + node + `,{"laId":8,"id":"n","position":{"x":500,"y":150}}]}`
		case "form/system/get":
			return `{"id":9}`
		case "batch":
			return batch
		}
		return `{}`
	})
}

const closedNode = `{"laId":7,"id":"a","position":{"x":50,"y":150},"actor":{"id":"a","formId":7}}`

func methodsOf(calls []rpcCall) string {
	var m []string
	for _, c := range calls {
		m = append(m, c.Method)
	}
	return strings.Join(m, ",")
}

func lastBatch(t *testing.T, calls []rpcCall) string {
	t.Helper()
	last := calls[len(calls)-1]
	if last.Method != "batch" {
		t.Fatalf("last call %s", last.Method)
	}
	return jsonOf(t, last.Params["list"])
}

// Opening on a layer with anisotropy writes the Graph Editor's bag and pushes the rest of the layer
// out of the box, in one atomic batch.
func TestExpandNodeMakesRoom(t *testing.T) {
	c, calls := expandBackend(t, "true", closedNode)
	out, err := c.ExpandNode(context.Background(), NodeExpandInput{LayerID: "L", LaID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if m := methodsOf(*calls); m != "graph/layer/elements/list,actor/get,graph/layer/actors/list,form/system/get,form/system/get,batch" {
		t.Fatal(m)
	}
	want := `[{"id":"0","method":"graph/layer/actor_settings/save","params":{"accId":"ws","actorId":"L","laActorId":7,"layerSettings":{"expand":true,"expandPushed":true,"expandType":"layer","flaskView":true,"height":450,"nestedFrame":{"height":9,"left":-6,"top":-4,"width":13},"width":650}}},` +
		`{"id":"1","method":"graph/layer/actors/offset","params":{"accId":"ws","actorId":"L","bounds":{"bottom":175,"left":25,"right":75,"top":125},"offset":{"bottom":200,"left":300,"right":300,"top":200}}}]`
	if got := lastBatch(t, *calls); got != want {
		t.Fatalf("batch %s\nwant %s", got, want)
	}
	if !out.Expanded || !out.Changed || out.ExpandType != "layer" || out.Area != "-E, -1 - H, 8" || out.Moved != 1 || out.Covers != "" {
		t.Fatalf("%+v", out)
	}
}

// Without anisotropy the box lies over its neighbours, as in the Graph Editor, and says so.
func TestExpandNodeQuiet(t *testing.T) {
	c, calls := expandBackend(t, "false", closedNode)
	out, err := c.ExpandNode(context.Background(), NodeExpandInput{LayerID: "L", LaID: 7, Width: 21, Height: 5})
	if err != nil {
		t.Fatal(err)
	}
	b := lastBatch(t, *calls)
	if strings.Contains(b, "actors/offset") || !strings.Contains(b, `"expandPushed":false`) || !strings.Contains(b, `"nestedFrame":{"height":5,"left":-10,"top":-2,"width":21}`) {
		t.Fatal(b)
	}
	if out.Moved != 0 || !strings.Contains(out.Covers, "laId 8") {
		t.Fatalf("%+v", out)
	}
}

// A node opened before expandPushed existed is closed with a pull back, whatever the setting now.
func TestCollapseNodePullsBack(t *testing.T) {
	c, calls := expandBackend(t, "false", `{"laId":7,"id":"a","position":{"x":50,"y":150},"layerSettings":{"expand":true,"expandType":"layer"},"actor":{"id":"a","formId":7}}`)
	no := false
	out, err := c.ExpandNode(context.Background(), NodeExpandInput{LayerID: "L", LaID: 7, Expand: &no})
	if err != nil {
		t.Fatal(err)
	}
	if m := methodsOf(*calls); m != "graph/layer/elements/list,actor/get,graph/layer/actors/list,batch" {
		t.Fatal(m)
	}
	if b := lastBatch(t, *calls); !strings.Contains(b, `"layerSettings":{"expand":false,"flaskView":false,"fullScreen":false,"profile":false}`) ||
		!strings.Contains(b, `"offset":{"bottom":-200,"left":-300,"right":-300,"top":-200}`) {
		t.Fatal(b)
	}
	if out.Expanded || !out.Changed || out.Area != "" {
		t.Fatalf("%+v", out)
	}
}

func TestExpandNodeRefusalsAndNoOps(t *testing.T) {
	block := `{"laId":7,"id":"a","position":{"x":50,"y":150},"layerSettings":{"blockId":"process"}}`
	c, calls := expandBackend(t, "true", block)
	if _, err := c.ExpandNode(context.Background(), NodeExpandInput{LayerID: "L", LaID: 7}); err == nil || !strings.Contains(err.Error(), "flowchart") {
		t.Error("a flowchart block opened:", err)
	}
	card := `{"laId":7,"id":"a","position":{"x":50,"y":150},"layerSettings":{"profile":true}}`
	c2, _ := expandBackend(t, "true", card)
	if _, err := c2.ExpandNode(context.Background(), NodeExpandInput{LayerID: "L", LaID: 7}); err == nil || !strings.Contains(err.Error(), "card") {
		t.Error("an actor card opened:", err)
	}
	open := `{"laId":7,"id":"a","position":{"x":50,"y":150},"layerSettings":{"expand":true,"expandType":"layer"}}`
	c3, calls3 := expandBackend(t, "true", open)
	out, err := c3.ExpandNode(context.Background(), NodeExpandInput{LayerID: "L", LaID: 7})
	if err != nil || out.Changed || !out.Expanded || strings.Contains(methodsOf(*calls3), "batch") {
		t.Errorf("reopening wrote: %+v %v %s", out, err, methodsOf(*calls3))
	}
	no := false
	for _, bad := range []NodeExpandInput{{LayerID: "L"}, {LaID: 7}, {LayerID: "L", LaID: 7, Width: 3, Height: 9},
		{LayerID: "L", LaID: 7, Width: 13}, {LayerID: "L", LaID: 7, Expand: &no, Width: 13, Height: 9}} {
		if _, err := c.ExpandNode(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if strings.Contains(methodsOf(*calls), "batch") {
		t.Error("a refusal wrote", methodsOf(*calls))
	}
}

// placeBackend is a layer whose only node holds A, 1; the manage answer is one node.
func placeBackend(t *testing.T, anisotropy string) (*Client, *[]rpcCall) {
	return rpcBackend(t, func(method string) string {
		switch method {
		case "actor/get":
			return `{"id":"L","formId":7,"data":{"anisotropy":` + anisotropy + `}}`
		case "graph/layer/actors/list":
			return `{"list":[{"laId":1,"id":"x","position":{"x":0,"y":0}}]}`
		case "form/system/get":
			return `{"id":9}`
		case "graph/layer/actors/manage":
			return `{"nodes":[{"laId":11,"id":"a1"}],"edges":[]}`
		case "batch":
			return `{"isAtomic":true,"list":[{"id":"0","result":{}},{"id":"1","result":{"nodes":[{"laId":11,"id":"a1"}],"edges":[]}}]}`
		}
		return `{}`
	})
}

func TestPlacementsRefuseATakenCell(t *testing.T) {
	c, calls := placeBackend(t, "false")
	_, err := c.CreatePlacements(context.Background(), PlacementsCreateInput{LayerID: "L", Nodes: []NodePlacement{{ActorID: "a1", Position: GraphPoint{X: 10, Y: -10}}}})
	if err == nil || !strings.Contains(err.Error(), "A, 1 (laId 1)") || !strings.Contains(err.Error(), "anisotropy is off") {
		t.Fatal(err)
	}
	if strings.Contains(methodsOf(*calls), "manage") {
		t.Error("placed over a taken cell")
	}
	// Two nodes of one call on one cell are refused before anything is read.
	*calls = nil
	two := []NodePlacement{{ActorID: "a1", Position: GraphPoint{X: 100}}, {ActorID: "a2", Position: GraphPoint{X: 110}}}
	if _, err := c.CreatePlacements(context.Background(), PlacementsCreateInput{LayerID: "L", Nodes: two}); err == nil || len(*calls) != 0 {
		t.Error("overlapping nodes accepted", err, *calls)
	}
}

func TestPlacementsMakeRoomWithAnisotropy(t *testing.T) {
	c, calls := placeBackend(t, "true")
	out, err := c.CreatePlacements(context.Background(), PlacementsCreateInput{LayerID: "L", Nodes: []NodePlacement{{ActorID: "a1", Position: GraphPoint{}}}})
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"id":"0","method":"graph/layer/actors/offset","params":{"accId":"ws","actorId":"L","bounds":{"bottom":25,"left":25,"right":25,"top":-25},"offset":{"bottom":0,"left":50,"right":0,"top":0}}},` +
		`{"id":"1","method":"graph/layer/actors/manage","params":{"accId":"ws","actorId":"L","items":[{"action":"create","data":{"id":"a1","position":{"x":0,"y":0},"type":"node"}}]}}]`
	if got := lastBatch(t, *calls); got != want {
		t.Fatalf("batch %s\nwant %s", got, want)
	}
	if out.Moved != 1 || len(out.Nodes) != 1 || out.Nodes[0].LaID != 11 {
		t.Fatalf("%+v", out)
	}
}

func TestPlacementsFreeCellIsOneCall(t *testing.T) {
	c, calls := placeBackend(t, "true")
	out, err := c.CreatePlacements(context.Background(), PlacementsCreateInput{LayerID: "L", Nodes: []NodePlacement{{ActorID: "a1", Position: GraphPoint{X: 50}}}})
	if err != nil || out.Moved != 0 {
		t.Fatal(err, out)
	}
	if m := methodsOf(*calls); m != "actor/get,graph/layer/actors/list,graph/layer/actors/manage" {
		t.Fatal(m)
	}
}

// A node placed with expand is born open: its bag rides the create, its neighbours make room first.
func TestPlacementBornOpen(t *testing.T) {
	c, calls := placeBackend(t, "true")
	out, err := c.CreatePlacements(context.Background(), PlacementsCreateInput{LayerID: "L", Nodes: []NodePlacement{{ActorID: "a1", Position: GraphPoint{X: 500}, Expand: true}}})
	if err != nil {
		t.Fatal(err)
	}
	b := lastBatch(t, *calls)
	if !strings.Contains(b, `"bounds":{"bottom":25,"left":475,"right":525,"top":-25},"offset":{"bottom":200,"left":300,"right":300,"top":200}`) ||
		!strings.Contains(b, `"layerSettings":{"expand":true,"expandPushed":true,"expandType":"layer","flaskView":true,"height":450,"nestedFrame":{"height":9,"left":-6,"top":-4,"width":13},"width":650}`) ||
		strings.Index(b, "actors/offset") > strings.Index(b, "actors/manage") {
		t.Fatal(b)
	}
	if out.Opened == nil || out.Opened.ExpandType != "layer" || out.Opened.Area != "E, -4 - Q, 5" || out.Moved != 1 {
		t.Fatalf("%+v", jsonOf(t, out))
	}
	if _, err := c.CreatePlacements(context.Background(), PlacementsCreateInput{LayerID: "L",
		Nodes: []NodePlacement{{ActorID: "a1", Position: GraphPoint{X: 500}, Expand: true}, {ActorID: "a2", Position: GraphPoint{X: 900}}}}); err == nil {
		t.Error("expand with a second node accepted")
	}
}

func TestLayerGetShowsOpenNodes(t *testing.T) {
	c, _ := rpcBackend(t, func(method string) string {
		switch method {
		case "actor/get":
			return `{"id":"L","data":{}}`
		case "graph/layer/actors/list":
			return `{"list":[{"laId":7,"id":"a","position":{"x":0,"y":0},"layerSettings":{"expand":true,"expandType":"chart"},"actor":{"id":"a","title":"Sales","formId":3}},{"laId":8,"id":"b","position":{"x":500,"y":0}}]}`
		}
		return `{"list":[]}`
	})
	v, err := c.GetLayer(context.Background(), LayerGetInput{LayerID: "L"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Anisotropy || len(v.Nodes) != 2 || jsonOf(t, v.Nodes[0].Open) != `{"expandType":"chart","area":"-F, -4 - G, 5"}` || v.Nodes[1].Open != nil {
		t.Fatal(jsonOf(t, v))
	}
}
