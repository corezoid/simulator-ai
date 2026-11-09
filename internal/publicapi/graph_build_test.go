package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/corezoid/simulator-ai/internal/simfake"
)

func fakeClient(t *testing.T) (*Client, *simfake.Server) {
	t.Helper()
	f := simfake.New()
	t.Cleanup(f.Close)
	c, err := New(Config{Origin: f.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// gridNodes are n plain nodes on a 40-wide grid, three cells apart, from the corner at.
func gridNodes(prefix string, n int, at GraphPoint) []BuildNode {
	nodes := make([]BuildNode, n)
	for i := range nodes {
		nodes[i] = BuildNode{Key: fmt.Sprintf("%s%d", prefix, i), FormID: 42, Title: fmt.Sprintf("Node %d", i),
			Position: GraphPoint{X: at.X + gridCell/2 + float64(i%40)*150, Y: at.Y + gridCell/2 + float64(i/40)*150}}
	}
	return nodes
}

// meshEdges chain the nodes and add a skip link from every even one.
func meshEdges(prefix string, n int) []BuildEdge {
	var edges []BuildEdge
	for i := 0; i+1 < n; i++ {
		edges = append(edges, BuildEdge{From: fmt.Sprintf("%s%d", prefix, i), To: fmt.Sprintf("%s%d", prefix, i+1), Name: "next"})
		if i%2 == 0 && i+2 < n {
			edges = append(edges, BuildEdge{From: fmt.Sprintf("%s%d", prefix, i), To: fmt.Sprintf("%s%d", prefix, i+2), EdgeType: "flow"})
		}
	}
	return edges
}

func build(t *testing.T, c *Client, target GraphBuildTarget, in GraphBuildInput) GraphBuildResult {
	t.Helper()
	b, err := c.OpenGraphBuild(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	out, err := b.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// assertLayer checks what really landed: so many nodes, each on its own cell and of its own actor,
// so many drawn edges, each once.
func assertLayer(t *testing.T, f *simfake.Server, layer string, nodes, edges int) {
	t.Helper()
	_, placed, drawn := f.Snapshot(layer)
	if len(placed) != nodes || len(drawn) != edges {
		t.Fatalf("layer has %d nodes and %d edges, want %d and %d", len(placed), len(drawn), nodes, edges)
	}
	cells, actors := map[[2]float64]bool{}, map[string]bool{}
	for _, n := range placed {
		cell := [2]float64{n.X, n.Y}
		if cells[cell] || actors[n.Actor] {
			t.Fatalf("placement %d shares a cell or an actor: %+v", n.LaID, n)
		}
		cells[cell], actors[n.Actor] = true, true
	}
	seen := map[string]bool{}
	for _, e := range drawn {
		if seen[e.Edge] {
			t.Fatalf("edge %s is drawn twice", e.Edge)
		}
		seen[e.Edge] = true
	}
}

func TestGraphBuildScale(t *testing.T) {
	for _, n := range []int{500, 2000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			c, f := fakeClient(t)
			in := GraphBuildInput{Nodes: gridNodes("n", n, GraphPoint{X: -25, Y: -25}), Edges: meshEdges("n", n)}
			in.Nodes[7].Data = json.RawMessage(`{"note":"` + strings.Repeat("x", 300<<10) + `"}`) // one fat actor: the batch is cut by size too
			out := build(t, c, GraphBuildTarget{NewLayer: "Big", OperationID: "big-1"}, in)
			edges := len(in.Edges)
			if !out.Done || out.Nodes != (BuildCount{Total: n, Done: n}) || out.Edges != (BuildCount{Total: edges, Done: edges}) || out.Error != "" || out.IDs != nil {
				t.Fatalf("%+v", out)
			}
			assertLayer(t, f, out.LayerID, n, edges)
			actors, _, _ := f.Snapshot(out.LayerID)
			recent := 0
			for _, a := range actors {
				if a.Recent {
					recent++
				}
			}
			if len(actors) != n+1 || recent != 1 { // the layer itself is the only actor in the recent list
				t.Fatalf("%d actors, %d in the recent list", len(actors), recent)
			}
			batches, other := 0, 0
			for _, call := range f.Calls() {
				switch call.Method {
				case "batch":
					batches++
					if call.Items > simfake.MaxBatchItems || call.Bytes > buildChunkBytes+(64<<10) {
						t.Fatalf("batch of %d items and %d bytes", call.Items, call.Bytes)
					}
				default:
					other++
				}
			}
			// One batch per 99 nodes and per 99 edges, and a constant handful of reads around them: the
			// layer's template, its by-ref lookup and creation, two edge types.
			want := (n+98)/99 + (edges+98)/99
			if batches < want || batches > want+1 || other != 5 {
				t.Fatalf("%d batches (want %d or one more for the fat actor), %d other calls: %v", batches, want, other, f.Calls()[:6])
			}
			raw, _ := json.Marshal(out)
			if len(raw) > 200 { // counts only: no ids, no chunks, no echo
				t.Fatalf("the answer of a %d-node build takes %d bytes: %s", n, len(raw), raw)
			}
			if out.Area != "A, 1 - N4, "+fmt.Sprint((n-1)/40*3+1) || out.Next == nil || out.Next.Right != "R4, 1" {
				t.Fatalf("area %s next %+v", out.Area, out.Next)
			}
		})
	}
}

func TestGraphBuildSmallAnswerHasIDs(t *testing.T) {
	c, f := fakeClient(t)
	out := build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "s"}, GraphBuildInput{Nodes: gridNodes("n", 3, GraphPoint{X: -25, Y: -25}), Edges: meshEdges("n", 3), IDs: []string{"n0", "nobody"}})
	if !out.Done || len(out.IDs) != 1 || out.IDs["n0"] == "" || out.Chunks != nil {
		t.Fatalf("%+v", out)
	}
	assertLayer(t, f, "L", 3, 3)
	// The wire of the node batch: actors with their refs and out of the recent list, placed by reference.
	if f.Count("batch") != 2 || f.Count("graph/layer/actors/list") != 1 {
		t.Fatal(f.Calls())
	}
	raw, _ := json.Marshal(out)
	if len(raw) > 220 {
		t.Fatalf("%d bytes: %s", len(raw), raw)
	}
}

// A lost answer, then a cut connection: the same call repeated builds the rest and nothing twice.
func TestGraphBuildResumes(t *testing.T) {
	c, f := fakeClient(t)
	const n = 500
	in := GraphBuildInput{Nodes: gridNodes("n", n, GraphPoint{X: -25, Y: -25}), Edges: meshEdges("n", n)}
	target := GraphBuildTarget{NewLayer: "Big", OperationID: "retry"}

	batches := 0
	f.Set(func(s *simfake.Server) {
		s.Hook = func(_ int, method string) simfake.Fault {
			if method == "batch" {
				if batches++; batches == 3 {
					return simfake.Lose // the third node batch lands, its answer does not
				}
			}
			return simfake.Answer
		}
	})
	first := build(t, c, target, in)
	if first.Done || first.Nodes.Done != 198 || !strings.Contains(first.Resume, "same operationId") || len(first.Chunks) != 4 ||
		first.Chunks[1] != (BuildChunks{Kind: "nodes", Chunks: 1, Items: 99, Status: "unknown"}) || first.Chunks[2].Status != "pending" || first.Chunks[3] != (BuildChunks{Kind: "edges", Chunks: 8, Items: len(in.Edges), Status: "pending"}) {
		t.Fatalf("%+v", first)
	}

	f.Set(func(s *simfake.Server) {
		batches = 0
		s.Hook = func(_ int, method string) simfake.Fault {
			if method == "batch" {
				if batches++; batches == 5 {
					return simfake.Drop // the second edge batch never arrives
				}
			}
			return simfake.Answer
		}
	})
	second := build(t, c, target, in)
	if second.Done || second.LayerID != first.LayerID || second.Nodes != (BuildCount{Total: n, Done: 203, Existing: 297}) || second.Edges.Done != 99 {
		t.Fatalf("%+v", second)
	}

	f.Set(func(s *simfake.Server) { s.Hook = nil })
	third := build(t, c, target, in)
	if !third.Done || third.Nodes != (BuildCount{Total: n, Existing: n}) || third.Edges != (BuildCount{Total: len(in.Edges), Done: len(in.Edges) - 99, Existing: 99}) {
		t.Fatalf("%+v", third)
	}
	assertLayer(t, f, third.LayerID, n, len(in.Edges))
	if actors, _, _ := f.Snapshot(third.LayerID); len(actors) != n+1 {
		t.Fatalf("%d actors after two retries", len(actors))
	}
	// Once more changes nothing.
	again := build(t, c, target, in)
	if !again.Done || again.Nodes.Done != 0 || again.Edges.Done != 0 || again.Edges.Existing != len(in.Edges) {
		t.Fatalf("%+v", again)
	}
	assertLayer(t, f, third.LayerID, n, len(in.Edges))
}

func TestGraphBuildSectionsJoin(t *testing.T) {
	c, f := fakeClient(t)
	a := build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "sec-a"}, GraphBuildInput{Nodes: gridNodes("a", 120, GraphPoint{X: -25, Y: -25}), Edges: meshEdges("a", 120), IDs: []string{"a5"}})
	if !a.Done || len(a.IDs) != 1 || a.Next.Below != "A, 11" {
		t.Fatalf("%+v", a)
	}
	b, err := c.OpenGraphBuild(context.Background(), GraphBuildTarget{LayerID: "L", OperationID: "sec-b"})
	if err != nil {
		t.Fatal(err)
	}
	free := b.FreeOrigin()
	if at, ok := b.Landed("a0"); ok || free != (GraphPoint{X: -25, Y: 475}) {
		t.Fatal(free, at, ok)
	}
	in := GraphBuildInput{Nodes: gridNodes("b", 30, free), Use: []string{"sec-a"}, Existing: map[string]string{"hub": a.IDs["a5"]},
		Edges: append(meshEdges("b", 30), BuildEdge{From: "a119", To: "b0"}, BuildEdge{From: "hub", To: "b1"})}
	out, err := b.Run(context.Background(), in)
	if err != nil || !out.Done || out.Edges.Done != len(in.Edges) || out.Area != "A, 11 - J3, 11" {
		t.Fatal(out, err)
	}
	assertLayer(t, f, "L", 150, len(meshEdges("a", 120))+len(in.Edges))

	for name, bad := range map[string]GraphBuildInput{
		"is not placed on the layer": {Existing: map[string]string{"x": "nobody"}, Edges: []BuildEdge{{From: "x", To: "a1"}}},
		"no node of this call":       {Edges: []BuildEdge{{From: "a1", To: "zzz"}}, Use: []string{"sec-a"}},
		"occupied":                   {Nodes: gridNodes("c", 2, GraphPoint{X: -25, Y: -25})},
		"overlap":                    {Nodes: append(gridNodes("c", 1, GraphPoint{X: 5000, Y: 5000}), gridNodes("d", 1, GraphPoint{X: 5000, Y: 5000})...)},
		"unique keys":                {Nodes: append(gridNodes("c", 1, GraphPoint{X: 5000, Y: 5000}), gridNodes("c", 1, GraphPoint{X: 6000, Y: 5000})...)},
		"two different keys":         {Edges: []BuildEdge{{From: "a1", To: "a1"}}},
		"edge type":                  {Edges: []BuildEdge{{From: "a1", To: "a3", EdgeType: "nope"}}, Use: []string{"sec-a"}},
		"use \"bad op\"":             {Edges: []BuildEdge{{From: "a1", To: "a3"}}, Use: []string{"bad op"}},
		"1..2000 nodes":              {},
		"formId":                     {Nodes: []BuildNode{{Key: "k", Title: "T"}}},
		"data must be an object":     {Nodes: []BuildNode{{Key: "k", Title: "T", FormID: 1, Data: json.RawMessage(`[1]`)}}},
		"position out of range":      {Nodes: []BuildNode{{Key: "k", Title: "T", FormID: 1, Position: GraphPoint{X: 1e12}}}},
	} {
		s, err := c.OpenGraphBuild(context.Background(), GraphBuildTarget{LayerID: "L", OperationID: "sec-c"})
		if err != nil {
			t.Fatal(err)
		}
		before := len(f.Calls())
		if _, err = s.Run(context.Background(), bad); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
		for _, call := range f.Calls()[before:] {
			if call.Method == "batch" || call.Method == "graph/layer/actors/manage" {
				t.Errorf("%s: wrote before refusing", name)
			}
		}
	}
	assertLayer(t, f, "L", 150, len(meshEdges("a", 120))+len(in.Edges))
}

func TestGraphBuildRefusedBatchAndBudget(t *testing.T) {
	c, f := fakeClient(t)
	f.Set(func(s *simfake.Server) { s.BadForm = 13 })
	in := GraphBuildInput{Nodes: gridNodes("n", 150, GraphPoint{X: -25, Y: -25}), Edges: meshEdges("n", 150)}
	in.Nodes[120].FormID = 13
	out := build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "bad"}, in)
	if out.Done || out.Nodes.Done != 99 || !strings.Contains(out.Error, `node "n120"`) || len(out.Error) > 200 || len(out.Resume) > 100 || out.Chunks[1].Status != "failed" || out.Chunks[2].Status != "pending" {
		t.Fatalf("%+v", out)
	}
	assertLayer(t, f, "L", 99, 0)
	in.Nodes[120].FormID = 42 // fixed: the same operation continues
	if out = build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "bad"}, in); !out.Done || out.Nodes != (BuildCount{Total: 150, Done: 51, Existing: 99}) {
		t.Fatalf("%+v", out)
	}
	assertLayer(t, f, "L", 150, len(in.Edges))

	defer func(d time.Duration) { buildBudget = d }(buildBudget)
	buildBudget = -1
	late := build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "late"}, GraphBuildInput{Nodes: gridNodes("m", 5, GraphPoint{X: -25, Y: 5000})})
	if late.Done || late.Nodes.Done != 0 || late.Chunks[0].Status != "pending" || !strings.Contains(late.Resume, "time budget") {
		t.Fatalf("%+v", late)
	}

	for _, bad := range []GraphBuildTarget{{LayerID: "L", OperationID: "has space"}, {OperationID: "x"}, {LayerID: "L", NewLayer: "N", OperationID: "x"}} {
		if _, err := c.OpenGraphBuild(context.Background(), bad); err == nil {
			t.Error("accepted", bad)
		}
	}
	c.config.Profile = "read"
	if _, err := c.OpenGraphBuild(context.Background(), GraphBuildTarget{LayerID: "L", OperationID: "x"}); err == nil {
		t.Fatal("built outside the console profile")
	}
	if _, err := c.DeleteGraphBuild(context.Background(), GraphBuildDeleteInput{LayerID: "L", OperationID: "x"}); err == nil {
		t.Fatal("deleted outside the console profile")
	}
}

func TestGraphBuildDelete(t *testing.T) {
	c, f := fakeClient(t)
	in := GraphBuildInput{Nodes: gridNodes("n", 230, GraphPoint{X: -25, Y: -25}), Edges: meshEdges("n", 230)}
	build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "gone"}, in)
	build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "stays"}, GraphBuildInput{Nodes: gridNodes("k", 4, GraphPoint{X: -25, Y: 5000})})
	dry, err := c.DeleteGraphBuild(context.Background(), GraphBuildDeleteInput{LayerID: "L", OperationID: "gone", DryRun: true})
	if err != nil || dry.Found != 230 || !dry.DryRun || dry.Deleted != 0 {
		t.Fatal(dry, err)
	}
	assertLayer(t, f, "L", 234, len(in.Edges))
	done, err := c.DeleteGraphBuild(context.Background(), GraphBuildDeleteInput{LayerID: "L", OperationID: "gone"})
	if err != nil || done.Found != 230 || done.Deleted != 230 || len(done.Errors) != 0 {
		t.Fatal(done, err)
	}
	assertLayer(t, f, "L", 4, 0)
	// The refs stay in the trash: the operation cannot be built again, and says so.
	out := build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "gone"}, in)
	if out.Done || !strings.Contains(out.Error, "use a new operationId") {
		t.Fatalf("%+v", out)
	}
	if _, err = c.DeleteGraphBuild(context.Background(), GraphBuildDeleteInput{LayerID: "L", OperationID: "no good"}); err == nil {
		t.Fatal("accepted a bad operationId")
	}
}

func TestCreateActorsRepeatsUnderOperationID(t *testing.T) {
	c, f := fakeClient(t)
	in := ActorsCreateInput{OperationID: "op1", Actors: []ActorCreateInput{{FormID: 5, Title: "A"}, {FormID: 6, Title: "B"}, {FormID: 5, Title: "C"}}}
	first, err := c.CreateActors(context.Background(), in)
	if err != nil || len(first.Actors) != 3 || *first.Actors[1].Ref != "gb:op1:1" {
		t.Fatal(first, err)
	}
	again, err := c.CreateActors(context.Background(), in)
	if err != nil || *again.Actors[0].ID != *first.Actors[0].ID || *again.Actors[2].ID != *first.Actors[2].ID {
		t.Fatal(again, err)
	}
	actors, _, _ := f.Snapshot("")
	if len(actors) != 3 || actors[0].Recent {
		t.Fatalf("%d actors; recent %v", len(actors), actors[0].Recent)
	}
	// Without an operationId a call creates anew, as before, and sends no lookup.
	before := f.Count("actor/by-ref/get")
	if _, err = c.CreateActors(context.Background(), ActorsCreateInput{Actors: in.Actors[:1]}); err != nil || f.Count("actor/by-ref/get") != before {
		t.Fatal(err)
	}
	// The same id with more actors is another operation: refused, nothing created.
	in.Actors = append(in.Actors, ActorCreateInput{FormID: 5, Title: "D"})
	if _, err = c.CreateActors(context.Background(), in); err == nil || !strings.Contains(err.Error(), "use a new operationId") {
		t.Fatal(err)
	}
	if _, err = c.CreateActors(context.Background(), ActorsCreateInput{OperationID: "no good", Actors: in.Actors}); err == nil {
		t.Fatal("accepted a bad operationId")
	}
	if actors, _, _ = f.Snapshot(""); len(actors) != 4 {
		t.Fatalf("%d actors", len(actors))
	}
}

// A page that comes back short is not the last one, and pages stay at 200 rows.
func TestLayerRoomReadsEveryPage(t *testing.T) {
	c, f := fakeClient(t)
	build(t, c, GraphBuildTarget{LayerID: "L", OperationID: "room"}, GraphBuildInput{Nodes: gridNodes("n", 450, GraphPoint{X: -25, Y: -25})})
	f.Set(func(s *simfake.Server) { s.Ghosts = 130 })
	before := len(f.Calls())
	room, err := c.readLayerRoom(context.Background(), "L")
	if err != nil || len(room.Occupants) != 450 || room.Partial {
		t.Fatal(len(room.Occupants), room.Partial, err)
	}
	if n := f.Count("graph/layer/actors/list"); len(f.Calls())-before != 5 || n < 4 { // anisotropy, three pages, the empty one
		t.Fatal(f.Calls()[before:])
	}
}

func TestLayerWritesTakeFiveHundred(t *testing.T) {
	c, f := fakeClient(t)
	keys := make([]string, 501)
	for i := range keys {
		keys[i] = fmt.Sprintf("n%d", i)
	}
	out := build(t, c, GraphBuildTarget{LayerID: "src", OperationID: "many"}, GraphBuildInput{Nodes: gridNodes("n", 501, GraphPoint{X: -25, Y: -25}), IDs: keys})
	in := PlacementsCreateInput{LayerID: "L2"}
	for i := range 501 {
		in.Nodes = append(in.Nodes, NodePlacement{ActorID: out.IDs[fmt.Sprintf("n%d", i)], Position: GraphPoint{X: float64(i%40) * 100, Y: float64(i/40) * 100}})
	}
	if _, err := c.CreatePlacements(context.Background(), in); err == nil {
		t.Fatal("501 placements accepted")
	}
	in.Nodes = in.Nodes[:500]
	placed, err := c.CreatePlacements(context.Background(), in)
	if err != nil || len(placed.Nodes) != 500 {
		t.Fatal(len(placed.Nodes), err)
	}
	assertLayer(t, f, "L2", 500, 0)
	moves := PositionsUpdateInput{LayerID: "L2"}
	for _, n := range placed.Nodes {
		moves.Items = append(moves.Items, PositionUpdate{LaID: n.LaID, X: n.Position.X + 10000, Y: n.Position.Y})
	}
	if moved, err := c.UpdatePositions(context.Background(), moves); err != nil || moved.Updated != 500 {
		t.Fatal(moved, err)
	}
	moves.Items = append(moves.Items, PositionUpdate{LaID: 1, X: 1, Y: 1})
	if _, err := c.UpdatePositions(context.Background(), moves); err == nil {
		t.Fatal("501 moves accepted")
	}
}
