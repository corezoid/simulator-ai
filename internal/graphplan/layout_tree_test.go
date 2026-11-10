package graphplan

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// fan builds a tree of plain actors level by level: fan(6, 9) is one root, 6 children, 9 under each.
func fan(widths ...int) ([]LayoutNode, []Edge) {
	nodes := []LayoutNode{{Key: "r", Width: PlainSize, Height: PlainSize}}
	var edges []Edge
	level := []string{"r"}
	for _, width := range widths {
		var next []string
		for _, parent := range level {
			for k := range width {
				key := fmt.Sprintf("%s.%d", parent, k)
				nodes = append(nodes, LayoutNode{Key: key, Width: PlainSize, Height: PlainSize})
				edges = append(edges, Edge{Key: parent + ">" + key, Source: parent, Target: key})
				next = append(next, key)
			}
		}
		level = next
	}
	return nodes, edges
}

// crossings counts the pairs of links whose straight segments between box centres cross; links that
// share an end do not count.
func crossings(nodes []LayoutNode, edges []Edge, at []publicapi.GraphPoint) int {
	index := map[string]int{}
	for i, n := range nodes {
		index[n.Key] = i
	}
	side := func(a, b, c publicapi.GraphPoint) float64 {
		return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	}
	count := 0
	for i, e := range edges {
		for _, g := range edges[i+1:] {
			if e.Source == g.Source || e.Source == g.Target || e.Target == g.Source || e.Target == g.Target {
				continue
			}
			a, b, c, d := at[index[e.Source]], at[index[e.Target]], at[index[g.Source]], at[index[g.Target]]
			if side(a, b, c)*side(a, b, d) < 0 && side(c, d, a)*side(c, d, b) < 0 {
				count++
			}
		}
	}
	return count
}

type span struct{ l, t, r, b float64 }

func (s span) apart(o span) bool { return s.r < o.l || o.r < s.l || s.b < o.t || o.b < s.t }

// childSpans answers, per parent key, the box around its children's centres.
func childSpans(nodes []LayoutNode, edges []Edge, at []publicapi.GraphPoint) map[string]span {
	index := map[string]int{}
	for i, n := range nodes {
		index[n.Key] = i
	}
	out := map[string]span{}
	for _, e := range edges {
		p := at[index[e.Target]]
		s, ok := out[e.Source]
		if !ok {
			s = span{p.X, p.Y, p.X, p.Y}
		}
		out[e.Source] = span{min(s.l, p.X), min(s.t, p.Y), max(s.r, p.X), max(s.b, p.Y)}
	}
	return out
}

// assertGrouped fails when a parent is not centred over its children (within a cell) and above them,
// or the children of two parents of one rank share room.
func assertGrouped(t *testing.T, nodes []LayoutNode, edges []Edge, at []publicapi.GraphPoint, parents []string) {
	t.Helper()
	index := map[string]int{}
	for i, n := range nodes {
		index[n.Key] = i
	}
	spans := childSpans(nodes, edges, at)
	for i, p := range parents {
		s, own := spans[p], at[index[p]]
		if math.Abs(own.X-(s.l+s.r)/2) > gridCell || own.Y >= s.t {
			t.Fatalf("%s at %v is not centred over its children %+v", p, own, s)
		}
		for _, q := range parents[i+1:] {
			if !s.apart(spans[q]) {
				t.Fatalf("children of %s %+v and of %s %+v interleave", p, s, q, spans[q])
			}
		}
	}
}

func keysOf(nodes []LayoutNode, from, to int) []string {
	var out []string
	for _, n := range nodes[from:to] {
		out = append(out, n.Key)
	}
	return out
}

func TestLayeredGroupsAnOrgChart(t *testing.T) {
	nodes, edges := fan(6, 9) // a director, 6 departments, 9 employees each
	opt := LayoutOptions{Gap: PlainGap}
	at, _, box := Layered(nodes, edges, opt)
	assertApart(t, nodes, at, box)
	assertGrouped(t, nodes, edges, at, keysOf(nodes, 1, 7))
	assertGrouped(t, nodes, edges, at, []string{"r"})
	if n := crossings(nodes, edges, at); n != 0 {
		t.Fatalf("a tree has %d crossing links", n)
	}
	// Six departments in one row, each over a 3x3 block of its employees.
	if box.W != 52*gridCell || box.H != 13*gridCell {
		t.Fatalf("box %+v", box)
	}
	for _, p := range at[1:7] {
		if p.Y != at[1].Y {
			t.Fatal("departments are not on one row:", at[1:7])
		}
	}
	// Whole ranks interleave the departments' employees over wrapped rows.
	rows, _, rowsBox := Layered(nodes, edges, LayoutOptions{Gap: PlainGap, RankRows: true})
	assertApart(t, nodes, rows, rowsBox)
	if n := crossings(nodes, edges, rows); n < 100 {
		t.Fatalf("rank rows cross %d times", n)
	}
	// Room for every department beside the others: 9 employees still fold, into 5 and 4.
	wide, _, wideBox := Layered(nodes, edges, LayoutOptions{Gap: PlainGap, RowWidth: 400 * gridCell})
	assertApart(t, nodes, wide, wideBox)
	if wideBox.W != 88*gridCell || wideBox.H != 10*gridCell || crossings(nodes, edges, wide) != 0 {
		t.Fatalf("wide box %+v", wideBox)
	}
}

func TestLayeredWrapsChildrenUnderTheirHub(t *testing.T) {
	nodes, edges := fan(10, 30)
	hubs := keysOf(nodes, 1, 11)
	at, _, box := Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
	assertApart(t, nodes, at, box)
	assertGrouped(t, nodes, edges, at, hubs)
	if box.W > DefaultRowWidth {
		t.Fatalf("box %+v is wider than a row", box)
	}
	// Ten hubs fit one row once each folds its 30 points into two columns: nothing crosses.
	if n := crossings(nodes, edges, at); n != 0 || box.W != 58*gridCell || box.H != 49*gridCell {
		t.Fatalf("%d crossings, box %+v", n, box)
	}
	// Twenty hubs do not: each takes a near-square block, and the hubs wrap, block by block.
	nodes, edges = fan(20, 30)
	at, _, box = Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
	assertApart(t, nodes, at, box)
	assertGrouped(t, nodes, edges, at, keysOf(nodes, 1, 21))
	rows := map[float64]bool{}
	for _, p := range at[1:21] {
		rows[p.Y] = true
	}
	if box.W > DefaultRowWidth || len(rows) < 2 {
		t.Fatalf("box %+v, hub rows %d", box, len(rows))
	}
	// One parent's childless children make a near-square block under it, not a strip; up to six stay
	// a row.
	for _, tc := range []struct{ children, w, h int }{{6, 16, 4}, {7, 10, 7}, {25, 13, 16}, {30, 16, 16}, {400, 58, 61}} {
		nodes, edges = fan(tc.children)
		at, _, box = Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
		assertApart(t, nodes, at, box)
		assertGrouped(t, nodes, edges, at, []string{"r"})
		if box.W != float64(tc.w)*gridCell || box.H != float64(tc.h)*gridCell {
			t.Fatalf("%d children: box %+v", tc.children, box)
		}
	}
}

func TestLayeredOrdersSharedChildrenAndForests(t *testing.T) {
	plain := func(keys ...string) []LayoutNode {
		out := make([]LayoutNode, len(keys))
		for i, k := range keys {
			out[i] = LayoutNode{Key: k, Width: PlainSize, Height: PlainSize}
		}
		return out
	}
	link := func(pairs ...string) []Edge {
		var out []Edge
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, Edge{Key: pairs[i] + ">" + pairs[i+1], Source: pairs[i], Target: pairs[i+1]})
		}
		return out
	}
	x := func(nodes []LayoutNode, at []publicapi.GraphPoint, key string) float64 {
		return at[slices.IndexFunc(nodes, func(n LayoutNode) bool { return n.Key == key })].X
	}
	opt := LayoutOptions{Gap: PlainGap}

	// Children listed against their parents' order are sorted under them; a child of three sits
	// under the middle one, a child of two between them.
	nodes := plain("top", "a", "b", "c", "c1", "b1", "a1", "all", "ab")
	edges := link("top", "a", "top", "b", "top", "c", "c", "c1", "b", "b1", "a", "a1", "a", "all", "b", "all", "c", "all", "a", "ab", "b", "ab")
	at, returns, box := Layered(nodes, edges, opt)
	assertApart(t, nodes, at, box)
	if len(returns) != 0 || crossings(nodes, edges, at) > 2 { // a's link to the child of all three passes b's own two
		t.Fatalf("returns %v, %d crossings", returns, crossings(nodes, edges, at))
	}
	if !(x(nodes, at, "a") < x(nodes, at, "b") && x(nodes, at, "b") < x(nodes, at, "c") && x(nodes, at, "a1") < x(nodes, at, "b1") && x(nodes, at, "b1") < x(nodes, at, "c1")) {
		t.Fatal("children do not follow their parents:", at)
	}
	if all := x(nodes, at, "all"); all <= x(nodes, at, "a1") || all >= x(nodes, at, "c1") {
		t.Fatal("the shared child is not in the middle:", at)
	}
	// Whole ranks get the same order.
	rows, _, rowsBox := Layered(nodes, edges, LayoutOptions{Gap: PlainGap, RankRows: true})
	assertApart(t, nodes, rows, rowsBox)
	if !(x(nodes, rows, "a1") < x(nodes, rows, "b1") && x(nodes, rows, "b1") < x(nodes, rows, "c1")) {
		t.Fatal("rank rows do not follow the parents:", rows)
	}

	// A forest with nodes of no link: every tree in its own block, in input order.
	nodes = plain("lone", "t1", "t1a", "t1b", "t2", "t2a", "t2b", "t2c", "last")
	edges = link("t1", "t1a", "t1", "t1b", "t2", "t2a", "t2", "t2b", "t2", "t2c")
	at, _, box = Layered(nodes, edges, opt)
	assertApart(t, nodes, at, box)
	assertGrouped(t, nodes, edges, at, []string{"t1", "t2"})
	if !(x(nodes, at, "lone") < x(nodes, at, "t1") && x(nodes, at, "t1") < x(nodes, at, "t2") && x(nodes, at, "t2") < x(nodes, at, "last")) || crossings(nodes, edges, at) != 0 {
		t.Fatal("forest:", at)
	}
	if box.H != 4*gridCell || at[0].Y != at[1].Y {
		t.Fatalf("roots share the first row: %+v %v", box, at)
	}
	// No links at all: balanced rows.
	nodes = plainBoxes(50)
	at, _, box = Layered(nodes, nil, opt)
	assertApart(t, nodes, at, box)
	if box.W != 49*gridCell || box.H != 7*gridCell { // 17, 17 and 16
		t.Fatalf("box %+v", box)
	}
	if at, _, box = Layered(nil, nil, opt); len(at) != 0 || box.W != 0 || box.H != 0 {
		t.Fatal(at, box)
	}
}

func TestLayeredSurvivesCyclesAndIsDeterministic(t *testing.T) {
	nodes := plainBoxes(40)
	var edges []Edge
	for i := range nodes { // a ring, chords across it and two-node rings
		for _, j := range []int{(i + 1) % 40, (i * 7) % 40, (i + 39) % 40} {
			if i != j {
				edges = append(edges, Edge{Key: fmt.Sprint(i, ">", j), Source: nodes[i].Key, Target: nodes[j].Key})
			}
		}
	}
	at, returns, box := Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
	assertApart(t, nodes, at, box)
	if len(returns) == 0 {
		t.Fatal("a ring has return links")
	}
	for range 3 {
		again, back, same := Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
		if !slices.Equal(again, at) || !slices.Equal(back, returns) || same != box {
			t.Fatal("layout is not deterministic")
		}
	}
	// Flowchart blocks of mixed sizes: ranks keep one centre line, corners stay on the grid.
	blocks := []LayoutNode{{"start", 200, 50}, {"check", 200, 150}, {"yes", 200, 100}, {"no", 100, 100}, {"y1", 200, 50}, {"n1", 200, 150}}
	links := []Edge{{"a", "start", "check", ""}, {"b", "check", "yes", ""}, {"c", "check", "no", ""}, {"d", "yes", "y1", ""}, {"e", "no", "n1", ""}, {"f", "n1", "start", ""}}
	at, returns, box = Layered(blocks, links, LayoutOptions{Origin: publicapi.GraphPoint{X: 1025, Y: 25}, Centered: true})
	assertApart(t, blocks, at, box)
	if len(returns) != 1 || at[2].Y != at[3].Y || at[4].Y != at[5].Y || at[0].X != at[1].X || crossings(blocks, links[:5], at) != 0 {
		t.Fatal(returns, at)
	}
	if mid := box.X + box.W/2; math.Abs(mid-1025) > gridCell {
		t.Fatalf("box %+v is not centred on the origin", box)
	}
}

// 5000 nodes in every shape lay out in milliseconds; the bound leaves room for the race detector.
func TestLayeredIsFast(t *testing.T) {
	shapes := map[string]func(i int) int{
		"chain":  func(i int) int { return i - 1 },
		"fan":    func(int) int { return 0 },
		"binary": func(i int) int { return (i - 1) / 2 },
		"hubs":   func(i int) int { return max(0, (i-1)/70*70-69) * min(1, (i-1)%70) },
	}
	for name, parent := range shapes {
		nodes := plainBoxes(MaxLayoutNodes)
		var edges []Edge
		for i := 1; i < len(nodes); i++ {
			edges = append(edges, Edge{Key: fmt.Sprint(i), Source: nodes[parent(i)].Key, Target: nodes[i].Key})
			if i%5 == 0 && i > 10 { // and shared children
				edges = append(edges, Edge{Key: fmt.Sprint("s", i), Source: nodes[parent(i-3)].Key, Target: nodes[i].Key})
			}
		}
		start := time.Now()
		at, _, box := Layered(nodes, edges, LayoutOptions{Gap: PlainGap})
		if took := time.Since(start); took > time.Second {
			t.Fatalf("%s: %v", name, took)
		}
		assertApart(t, nodes, at, box)
		if box.W > DefaultRowWidth {
			t.Fatalf("%s: box %+v", name, box)
		}
	}
}
