package graphplan

import (
	"math"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// LayoutNode is one box of a layout: a flowchart block of its catalog kind's size, or a plain actor
// (PlainSize).
type LayoutNode struct {
	Key    string
	Width  float64
	Height float64
}

const (
	// PlainSize is the side of a plain actor's cell.
	PlainSize = gridCell
	// PlainGap separates plain actors: two free cells keep a title and an edge label readable.
	PlainGap = 2 * gridCell
	// DefaultRowWidth wraps a rank: 60 cells hold about 8 flowchart blocks or 20 plain actors, which
	// still fits a screen at a readable zoom.
	DefaultRowWidth = 60 * gridCell
	// MaxLayoutNodes bounds one layout. Ranking is a DFS and a queue pass (O(nodes + edges)) and
	// packing one pass over the nodes, so the bound is about memory and the size of one request, not
	// time: 5000 boxes lay out in a few milliseconds.
	MaxLayoutNodes = 5000
)

// LayoutOptions puts a layout on the layer.
type LayoutOptions struct {
	// Origin is the top-left corner of the layout's box, moved onto the nearest grid line. With
	// Centered the box is centred on Origin.X instead (the flowchart_layout default).
	Origin   publicapi.GraphPoint
	Centered bool
	// RowWidth wraps a rank wider than this onto further rows; 0 takes DefaultRowWidth.
	RowWidth float64
	// Gap separates boxes and rows; 0 takes three cells, room for an edge label between blocks.
	Gap float64
}

// LayoutBox is the area a layout covers, in graph units, X and Y its top-left corner.
type LayoutBox struct{ X, Y, W, H float64 }

// GridLayout is Layered as flowchart_layout always placed it: centred on x = 0, from the second row.
func GridLayout(nodes []LayoutNode, edges []Edge) ([]publicapi.GraphPoint, []string) {
	points, returns, _ := Layered(nodes, edges, LayoutOptions{Origin: publicapi.GraphPoint{Y: gridPhase}, Centered: true})
	return points, returns
}

// Layered ranks nodes by longest path (return links excluded, as in Build) and lays the ranks top to
// bottom; a rank wider than RowWidth continues on the next row. Every box's corner is on a grid
// line, so placements keep exactly these centres. Same input, same layout. Edges must join known
// keys. It answers the centres, the return links' keys and the box covered.
func Layered(nodes []LayoutNode, edges []Edge, opt LayoutOptions) ([]publicapi.GraphPoint, []string, LayoutBox) {
	keys := make([]string, len(nodes))
	for i, n := range nodes {
		keys[i] = n.Key
	}
	rank, back := ranks(keys, edges)
	var returns []string
	for i, e := range edges {
		if back[i] {
			returns = append(returns, e.Key)
		}
	}
	depth := 0
	for _, r := range rank {
		depth = max(depth, r+1)
	}
	byRank := make([][]int, depth)
	for i, r := range rank {
		byRank[r] = append(byRank[r], i)
	}
	points, box := packRows(nodes, byRank, opt)
	return points, returns, box
}

// Grid lays nodes out in input order, row by row, in a near-square block no wider than RowWidth: the
// layout of a graph whose edges give it no direction.
func Grid(nodes []LayoutNode, opt LayoutOptions) ([]publicapi.GraphPoint, LayoutBox) {
	all := make([]int, len(nodes))
	widest := 0.0
	for i, n := range nodes {
		all[i] = i
		widest = max(widest, n.Width)
	}
	gap := opt.gap()
	perRow := int(math.Ceil(math.Sqrt(float64(len(nodes)))))
	if limit := int((opt.rowWidth() + gap) / (widest + gap)); limit >= 1 && perRow > limit {
		perRow = limit
	}
	var rows [][]int
	for start := 0; start < len(all); start += max(perRow, 1) {
		rows = append(rows, all[start:min(start+max(perRow, 1), len(all))])
	}
	return packRows(nodes, rows, opt)
}

// Layerable says whether the edges give the graph a direction worth ranking by: there is one, and at
// most a fifth of the edges run against it (return links). Otherwise Grid reads better.
func Layerable(nodes []LayoutNode, edges []Edge) bool {
	if len(edges) == 0 {
		return false
	}
	keys := make([]string, len(nodes))
	for i, n := range nodes {
		keys[i] = n.Key
	}
	_, back := ranks(keys, edges)
	returns := 0
	for _, b := range back {
		if b {
			returns++
		}
	}
	return returns*5 <= len(edges)
}

func (o LayoutOptions) gap() float64 {
	if o.Gap > 0 {
		return o.Gap
	}
	return layoutGap
}

func (o LayoutOptions) rowWidth() float64 {
	if o.RowWidth > 0 {
		return o.RowWidth
	}
	return DefaultRowWidth
}

// cells rounds a length up to whole cells: an advance from a grid line lands on a grid line.
func cells(v float64) float64 { return math.Ceil(v/gridCell) * gridCell }

// packRows places each group of nodes on its own row, wrapping a group wider than the row width.
// Rows are centred in the layout's box and boxes of one row share a centre line when their heights
// allow it. One pass over the nodes.
func packRows(nodes []LayoutNode, groups [][]int, opt LayoutOptions) ([]publicapi.GraphPoint, LayoutBox) {
	type row struct {
		nodes  []int
		width  float64
		height float64
	}
	gap, limit := opt.gap(), opt.rowWidth()
	var rows []row
	for _, group := range groups {
		cur := row{}
		for _, i := range group {
			n := nodes[i]
			next := n.Width
			if len(cur.nodes) > 0 {
				next = cells(cur.width+gap) + n.Width
				if next > limit {
					rows, cur, next = append(rows, cur), row{}, n.Width
				}
			}
			cur.nodes, cur.width, cur.height = append(cur.nodes, i), next, max(cur.height, n.Height)
		}
		if len(cur.nodes) > 0 {
			rows = append(rows, cur)
		}
	}
	box := LayoutBox{X: gridLine(opt.Origin.X), Y: gridLine(opt.Origin.Y)}
	for _, r := range rows {
		box.W = max(box.W, r.width)
	}
	if opt.Centered {
		box.X = gridLine(opt.Origin.X - box.W/2)
	}
	out := make([]publicapi.GraphPoint, len(nodes))
	top := box.Y
	for k, r := range rows {
		x := box.X + math.Floor((box.W-r.width)/2/gridCell)*gridCell
		for _, i := range r.nodes {
			n := nodes[i]
			y := top + math.Floor((r.height-n.Height)/2/gridCell)*gridCell
			out[i] = publicapi.GraphPoint{X: x + n.Width/2, Y: y + n.Height/2}
			x += cells(n.Width + gap)
		}
		box.H = top + r.height - box.Y
		if k < len(rows)-1 {
			top += cells(r.height + gap)
		}
	}
	return out, box
}
