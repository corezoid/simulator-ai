package graphplan

import (
	"fmt"
	"math"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

func plainBoxes(n int) []LayoutNode {
	out := make([]LayoutNode, n)
	for i := range out {
		out[i] = LayoutNode{Key: fmt.Sprintf("n%d", i), Width: PlainSize, Height: PlainSize}
	}
	return out
}

// assertApart fails when two boxes overlap or a corner is off the grid lines.
func assertApart(t *testing.T, nodes []LayoutNode, at []publicapi.GraphPoint, box LayoutBox) {
	t.Helper()
	cells := map[[2]int]int{}
	for i, n := range nodes {
		left, top := at[i].X-n.Width/2, at[i].Y-n.Height/2
		if !onLine(left) || !onLine(top) {
			t.Fatalf("%s at %v is off the grid", n.Key, at[i])
		}
		if left < box.X || top < box.Y || left+n.Width > box.X+box.W || top+n.Height > box.Y+box.H {
			t.Fatalf("%s at %v is outside the box %+v", n.Key, at[i], box)
		}
		for x := left; x < left+n.Width; x += gridCell {
			for y := top; y < top+n.Height; y += gridCell {
				cell := [2]int{int(math.Floor(x / gridCell)), int(math.Floor(y / gridCell))}
				if j, taken := cells[cell]; taken {
					t.Fatalf("%s and %s overlap at %v", nodes[j].Key, n.Key, cell)
				}
				cells[cell] = i
			}
		}
	}
}

func TestLayeredWrapsRanksAndStartsAtOrigin(t *testing.T) {
	// One root over 30 children: the second rank is wider than a row and wraps under the root.
	nodes := []LayoutNode{{"root", 200, 100}}
	var edges []Edge
	for i := range 30 {
		key := fmt.Sprintf("c%d", i)
		nodes = append(nodes, LayoutNode{key, 200, 100})
		edges = append(edges, Edge{Key: "root->" + key, Source: "root", Target: key})
	}
	origin := publicapi.GraphPoint{X: 1025, Y: 2025}
	at, returns, box := Layered(nodes, edges, LayoutOptions{Origin: origin})
	assertApart(t, nodes, at, box)
	if len(returns) != 0 || box.X != 1025 || box.Y != 2025 || box.W > DefaultRowWidth {
		t.Fatalf("returns %v box %+v", returns, box)
	}
	rows := map[float64]int{}
	for _, p := range at[1:] {
		rows[p.Y]++
	}
	if len(rows) != 5 || at[0].Y >= at[1].Y { // 30 childless blocks fold into 6 by 5 under the root
		t.Fatalf("rows %v", rows)
	}
	// A second layout from the cell under the first does not touch it.
	more, _, below := Layered(nodes, edges, LayoutOptions{Origin: publicapi.GraphPoint{X: box.X, Y: box.Y + box.H + layoutGap}})
	assertApart(t, append(append([]LayoutNode{}, nodes...), nodes...), append(append([]publicapi.GraphPoint{}, at...), more...), LayoutBox{X: box.X, Y: box.Y, W: box.W, H: below.Y + below.H - box.Y})

	narrow, _, narrowBox := Layered(nodes, edges, LayoutOptions{RowWidth: 700})
	assertApart(t, nodes, narrow, narrowBox)
	if narrowBox.W != 550 {
		t.Fatalf("two blocks a row: %+v", narrowBox)
	}
}

func TestGridAndLayerable(t *testing.T) {
	nodes := plainBoxes(100)
	at, box := Grid(nodes, LayoutOptions{Gap: PlainGap, Origin: publicapi.GraphPoint{X: -25, Y: -25}})
	assertApart(t, nodes, at, box)
	if box != (LayoutBox{X: -25, Y: -25, W: 9*150 + 50, H: 9*150 + 50}) || at[0] != (publicapi.GraphPoint{}) || at[11] != (publicapi.GraphPoint{X: 150, Y: 150}) {
		t.Fatalf("%+v %v %v", box, at[0], at[11])
	}
	// 2500 nodes would be 50 a row; the row width allows 20.
	wide := plainBoxes(2500)
	at, box = Grid(wide, LayoutOptions{Gap: PlainGap})
	assertApart(t, wide, at, box)
	if box.W != 19*150+50 {
		t.Fatalf("%+v", box)
	}
	if at, box = Grid(nil, LayoutOptions{}); len(at) != 0 || box.W != 0 {
		t.Fatal(at, box)
	}

	chain := []Edge{{"a", "n0", "n1", ""}, {"b", "n1", "n2", ""}, {"c", "n2", "n3", ""}, {"d", "n3", "n4", ""}, {"e", "n4", "n0", ""}}
	if !Layerable(nodes[:5], chain) || Layerable(nodes[:5], nil) {
		t.Fatal("a chain with one return link is a direction; no edges are none")
	}
	if ring := []Edge{{"a", "n0", "n1", ""}, {"b", "n1", "n0", ""}}; Layerable(nodes[:2], ring) {
		t.Fatal("a two-node ring has no direction")
	}
}

// A long chain and a wide fan lay out in near-linear time: the cap is not about speed.
func TestLayeredScales(t *testing.T) {
	nodes := plainBoxes(MaxLayoutNodes)
	var edges []Edge
	for i := 1; i < len(nodes); i++ {
		parent := (i - 1) / 3
		if i%2 == 0 {
			parent = i - 1 // a chain through the even ones: deep and wide at once
		}
		edges = append(edges, Edge{Key: fmt.Sprint(i), Source: nodes[parent].Key, Target: nodes[i].Key})
	}
	at, _, box := Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
	assertApart(t, nodes, at, box)
}

func TestMoveBatches(t *testing.T) {
	// 1200 placements in a row shift one cell right: every target but the last is held by a neighbour.
	var nodes []LayerNode
	var moves []Move
	for i := range 1200 {
		nodes = append(nodes, LayerNode{LaID: int64(i + 1), X: float64(i) * 50})
		moves = append(moves, Move{LaID: int64(i + 1), X: float64(i+1) * 50})
	}
	// And a ring of three trading cells, which has no order: one steps aside.
	nodes = append(nodes, LayerNode{LaID: 5001, X: 0, Y: 500}, LayerNode{LaID: 5002, X: 50, Y: 500}, LayerNode{LaID: 5003, X: 100, Y: 500})
	moves = append(moves, Move{LaID: 5001, X: 50, Y: 500}, Move{LaID: 5002, X: 100, Y: 500}, Move{LaID: 5003, X: 0, Y: 500})

	if one := MoveBatches(nodes, moves[:500], 500); len(one) != 1 || len(one[0]) != 500 {
		t.Fatal("a layout that fits one call goes as it is")
	}
	if MoveBatches(nodes, nil, 500) != nil {
		t.Fatal("no moves, no calls")
	}
	batches := MoveBatches(nodes, moves, 500)
	at := map[int64][2]float64{}
	for _, n := range nodes {
		at[n.LaID] = [2]float64{n.X, n.Y}
	}
	total := 0
	for k, batch := range batches {
		if len(batch) > 500 {
			t.Fatalf("call %d moves %d", k, len(batch))
		}
		total += len(batch)
		moving := map[int64]bool{}
		for _, m := range batch {
			if moving[m.LaID] {
				t.Fatalf("call %d moves %d twice", k, m.LaID)
			}
			moving[m.LaID] = true
		}
		held := map[[2]float64]bool{}
		for id, p := range at {
			if !moving[id] {
				held[p] = true
			}
		}
		for _, m := range batch {
			if p := [2]float64{m.X, m.Y}; held[p] {
				t.Fatalf("call %d moves %d onto a held cell %v", k, m.LaID, p)
			} else {
				held[p] = true
			}
			at[m.LaID] = [2]float64{m.X, m.Y}
		}
	}
	if len(batches) < 3 || len(batches) > 4 || total != len(moves)+1 { // one step aside, in a call of its own turn
		t.Fatalf("%d calls, %d moves", len(batches), total)
	}
	for _, m := range moves {
		if at[m.LaID] != [2]float64{m.X, m.Y} {
			t.Fatalf("%d ended at %v, want %v", m.LaID, at[m.LaID], m)
		}
	}
}
