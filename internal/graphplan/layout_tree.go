package graphplan

import (
	"math"
	"sort"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

const (
	// fanRow is the most childless children a node keeps in one row when there is room; more fold into
	// a near-square block under it, at least fanColumns wide: 25 make 5 by 5, not a strip of 25.
	fanRow     = 6
	fanColumns = 5
)

// orderPasses is the number of downward barycentre sweeps; an upward one runs between two of them.
const orderPasses = 3

// rankOrder lists the nodes of every rank, ordered to keep links short and uncrossed: a rank is
// sorted by the mean position of its nodes' neighbours on the other side (barycentre), down then up,
// a fixed number of times. Ties keep the input order, and a node with nothing on that side keeps its
// slot, so a tree given in order is left as it is. links are the forward links, [source, target].
// O(passes * (links + nodes log nodes)).
func rankOrder(rank []int, links [][2]int) [][]int {
	depth := 0
	for _, r := range rank {
		depth = max(depth, r+1)
	}
	byRank := make([][]int, depth)
	for i, r := range rank {
		byRank[r] = append(byRank[r], i)
	}
	above, below := make([][]int, len(rank)), make([][]int, len(rank))
	for _, l := range links {
		below[l[0]] = append(below[l[0]], l[1])
		above[l[1]] = append(above[l[1]], l[0])
	}
	// at is a node's place in its rank as a share of the rank, so links that skip ranks compare too.
	at, key := make([]float64, len(rank)), make([]float64, len(rank))
	place := func(row []int) {
		for k, i := range row {
			at[i] = (float64(k) + 0.5) / float64(len(row))
		}
	}
	for _, row := range byRank {
		place(row)
	}
	sweep := func(row []int, side [][]int) {
		var slots, moving []int
		for k, i := range row {
			if len(side[i]) == 0 {
				continue
			}
			sum := 0.0
			for _, j := range side[i] {
				sum += at[j]
			}
			key[i] = sum / float64(len(side[i]))
			slots, moving = append(slots, k), append(moving, i)
		}
		sort.SliceStable(moving, func(a, b int) bool { return key[moving[a]] < key[moving[b]] })
		for j, k := range slots {
			row[k] = moving[j]
		}
		place(row)
	}
	for pass := range orderPasses {
		for r := 1; r < depth; r++ {
			sweep(byRank[r], above)
		}
		if pass == orderPasses-1 {
			break
		}
		for r := depth - 2; r >= 0; r-- {
			sweep(byRank[r], below)
		}
	}
	return byRank
}

// forest is the layout of a graph as trees: every node below the first rank hangs under one parent
// of the rank above, and a node's subtree takes a block of its own, the node centred over it. The
// node past the last one is the root of the roots: it has no box.
type forest struct {
	nodes      []LayoutNode
	rank       []int
	gap, limit float64
	kids       [][]int
	// head is the height of a rank's row: nodes of a rank share a centre line wherever blocks are
	// side by side.
	head []float64
	// nat is a subtree's width with every node's children in one row (a fan of childless ones folded,
	// see fanRow), low the narrowest it can be (its widest box), area the room its boxes take with
	// their gaps.
	nat, low, area []float64
	// budget is the width a subtree may take; w and h are its block's size; rows counts the children
	// in each row of the block.
	budget, w, h []float64
	rows         [][]int
}

// packTrees places every subtree in its own block: children sit in a row under their parent, and a
// row that does not fit the width is first narrowed (subtrees fold their own children into several
// rows, down to two columns) and only then wrapped, inside the parent's block. Blocks are disjoint,
// so a tree has no crossing links while its rows are whole, and no box is wider than RowWidth unless
// a single node is. order is rankOrder's; links are the forward links. Near-linear:
// O(nodes log RowWidth).
func packTrees(nodes []LayoutNode, rank []int, order [][]int, links [][2]int, opt LayoutOptions) ([]publicapi.GraphPoint, LayoutBox) {
	n := len(nodes)
	f := &forest{
		nodes: nodes, rank: rank, gap: opt.gap(), limit: opt.rowWidth(), kids: make([][]int, n+1), head: make([]float64, len(order)),
		nat: make([]float64, n+1), low: make([]float64, n+1), area: make([]float64, n+1),
		budget: make([]float64, n+1), w: make([]float64, n+1), h: make([]float64, n+1), rows: make([][]int, n+1),
	}
	f.adopt(order, links)
	for r := len(order) - 1; r >= 0; r-- {
		for _, v := range order[r] {
			f.head[r] = max(f.head[r], nodes[v].Height)
			f.measure(v)
		}
	}
	f.measure(n)
	f.budget[n] = f.limit
	f.share(n)
	for _, row := range order {
		for _, v := range row {
			f.share(v)
		}
	}
	for r := len(order) - 1; r >= 0; r-- {
		for _, v := range order[r] {
			f.size(v)
		}
	}
	f.size(n)
	box := LayoutBox{X: gridLine(opt.Origin.X), Y: gridLine(opt.Origin.Y), W: f.w[n], H: f.h[n]}
	if opt.Centered {
		box.X = gridLine(opt.Origin.X - box.W/2)
	}
	out := make([]publicapi.GraphPoint, n)
	corner := make([]publicapi.GraphPoint, n+1)
	corner[n] = publicapi.GraphPoint{X: box.X, Y: box.Y}
	f.put(n, corner, out)
	for _, row := range order {
		for _, v := range row {
			f.put(v, corner, out)
		}
	}
	return out, box
}

// adopt gives every node below the first rank its parent: of its parents one rank up, the middle one
// in the rank's order, so a shared child sits between those who share it. Children keep that order.
func (f *forest) adopt(order [][]int, links [][2]int) {
	n := len(f.nodes)
	pos := make([]int, n)
	for _, row := range order {
		for k, i := range row {
			pos[i] = k
		}
	}
	above := make([][]int, n)
	for _, l := range links {
		if f.rank[l[0]] == f.rank[l[1]]-1 {
			above[l[1]] = append(above[l[1]], l[0])
		}
	}
	for r, row := range order {
		for _, i := range row {
			parent := n
			if r > 0 {
				sort.SliceStable(above[i], func(a, b int) bool { return pos[above[i][a]] < pos[above[i][b]] })
				parent = above[i][(len(above[i])-1)/2]
			}
			f.kids[parent] = append(f.kids[parent], i)
		}
	}
}

func snapDown(v float64) float64 { return math.Floor(v/gridCell) * gridCell }

// across is the width of a row once a box of the given width joins it.
func (f *forest) across(row float64, count int, width float64) float64 {
	if count == 0 {
		return width
	}
	return cells(row+f.gap) + width
}

func (f *forest) own(v int) (float64, float64) {
	if v == len(f.nodes) {
		return 0, 0
	}
	return f.nodes[v].Width, f.nodes[v].Height
}

func (f *forest) measure(v int) {
	width, height := f.own(v)
	row, low := 0.0, width
	f.area[v] = cells(width+f.gap) * cells(height+f.gap)
	fold, leaves := len(f.kids[v]), v < len(f.nodes)
	for _, c := range f.kids[v] {
		leaves = leaves && len(f.kids[c]) == 0
	}
	if leaves && fold > fanRow {
		fold = max(fanColumns, int(math.Ceil(math.Sqrt(float64(fold)))))
	}
	widest := 0.0
	for k, c := range f.kids[v] {
		row, low = f.across(row, k%fold, f.nat[c]), max(low, f.low[c])
		widest = max(widest, row)
		f.area[v] += f.area[c]
	}
	f.nat[v], f.low[v] = max(width, widest), low
}

// floor is the narrowest a subtree is folded to keep its parent's row whole: two of its children
// side by side. One column under a parent would read as a chain.
func (f *forest) floor(v int) float64 {
	if len(f.kids[v]) < 2 {
		return f.low[v]
	}
	widest := 0.0
	for _, c := range f.kids[v] {
		widest = max(widest, f.low[c])
	}
	return max(f.low[v], cells(widest+f.gap)+widest)
}

// share gives a node's children their widths: their natural ones when the row fits, else one cap for
// all that makes it fit, and when that would fold a child below its floor, a near-square block each
// and several rows of them.
func (f *forest) share(v int) {
	kids, budget := f.kids[v], f.budget[v]
	fits := func(limit float64) bool {
		row := 0.0
		for k, c := range kids {
			row = f.across(row, k, min(f.nat[c], limit))
		}
		return row <= budget
	}
	limit := math.Inf(1)
	if !fits(limit) {
		lo, hi := 0, int(budget/gridCell) // lo cells fit (0: none do), hi is the widest worth trying
		for lo < hi {
			if mid := (lo + hi + 1) / 2; fits(float64(mid) * gridCell) {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		limit = float64(lo) * gridCell
		for _, c := range kids {
			if f.nat[c] > limit && limit < f.floor(c) {
				limit = 0
				break
			}
		}
	}
	for _, c := range kids {
		f.budget[c] = min(f.nat[c], limit)
		if limit == 0 {
			f.budget[c] = min(f.nat[c], max(f.floor(c), snapDown(math.Sqrt(f.area[c]))), max(budget, f.low[c]))
		}
	}
}

// size packs a node's children's blocks into rows no wider than its budget and measures its block.
// Blocks of one width are spread evenly over the rows.
func (f *forest) size(v int) {
	kids := f.kids[v]
	width, _ := f.own(v)
	var rows []int
	row, uniform := 0.0, true
	for _, c := range kids {
		next := f.across(row, 1, f.w[c])
		if len(rows) == 0 || next > f.budget[v] {
			rows, next = append(rows, 0), f.w[c]
		}
		rows[len(rows)-1]++
		row, uniform = next, uniform && f.w[c] == f.w[kids[0]]
	}
	if uniform && len(rows) > 1 {
		for k := range rows {
			rows[k] = len(kids) / len(rows)
			if k < len(kids)%len(rows) {
				rows[k]++
			}
		}
	}
	f.rows[v] = rows
	top := 0.0
	if v < len(f.nodes) {
		top = f.head[f.rank[v]]
		if len(kids) > 0 {
			top = cells(top + f.gap)
		}
	}
	f.w[v], f.h[v] = width, top
	start := 0
	for k, count := range rows {
		across, tall := 0.0, 0.0
		for j, c := range kids[start : start+count] {
			across, tall = f.across(across, j, f.w[c]), max(tall, f.h[c])
		}
		start += count
		f.w[v], f.h[v] = max(f.w[v], across), top+tall
		if k < len(rows)-1 {
			top += cells(tall + f.gap)
		}
	}
}

// put places a node at the top of its block, centred, and hands its children their blocks' corners.
func (f *forest) put(v int, corner, out []publicapi.GraphPoint) {
	at := corner[v]
	top := at.Y
	if v < len(f.nodes) {
		n, head := f.nodes[v], f.head[f.rank[v]]
		out[v] = publicapi.GraphPoint{X: at.X + snapDown((f.w[v]-n.Width)/2) + n.Width/2, Y: at.Y + snapDown((head-n.Height)/2) + n.Height/2}
		top += cells(head + f.gap)
	}
	start := 0
	for _, count := range f.rows[v] {
		row := f.kids[v][start : start+count]
		start += count
		across, tall := 0.0, 0.0
		for j, c := range row {
			across, tall = f.across(across, j, f.w[c]), max(tall, f.h[c])
		}
		x := at.X + snapDown((f.w[v]-across)/2)
		for _, c := range row {
			corner[c] = publicapi.GraphPoint{X: x, Y: top}
			x += cells(f.w[c] + f.gap)
		}
		top += cells(tall + f.gap)
	}
}
