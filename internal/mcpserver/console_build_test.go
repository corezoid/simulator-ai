package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/corezoid/simulator-ai/internal/simfake"
)

// fakeConsole is the console-full server over an in-memory backend; call runs a tool and answers
// its structuredContent (or the error text) and whether it failed.
func fakeConsole(t *testing.T) (func(tool, args string) (string, bool), *simfake.Server) {
	t.Helper()
	f := simfake.New()
	t.Cleanup(f.Close)
	c, err := publicapi.New(publicapi.Config{Origin: f.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator user-jwt"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	id := 0
	return func(tool, args string) (string, bool) {
		id++
		raw, err := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, tool, args))))
		if err != nil {
			t.Fatal(err)
		}
		var reply struct {
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				Structured json.RawMessage `json:"structuredContent"`
			} `json:"result"`
		}
		if err = json.Unmarshal(raw, &reply); err != nil || len(reply.Result.Content) == 0 {
			t.Fatal(string(raw), err)
		}
		if reply.Result.IsError {
			return reply.Result.Content[0].Text, true
		}
		return string(reply.Result.Structured), false
	}, f
}

// buildArgs are n plain nodes n0.. in a three-way tree, as a model would send them: short tuples.
func buildArgs(head string, n int) string {
	var nodes, edges []string
	for i := range n {
		nodes = append(nodes, fmt.Sprintf(`["n%d","Step %d"]`, i, i))
		if i > 0 {
			edges = append(edges, fmt.Sprintf(`["n%d","n%d"]`, (i-1)/3, i))
		}
	}
	return `{` + head + `,"formId":42,"nodes":[` + strings.Join(nodes, ",") + `],"edges":[` + strings.Join(edges, ",") + `]}`
}

// cellsOf is the layer's placements as a sorted set of positions, and fails on a shared cell.
func cellsOf(t *testing.T, f *simfake.Server, layer string, nodes, edges int) map[[2]float64]bool {
	t.Helper()
	_, placed, drawn := f.Snapshot(layer)
	out := map[[2]float64]bool{}
	for _, n := range placed {
		out[[2]float64{n.X, n.Y}] = true
	}
	if len(placed) != nodes || len(out) != nodes || len(drawn) != edges {
		t.Fatalf("layer %s: %d nodes on %d cells and %d edges, want %d and %d", layer, len(placed), len(out), len(drawn), nodes, edges)
	}
	return out
}

func layerOf(t *testing.T, reply string) string {
	t.Helper()
	var out struct {
		Done    bool   `json:"done"`
		LayerID string `json:"layerId"`
	}
	if err := json.Unmarshal([]byte(reply), &out); err != nil || !out.Done || out.LayerID == "" {
		t.Fatal(reply, err)
	}
	return out.LayerID
}

func TestGraphBuildToolLaysOutAndBuilds(t *testing.T) {
	call, f := fakeConsole(t)
	reply, failed := call("graph_build", buildArgs(`"operationId":"tree","newLayer":"Process map"`, 600))
	if failed || len(reply) > 200 || strings.Contains(reply, `"ids"`) || strings.Contains(reply, `"chunks"`) || !strings.Contains(reply, `"nodes":{"total":600,"done":600}`) || !strings.Contains(reply, `"area":"A, 1 - `) {
		t.Fatal(len(reply), reply)
	}
	layer := layerOf(t, reply)
	clean := cellsOf(t, f, layer, 600, 599)
	// A tree is laid out by rank: the root alone on the first row, and no row wider than 60 cells.
	rows := map[float64]int{}
	for at := range clean {
		rows[at[1]]++
		if at[0] < 0 || at[0] > 60*50 {
			t.Fatalf("a node at %v is outside the row width", at)
		}
	}
	if rows[0] != 1 || len(rows) < 30 {
		t.Fatalf("%d rows, %d nodes on the first", len(rows), rows[0])
	}

	// The same build with a lost answer and a repeat ends as the same picture, on another layer.
	batches := 0
	f.Set(func(s *simfake.Server) {
		s.Hook = func(_ int, method string) simfake.Fault {
			if method == "batch" {
				if batches++; batches == 2 {
					return simfake.Lose
				}
			}
			return simfake.Answer
		}
	})
	args := buildArgs(`"operationId":"tree2","newLayer":"Process map 2"`, 600)
	if reply, failed = call("graph_build", args); failed || !strings.Contains(reply, `"done":false`) || !strings.Contains(reply, `"status":"unknown"`) {
		t.Fatal(reply)
	}
	f.Set(func(s *simfake.Server) { s.Hook = nil })
	if reply, failed = call("graph_build", args); failed || !strings.Contains(reply, `"existing":198`) {
		t.Fatal(reply)
	}
	again := cellsOf(t, f, layerOf(t, reply), 600, 599)
	for at := range clean {
		if !again[at] {
			t.Fatalf("the repeated build has no node at %v", at)
		}
	}

	// A second section lands under the first unasked, joins it by the first operation's keys, and
	// answers its few ids.
	reply, failed = call("graph_build", `{"operationId":"more","layerId":"`+layer+`","formId":42,"use":["tree"],"ids":["x"],"layout":"grid","nodes":[["x","X"],{"key":"y","title":"Y","hole":true},{"key":"z","title":"Z","color":"#112233","desc":"d","data":{"a":1}}],"edges":[{"from":"n599","to":"x","name":"then","type":"flow"},["x","y","next"]]}`)
	if failed || strings.Contains(reply, `"y":"actor-`) || !strings.Contains(reply, `"ids":{"x":"actor-`) || !strings.Contains(reply, `"edges":{"total":2,"done":2}`) {
		t.Fatal(reply)
	}
	cellsOf(t, f, layer, 603, 601)

	dry, failed := call("graph_build_delete", `{"layerId":"`+layer+`","operationId":"tree","dryRun":true}`)
	if failed || dry != `{"found":600,"deleted":0,"dryRun":true}` {
		t.Fatal(dry)
	}
	if gone, failed := call("graph_build_delete", `{"layerId":"`+layer+`","operationId":"tree"}`); failed || gone != `{"found":600,"deleted":600}` {
		t.Fatal(gone)
	}
	cellsOf(t, f, layer, 3, 1)
}

func TestGraphBuildToolFlowchartAndGiven(t *testing.T) {
	call, f := fakeConsole(t)
	reply, failed := call("graph_build", `{"operationId":"flow","layerId":"L","kind":"process","origin":"C, 3","nodes":[{"key":"a","title":"Start"},{"key":"b","title":"Check","data":{"note":"n"}},{"key":"c","title":"Fix"},{"key":"d","title":"Plain","formId":42}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"c"},{"from":"c","to":"b","name":"again"},{"from":"b","to":"d"}]}`)
	if failed || !strings.Contains(reply, `"done":true`) || !strings.Contains(reply, `"area":"C, 3 - `) {
		t.Fatal(reply)
	}
	_, nodes, drawn := f.Snapshot("L")
	blocks := 0
	for _, n := range nodes {
		if len(n.AreaPicture) > 0 {
			blocks++
			// A block's corner sits on a grid line (25 + 50k) and its shape is stored.
			if x := n.X - 100 - 25; x != float64(int(x/50))*50 || !strings.Contains(string(n.LayerSettings), `"blockId":"process"`) {
				t.Fatalf("block at %v,%v with %s", n.X, n.Y, n.LayerSettings)
			}
		}
	}
	if len(nodes) != 4 || blocks != 3 || len(drawn) != 4 {
		t.Fatalf("%d nodes, %d blocks, %d edges", len(nodes), blocks, len(drawn))
	}

	if reply, failed = call("graph_build", `{"operationId":"given","layerId":"G","formId":42,"layout":"given","nodes":[["a","A","C, 7"],{"key":"b","title":"B","cell":"E, 7"}],"edges":[["a","b"]]}`); failed || !strings.Contains(reply, `"area":"C, 7 - E, 7"`) {
		t.Fatal(reply)
	}
	if at := cellsOf(t, f, "G", 2, 1); !at[[2]float64{100, 300}] || !at[[2]float64{200, 300}] {
		t.Fatal(at)
	}
	for want, args := range map[string]string{
		"only with layout given":       `{"operationId":"e1","layerId":"G","formId":42,"nodes":[{"key":"a","title":"A","cell":"C, 9"}]}`,
		"needs every node's cell":      `{"operationId":"e2","layerId":"G","formId":42,"layout":"given","nodes":[{"key":"a","title":"A"}]}`,
		"formId or kind required":      `{"operationId":"e3","layerId":"G","nodes":[{"key":"a","title":"A"}]}`,
		"not in the workspace catalog": `{"operationId":"e4","layerId":"G","nodes":[{"key":"a","title":"A","kind":"nope"}]}`,
		"origin":                       `{"operationId":"e5","layerId":"G","formId":42,"origin":"C7","nodes":[{"key":"a","title":"A"}]}`,
		"occupied":                     `{"operationId":"e6","layerId":"G","formId":42,"origin":"C, 7","nodes":[{"key":"a","title":"A"}]}`,
		"layerId of an existing layer": `{"operationId":"e7","formId":42,"nodes":[{"key":"a","title":"A"}]}`,
		"/nodes/0/data":                `{"operationId":"e8","layerId":"G","kind":"process","nodes":[{"key":"a","title":"A","data":[1]}]}`,
		"/nodes: ":                     buildArgs(`"operationId":"e9","layerId":"G"`, 2001),
	} {
		if reply, failed = call("graph_build", args); !failed || !strings.Contains(reply, want) {
			t.Errorf("%s: %s", want, reply)
		}
	}
	cellsOf(t, f, "G", 2, 1)
	if n := f.Count("actor/create"); n != 0 { // a refused call made no layer either
		t.Fatalf("%d single actor/create calls", n)
	}
}

func TestGraphBuildAnnotations(t *testing.T) {
	tools := consoleTools(t, "console").ListTools()
	build, remove := tools["graph_build"], tools["graph_build_delete"]
	if build == nil || remove == nil {
		t.Fatal("the plain console catalog lacks graph_build or its removal")
	}
	if a := build.Tool.Annotations; *a.ReadOnlyHint || !*a.IdempotentHint || *a.DestructiveHint {
		t.Fatalf("graph_build %+v", a)
	}
	if raw, _ := json.Marshal(remove.Tool); len(raw) > 800 {
		t.Fatalf("graph_build_delete takes %d bytes of the catalog", len(raw))
	}
	if a := remove.Tool.Annotations; *a.ReadOnlyHint || !*a.DestructiveHint || !strings.Contains(remove.Tool.Description, "Destructive: run dryRun") {
		t.Fatalf("graph_build_delete %+v", a)
	}
	// The schema rides in every turn: keep graph_build's whole entry small.
	if raw, _ := json.Marshal(build.Tool); len(raw) > 2900 {
		t.Fatalf("graph_build takes %d bytes of the catalog", len(raw))
	}
}

func TestFlowchartLayoutOriginAndWrap(t *testing.T) {
	send, _ := consoleServer(t, nil)
	var nodes, edges []string
	for i := range 25 {
		nodes = append(nodes, fmt.Sprintf(`{"key":"k%d","blockId":"process"}`, i))
		if i > 0 {
			edges = append(edges, fmt.Sprintf(`{"source":"k0","target":"k%d"}`, i))
		}
	}
	reply := send(`"method":"tools/call","params":{"name":"flowchart_layout","arguments":{"origin":"C, 3","nodes":[` + strings.Join(nodes, ",") + `],"edges":[` + strings.Join(edges, ",") + `]}}`)
	var out struct {
		Result struct {
			Structured struct {
				Nodes []struct {
					Key      string               `json:"key"`
					Position publicapi.GraphPoint `json:"position"`
				} `json:"nodes"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(reply), &out); err != nil || len(out.Result.Structured.Nodes) != 25 {
		t.Fatal(reply, err)
	}
	rows, left, top := map[float64]int{}, 1e9, 1e9
	for _, n := range out.Result.Structured.Nodes {
		rows[n.Position.Y]++
		left, top = min(left, n.Position.X-100), min(top, n.Position.Y-50)
	}
	// 24 children of 350 pitch wrap at 60 cells into three rows; the box starts at C, 3's corner.
	if len(rows) != 4 || left != 75 || top != 75 {
		t.Fatalf("rows %v, corner %v,%v", rows, left, top)
	}
	if bad := callTool(send, "flowchart_layout", `{"origin":"C7","nodes":[{"key":"a","blockId":"process"}]}`); !strings.Contains(bad, `"isError":true`) {
		t.Fatal(bad)
	}
}

// A layout of more than 500 moves goes in several calls, none onto a cell still held.
func TestLayerCompactMovesALargeLayer(t *testing.T) {
	call, f := fakeConsole(t)
	if reply, failed := call("graph_build", buildArgs(`"operationId":"wide","layerId":"L","layout":"grid","rowCells":400`, 1300)); failed || !strings.Contains(reply, `"done":true`) {
		t.Fatal(reply)
	}
	reply, failed := call("layer_compact", `{"layerId":"L"}`)
	if failed || !strings.Contains(reply, `"placementsMoved":`) {
		t.Fatal(reply)
	}
	calls := 0
	for _, c := range f.Calls() {
		if c.Method == "graph/layer/actors/positions/update" {
			calls++
			if c.Items > 500 {
				t.Fatalf("a call moves %d placements", c.Items)
			}
		}
	}
	if calls < 3 {
		t.Fatalf("%d position calls: %s", calls, reply)
	}
	cellsOf(t, f, "L", 1300, 1299)
}

// Flowchart nodes placed without a colour read their actors' colours 100 a batch.
func TestPlacementsReadColoursInBatches(t *testing.T) {
	call, f := fakeConsole(t)
	keys := make([]string, 120)
	for i := range keys {
		keys[i] = fmt.Sprintf(`"n%d"`, i)
	}
	reply, failed := call("graph_build", buildArgs(`"operationId":"src","layerId":"A","ids":[`+strings.Join(keys, ",")+`]`, 120))
	if failed {
		t.Fatal(reply)
	}
	var built struct {
		IDs map[string]string `json:"ids"`
	}
	_ = json.Unmarshal([]byte(reply), &built)
	var nodes []string
	for i := range 120 {
		nodes = append(nodes, fmt.Sprintf(`{"actorId":%q,"position":{"x":%d,"y":%d},"layerSettings":{"width":200,"height":100,"blockId":"process","shape":{"type":"path","d":"M0 0H1"},"textFrame":{"x":0,"y":0,"w":200,"h":100}}}`,
			built.IDs[fmt.Sprintf("n%d", i)], (i%10)*400, (i/10)*300))
	}
	before := f.Count("batch")
	if reply, failed = call("layer_placements_create", `{"layerId":"B","nodes":[`+strings.Join(nodes, ",")+`]}`); failed {
		t.Fatal(reply)
	}
	if f.Count("batch")-before != 2 {
		t.Fatalf("%d colour batches", f.Count("batch")-before)
	}
	cellsOf(t, f, "B", 120, 0)
}

// A bad origin or cell is refused before newLayer is created: no empty layer is left behind.
func TestGraphBuildBadCellsLeaveNoLayer(t *testing.T) {
	call, f := fakeConsole(t)
	for _, args := range []string{
		`{"operationId":"o1","newLayer":"Org","formId":42,"origin":"nowhere","nodes":[["a","A"]]}`,
		`{"operationId":"o2","newLayer":"Org","formId":42,"layout":"given","nodes":[["a","A"]]}`,
		`{"operationId":"o3","newLayer":"Org","formId":42,"nodes":[["a","A","B, 2"]]}`,
	} {
		if text, failed := call("graph_build", args); !failed {
			t.Fatal(args, text)
		}
	}
	if actors, _, _ := f.Snapshot(""); len(actors) != 0 || f.Count("actor/create") != 0 {
		t.Fatalf("%d actors left", len(actors))
	}
}

// A layer built in sections, each a parent over 25 children and no origin: compact blocks that fill
// rows, a page and not a strip.
func TestGraphBuildToolFlowsSections(t *testing.T) {
	call, f := fakeConsole(t)
	for i := range 12 {
		nodes, edges := []string{fmt.Sprintf(`["p%d","Group %d"]`, i, i)}, []string{}
		for k := range 25 {
			nodes = append(nodes, fmt.Sprintf(`["p%dc%d","Item %d"]`, i, k, k))
			edges = append(edges, fmt.Sprintf(`["p%d","p%dc%d"]`, i, i, k))
		}
		reply, failed := call("graph_build", fmt.Sprintf(`{"operationId":"sec%d","layerId":"L","formId":42,"nodes":[%s],"edges":[%s]}`, i, strings.Join(nodes, ","), strings.Join(edges, ",")))
		if failed || !strings.Contains(reply, `"done":true`) || strings.Contains(reply, `"next"`) || len(reply) > 120 {
			t.Fatal(reply)
		}
		if i == 0 && !strings.Contains(reply, `"area":"A, 1 - M, 16"`) { // 5 by 5 under the parent
			t.Fatal(reply)
		}
	}
	cells := cellsOf(t, f, "L", 12*26, 12*25)
	right, bottom := 0.0, 0.0
	for at := range cells {
		right, bottom = max(right, at[0]), max(bottom, at[1])
	}
	// Five sections a row, three rows: 77 by 54 cells.
	if right != 76*50 || bottom != 53*50 {
		t.Fatalf("the layer is %v by %v cells", right/50+1, bottom/50+1)
	}
}

// rowsOf counts the layer's nodes by row, top to bottom.
func rowsOf(f *simfake.Server, layer string) []int {
	_, placed, _ := f.Snapshot(layer)
	count := map[float64]int{}
	var ys []float64
	for _, n := range placed {
		if count[n.Y] == 0 {
			ys = append(ys, n.Y)
		}
		count[n.Y]++
	}
	sort.Float64s(ys)
	out := make([]int, len(ys))
	for i, y := range ys {
		out[i] = count[y]
	}
	return out
}

// auto keeps whole ranks for flowchart blocks and groups plain actors under their parents; layered
// and flowchart ask for one or the other whatever the nodes are.
func TestGraphBuildToolLayoutModes(t *testing.T) {
	// Two parents of eight children each: whole ranks put the sixteen in one row, blocks fold each
	// eight into two rows of four under its parent.
	nodes, edges := []string{`["r","R"]`, `["a","A"]`, `["b","B"]`}, []string{`["r","a"]`, `["r","b"]`}
	for i := range 16 {
		nodes = append(nodes, fmt.Sprintf(`["c%d","C"]`, i))
		edges = append(edges, fmt.Sprintf(`["%s","c%d"]`, []string{"a", "b"}[i%2], i))
	}
	ranks, grouped := []int{1, 2, 16}, []int{1, 2, 8, 8}
	for name, want := range map[string]struct {
		head string
		rows []int
	}{
		"plain auto":       {`"formId":42`, grouped},
		"plain layered":    {`"formId":42,"layout":"layered"`, grouped},
		"plain flowchart":  {`"formId":42,"layout":"flowchart"`, ranks},
		"blocks auto":      {`"kind":"process"`, ranks},
		"blocks flowchart": {`"kind":"process","layout":"flowchart"`, ranks},
		"blocks layered":   {`"kind":"process","layout":"layered"`, grouped},
	} {
		call, f := fakeConsole(t)
		reply, failed := call("graph_build", fmt.Sprintf(`{"operationId":"m","layerId":"L","rowCells":200,%s,"nodes":[%s],"edges":[%s]}`, want.head, strings.Join(nodes, ","), strings.Join(edges, ",")))
		if failed || !strings.Contains(reply, `"done":true`) {
			t.Fatal(name, reply)
		}
		if got := rowsOf(f, "L"); fmt.Sprint(got) != fmt.Sprint(want.rows) {
			t.Errorf("%s: rows %v, want %v", name, got, want.rows)
		}
	}
}

// A build without origin that lost an answer half way is finished by the same call where it began:
// the repeat takes its place from what landed, not from the layer's next free place, which the
// landed part has moved.
func TestGraphBuildToolResumesAtItsDefaultOrigin(t *testing.T) {
	args := buildArgs(`"operationId":"part","layerId":"L"`, 300)
	first := `{"operationId":"first","layerId":"L","formId":42,"nodes":[["s0","S"],["s1","S"],["s2","S"]],"edges":[["s0","s1"],["s0","s2"]]}`
	call, f := fakeConsole(t)
	for _, in := range []string{first, args} {
		if reply, failed := call("graph_build", in); failed || !strings.Contains(reply, `"done":true`) {
			t.Fatal(reply)
		}
	}
	clean := cellsOf(t, f, "L", 303, 301)

	call, f = fakeConsole(t)
	if reply, failed := call("graph_build", first); failed {
		t.Fatal(reply)
	}
	batches := 0
	f.Set(func(s *simfake.Server) {
		s.Hook = func(_ int, method string) simfake.Fault {
			if method == "batch" {
				if batches++; batches == 2 {
					return simfake.Lose
				}
			}
			return simfake.Answer
		}
	})
	if reply, failed := call("graph_build", args); failed || !strings.Contains(reply, `"done":false`) {
		t.Fatal(reply)
	}
	f.Set(func(s *simfake.Server) { s.Hook = nil })
	if reply, failed := call("graph_build", args); failed || !strings.Contains(reply, `"done":true`) {
		t.Fatal(reply)
	}
	again := cellsOf(t, f, "L", 303, 301)
	for at := range clean {
		if !again[at] {
			t.Fatalf("the repeated build has no node at %v", at)
		}
	}
}
