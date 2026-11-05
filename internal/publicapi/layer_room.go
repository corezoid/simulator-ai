package publicapi

import "math"

// Anisotropy "make room", ported from simulator-ui-kit graph-layer store/anisotropy.ts. When a
// node's footprint grows, graph/layer/actors/offset pushes every placement whose CENTRE lies past
// the node's OLD edges outward by how far each side grew, over the whole layer (strict > right and
// bottom, < left and top; a zero side moves nobody). A negative side pulls them back, which is how
// a collapse returns them. The plugin sends the same directive the Graph Editor sends.

// graphRect is a box in graph units: X, Y its top-left corner (y grows down).
type graphRect struct{ X, Y, W, H float64 }

func (r graphRect) right() float64  { return r.X + r.W }
func (r graphRect) bottom() float64 { return r.Y + r.H }

// OffsetBounds is the grown node's footprint before it grew (top is the smaller y).
type OffsetBounds struct {
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}

// SideOffset is how far each side grew, grid-snapped and signed (a shrink is negative).
type SideOffset struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

func (o SideOffset) zero() bool { return o == SideOffset{} }

// OffsetDirective is graph/layer/actors/offset's {bounds, offset}.
type OffsetDirective struct {
	Bounds OffsetBounds `json:"bounds"`
	Offset SideOffset   `json:"offset"`
}

// snapOffset rounds a growth to whole cells as the client and the server do (JS Math.round:
// halves go up, so -25 snaps to 0, not -50); +0 rather than -0 keeps the wire tidy.
func snapOffset(v float64) float64 {
	s := math.Floor(v/gridCell+0.5) * gridCell
	if s == 0 {
		return 0
	}
	return s
}

// deriveOffsetDirective turns a footprint change into the make-room directive, nil when no side
// crosses a grid line (anisotropy.ts deriveOffsetDirective).
func deriveOffsetDirective(before, after graphRect) *OffsetDirective {
	d := OffsetDirective{
		Bounds: OffsetBounds{Left: before.X, Right: before.right(), Top: before.Y, Bottom: before.bottom()},
		Offset: SideOffset{
			Right:  snapOffset(after.right() - before.right()),
			Left:   snapOffset(before.X - after.X),
			Top:    snapOffset(before.Y - after.Y),
			Bottom: snapOffset(after.bottom() - before.bottom()),
		},
	}
	if d.Offset.zero() {
		return nil
	}
	return &d
}

// moves reports whether the directive moves a placement centred at (x, y), and where to: the
// predicate the server's SQL applies (pg_graph.OffsetActorsPositions).
func (d OffsetDirective) moves(x, y float64) (float64, float64, bool) {
	dx, dy := 0.0, 0.0
	if d.Offset.Right != 0 && x > d.Bounds.Right {
		dx += d.Offset.Right
	}
	if d.Offset.Left != 0 && x < d.Bounds.Left {
		dx -= d.Offset.Left
	}
	if d.Offset.Bottom != 0 && y > d.Bounds.Bottom {
		dy += d.Offset.Bottom
	}
	if d.Offset.Top != 0 && y < d.Bounds.Top {
		dy -= d.Offset.Top
	}
	return x + dx, y + dy, dx != 0 || dy != 0
}

// insertDirective makes room for new nodes landing on an occupied area. The area grows from a seam
// to its full box, as a node opening in place grows from its cell: the seam is a grid line through
// its middle, where no snapped centre ever sits, so every placement inside the area ends up past
// its edges and the layer on that side moves with it, keeping its own layout. One axis is enough to
// clear the area, and the narrower one moves the layer least: a tall area inserts columns, a wide
// one rows. nil when nothing needs moving.
func insertDirective(area graphRect) *OffsetDirective {
	seam := func(c float64) float64 { return math.Floor(c/gridCell)*gridCell + gridCell/2 }
	if area.W <= area.H {
		return deriveOffsetDirective(graphRect{X: seam(area.X + area.W/2), Y: area.Y, H: area.H}, area)
	}
	return deriveOffsetDirective(graphRect{X: area.X, Y: seam(area.Y + area.H/2), W: area.W}, area)
}

// cellBox is the one-cell footprint of a placement centred at (x, y).
func cellBox(x, y float64) graphRect {
	return graphRect{X: x - gridCell/2, Y: y - gridCell/2, W: gridCell, H: gridCell}
}

// actorBox is a plain actor's collapsed body by its sizeGrade (placementGeometry.ts
// actorNodeSize): S2 80, S3 100, else one cell.
func actorBox(x, y float64, sizeGrade string) graphRect {
	side := gridCell
	switch sizeGrade {
	case "S2":
		side = 80
	case "S3":
		side = 100
	}
	return graphRect{X: x - side/2, Y: y - side/2, W: side, H: side}
}

// union is the smallest box holding both.
func (r graphRect) union(o graphRect) graphRect {
	x, y := math.Min(r.X, o.X), math.Min(r.Y, o.Y)
	return graphRect{X: x, Y: y, W: math.Max(r.right(), o.right()) - x, H: math.Max(r.bottom(), o.bottom()) - y}
}

// holds reports whether a centre lies inside the box (edges excluded, as the offset predicate).
func (r graphRect) holds(x, y float64) bool {
	return x > r.X && x < r.right() && y > r.Y && y < r.bottom()
}

// overlaps reports whether two boxes share area (touching edges do not).
func (r graphRect) overlaps(o graphRect) bool {
	return r.X < o.right() && o.X < r.right() && r.Y < o.bottom() && o.Y < r.bottom()
}
