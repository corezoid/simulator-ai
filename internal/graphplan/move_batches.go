package graphplan

import "math"

// MoveBatches cuts a layout's moves into calls of at most size placements, for a server that takes
// that many positions at once and refuses a target cell another placement still holds. Within one
// call the moves happen together, so a vacated cell can be taken; across calls they do not, so the
// moves are ordered: a placement moves only after the one on its target cell has left. A ring of
// placements trading cells has no such order: one of them first steps aside, to a free cell right of
// everything, and takes its place once it is free. A placement is never in one call twice. One pass
// over the moves; nodes are the layer's placements as read.
func MoveBatches(nodes []LayerNode, moves []Move, size int) [][]Move {
	if len(moves) <= size {
		if len(moves) == 0 {
			return nil
		}
		return [][]Move{moves}
	}
	type cell [2]float64
	at := func(x, y float64) cell {
		return cell{math.Round(x/layerActorGrid) * layerActorGrid, math.Round(y/layerActorGrid) * layerActorGrid}
	}
	moving := map[int64]bool{}
	for _, m := range moves {
		moving[m.LaID] = true
	}
	from := map[int64]cell{}
	holders := map[cell]int{} // placements still to move, per cell they stand on
	right, top := math.Inf(-1), math.Inf(1)
	for _, n := range nodes {
		right, top = math.Max(right, n.X), math.Min(top, n.Y)
		if moving[n.LaID] {
			from[n.LaID] = at(n.X, n.Y)
			holders[from[n.LaID]]++
		}
	}
	for _, m := range moves {
		right, top = math.Max(right, m.X), math.Min(top, m.Y)
	}
	park := at(right, top)
	park[0] += 10 * layerActorGrid

	waiting := map[cell][]int{}
	var ready, ordered []int // indexes into moves; a negative entry -(i+1) steps move i aside
	done := make([]bool, len(moves))
	for i, m := range moves {
		if target := at(m.X, m.Y); holders[target] > 0 && target != from[m.LaID] {
			waiting[target] = append(waiting[target], i)
		} else {
			ready = append(ready, i)
		}
	}
	leave := func(i int) {
		c := from[moves[i].LaID]
		if holders[c]--; holders[c] == 0 {
			ready = append(ready, waiting[c]...)
			delete(waiting, c)
		}
	}
	next := 0
	for emitted := 0; emitted < len(moves); {
		if len(ready) == 0 {
			for done[next] || from[moves[next].LaID] == park {
				next++
			}
			// Everything left waits in a ring: this one steps aside and frees its cell.
			ordered = append(ordered, -(next + 1))
			leave(next)
			from[moves[next].LaID] = park
			continue
		}
		i := ready[0]
		ready = ready[1:]
		ordered, done[i] = append(ordered, i), true
		emitted++
		if from[moves[i].LaID] != park {
			leave(i)
		}
	}

	var out [][]Move
	var batch []Move
	in := map[int64]bool{}
	parked := 0
	for _, i := range ordered {
		m := Move{}
		if i < 0 {
			m = Move{LaID: moves[-i-1].LaID, X: park[0] + float64(parked)*layerActorGrid, Y: park[1]}
			parked++
		} else {
			m = moves[i]
		}
		if len(batch) == size || in[m.LaID] {
			out, batch, in = append(out, batch), nil, map[int64]bool{}
		}
		batch, in[m.LaID] = append(batch, m), true
	}
	return append(out, batch)
}
