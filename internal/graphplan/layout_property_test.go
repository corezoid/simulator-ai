package graphplan

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// randomGraph is a seeded graph of mixed footprints with every awkward link: shared children, links
// that skip ranks, cycles, self-loops, duplicates and nodes with no link at all.
func randomGraph(r *rand.Rand, n int) ([]LayoutNode, []Edge) {
	sizes := [][2]float64{{PlainSize, PlainSize}, {PlainSize, PlainSize}, {150, 100}, {200, 50}, {100, 150}}
	mixed := r.Intn(2) == 0
	nodes := make([]LayoutNode, n)
	for i := range nodes {
		size := sizes[0]
		if mixed {
			size = sizes[r.Intn(len(sizes))]
		}
		nodes[i] = LayoutNode{Key: fmt.Sprint("n", i), Width: size[0], Height: size[1]}
	}
	var edges []Edge
	link := func(from, to int) {
		edges = append(edges, Edge{Key: fmt.Sprint("e", len(edges)), Source: nodes[from].Key, Target: nodes[to].Key})
	}
	fanout := 1 + r.Intn(40)
	for i := 1; i < n; i++ {
		switch r.Intn(12) {
		case 0: // isolated
		case 1:
			link(i, r.Intn(n)) // any direction: cycles and self-loops
		case 2:
			link(r.Intn(i), i)
			link(r.Intn(i), i)
			link(r.Intn(i), i)
		case 3:
			p := r.Intn(i)
			link(p, i)
			link(p, i)
		default:
			link(max(0, i-1-r.Intn(fanout)), i)
		}
	}
	return nodes, edges
}

// assertGapped fails when two boxes are closer than the gap on both axes (so also when they overlap).
func assertGapped(t *testing.T, name string, nodes []LayoutNode, at []LayoutBox, gap float64) {
	t.Helper()
	type cell [2]int
	reach := cells(gap)
	grid := map[cell][]int{}
	const bucket = 400
	for i, b := range at {
		for x := int((b.X - reach) / bucket); x <= int((b.X+b.W+reach)/bucket); x++ {
			for y := int((b.Y - reach) / bucket); y <= int((b.Y+b.H+reach)/bucket); y++ {
				for _, j := range grid[cell{x, y}] {
					o := at[j]
					if b.X < o.X+o.W+reach && o.X < b.X+b.W+reach && b.Y < o.Y+o.H+reach && o.Y < b.Y+b.H+reach {
						t.Fatalf("%s: %s %+v and %s %+v are closer than %v", name, nodes[i].Key, b, nodes[j].Key, o, reach)
					}
				}
				grid[cell{x, y}] = append(grid[cell{x, y}], i)
			}
		}
	}
}

func TestLayeredInvariantsOnRandomGraphs(t *testing.T) {
	r := rand.New(rand.NewSource(20261110))
	for round := range 400 {
		n := 1 + r.Intn(60)
		if round%50 == 0 {
			n = 1500
		}
		nodes, edges := randomGraph(r, n)
		opt := LayoutOptions{RowWidth: float64(4+r.Intn(100)) * gridCell, RankRows: r.Intn(3) == 0, Centered: r.Intn(4) == 0,
			Origin: publicapi.GraphPoint{X: float64(r.Intn(4000) - 2000), Y: float64(r.Intn(4000) - 2000)}}
		if r.Intn(2) == 0 {
			opt.Gap = PlainGap
		}
		name := fmt.Sprintf("round %d (%d nodes, %+v)", round, n, opt)
		at, returns, box := Layered(nodes, edges, opt)
		if len(at) != n {
			t.Fatalf("%s: %d of %d placed", name, len(at), n)
		}
		assertApart(t, nodes, at, box)
		boxes := make([]LayoutBox, n)
		widest := 0.0
		for i, nd := range nodes {
			boxes[i] = LayoutBox{X: at[i].X - nd.Width/2, Y: at[i].Y - nd.Height/2, W: nd.Width, H: nd.Height}
			widest = max(widest, nd.Width)
		}
		assertGapped(t, name, nodes, boxes, opt.gap())
		if box.W > max(opt.rowWidth(), widest) {
			t.Fatalf("%s: box %+v is wider than the row", name, box)
		}
		again, returnsAgain, boxAgain := Layered(nodes, edges, opt)
		if !reflect.DeepEqual(at, again) || !reflect.DeepEqual(returns, returnsAgain) || box != boxAgain {
			t.Fatalf("%s: not deterministic", name)
		}
		// The layout does not depend on where it starts: a repeated graph_build shifts it onto what landed.
		moved := opt
		moved.Origin.X, moved.Origin.Y = opt.Origin.X+350, opt.Origin.Y-1200
		shifted, _, _ := Layered(nodes, edges, moved)
		for i := range at {
			if shifted[i].X-at[i].X != 350 || shifted[i].Y-at[i].Y != -1200 {
				t.Fatalf("%s: %s moved by %v, %v", name, nodes[i].Key, shifted[i].X-at[i].X, shifted[i].Y-at[i].Y)
			}
		}
	}
	if at, returns, box := Layered(nil, nil, LayoutOptions{}); len(at) != 0 || len(returns) != 0 || box.W != 0 || box.H != 0 {
		t.Fatalf("empty: %v %v %+v", at, returns, box)
	}
}

// Blocks that cannot share a row at their near-square width are narrowed a little so a whole number
// of them fills it: one hub more does not halve the blocks in a row.
func TestLayeredFitsWholeBlocksInARow(t *testing.T) {
	for hubs, tall := range map[int]float64{10: 64, 11: 97} {
		nodes, edges := fan(hubs, 40)
		at, _, box := Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
		assertApart(t, nodes, at, box)
		if box.W > DefaultRowWidth || box.W < 50*gridCell || box.H > tall*gridCell {
			t.Fatalf("%d hubs: %v by %v cells", hubs, box.W/gridCell, box.H/gridCell)
		}
	}
}
