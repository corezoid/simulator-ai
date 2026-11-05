package publicapi

import (
	"encoding/json"
	"math"
	"testing"
)

// Grid snapping rounds as JavaScript does (halves up), so -25 is 0 and 25 is 50, and never -0.
func TestSnapOffsetIsJavaScriptRound(t *testing.T) {
	for in, want := range map[float64]float64{-25: 0, 25: 50, 24: 0, -26: -50, 285: 300, -75: -50, 0: 0} {
		if got := snapOffset(in); got != want || math.Signbit(got) && got == 0 {
			t.Errorf("snapOffset(%v) = %v, want %v", in, got, want)
		}
	}
	if d := deriveOffsetDirective(cellBox(0, 0), graphRect{X: -30, Y: -30, W: 60, H: 60}); d != nil {
		t.Error("a sub-cell growth sent a directive", d)
	}
}

// The make-room predicate is the server's: centres strictly past the old edges move, a zero side
// moves nobody.
func TestDirectiveMoves(t *testing.T) {
	d := OffsetDirective{Bounds: OffsetBounds{Left: -25, Right: 25, Top: 125, Bottom: 175}, Offset: SideOffset{Right: 100, Bottom: 100}}
	for _, tc := range []struct{ x, y, wx, wy float64 }{
		{250, 0, 350, 0}, {100, 200, 200, 300}, {25, 150, 25, 150}, {-100, 150, -100, 150}, {0, 100, 0, 100},
	} {
		if x, y, _ := d.moves(tc.x, tc.y); x != tc.wx || y != tc.wy {
			t.Errorf("(%v,%v) moved to (%v,%v), want (%v,%v)", tc.x, tc.y, x, y, tc.wx, tc.wy)
		}
	}
}

// Making room for an area grows it from a seam through its middle along its narrower side: every
// cell placement inside ends up outside, and the rest of that side keeps its layout.
func TestInsertDirectiveClearsTheArea(t *testing.T) {
	tall := graphRect{X: -75, Y: -25, W: 200, H: 300} // a flowchart column
	d := insertDirective(tall)
	if jsonOf(t, d) != `{"bounds":{"left":25,"right":25,"top":-25,"bottom":275},"offset":{"top":0,"right":100,"bottom":0,"left":100}}` {
		t.Fatal(jsonOf(t, d))
	}
	room := layerRoom{}
	for x := -200.0; x <= 300; x += 50 {
		for y := -100.0; y <= 350; y += 50 {
			room.Occupants = append(room.Occupants, occupant{LaID: int64(len(room.Occupants) + 1), X: x, Y: y, Box: cellOf(x, y)})
		}
	}
	if left := room.after(d, 0).conflicts([]graphRect{tall}, 0); len(left) > 0 {
		t.Errorf("still inside: %s", cellsOf(left))
	}
	wide := graphRect{X: -25, Y: -25, W: 300, H: 50}
	if d := insertDirective(wide); d.Offset.Left != 0 || d.Offset.Right != 0 || d.Offset.Top+d.Offset.Bottom != 50 {
		t.Error("a wide area did not insert a row", jsonOf(t, d))
	}
	if left := room.after(insertDirective(wide), 0).conflicts([]graphRect{wide}, 0); len(left) > 0 {
		t.Errorf("row still taken: %s", cellsOf(left))
	}
}

// What a placement holds: its cell, an open node its box, a flowchart block its own box; pictures,
// drawings and markup hold nothing.
func TestFootprints(t *testing.T) {
	var nodes []layerNodeFull
	if err := json.Unmarshal([]byte(`[
		{"laId":1,"id":"a","position":{"x":110,"y":40}},
		{"laId":2,"id":"b","position":{"x":0,"y":0},"layerSettings":{"expand":true,"expandType":"layer"}},
		{"laId":3,"id":"c","position":{"x":500,"y":0},"layerSettings":{"blockId":"process","width":200,"height":100}},
		{"laId":4,"id":"d","position":{"x":0,"y":0},"layerSettings":{"image":{"src":"x"}}},
		{"laId":5,"id":"e","position":{"x":0,"y":0},"areaPicture":{"img":"x","width":10,"height":10}},
		{"laId":6,"id":"f","position":{"x":0,"y":0},"polygon":[[0,0],[50,0],[50,50]]},
		{"laId":7,"id":"g","position":{"x":0,"y":0},"layerSettings":{"drawing":{"shape":"ink"}}},
		{"laId":8,"id":"h","position":{"x":0,"y":0},"hole":true},
		{"laId":9,"id":"i","accessDenied":true,"position":{"x":50,"y":50}},
		{"laId":10,"id":"j"},
		{"laId":11,"id":"k","position":{"x":0,"y":0},"actor":{"id":"k","pictureObject":{"type":"avatar","img":"x"}}},
		{"laId":12,"id":"l","position":{"x":0,"y":0},"actor":{"id":"l","pictureObject":{"type":"napkin","img":"data:x"}}},
		{"laId":13,"id":"m","position":{"x":0,"y":0},"actor":{"id":"m","pictureObject":{"type":"image","img":""}}}
	]`), &nodes); err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{
		1: `{"X":75,"Y":25,"W":50,"H":50}`, 2: `{"X":-325,"Y":-225,"W":650,"H":450}`, 3: `{"X":400,"Y":-50,"W":200,"H":100}`,
		8: `{"X":-25,"Y":-25,"W":50,"H":50}`, 9: `{"X":25,"Y":25,"W":50,"H":50}`,
		// An avatar or an image without a source is still an actor; a napkin is a picture.
		11: `{"X":-25,"Y":-25,"W":50,"H":50}`, 13: `{"X":-25,"Y":-25,"W":50,"H":50}`,
	}
	for _, n := range nodes {
		box, ok := n.footprint()
		if w, held := want[n.LaID]; held != ok || ok && jsonOf(t, box) != w {
			t.Errorf("laId %d (%s) holds %v %s, want %s", n.LaID, n.kind(), ok, jsonOf(t, box), w)
		}
	}
	if areaCells(graphRect{X: -325, Y: -225, W: 650, H: 450}) != "-F, -4 - G, 5" {
		t.Error(areaCells(graphRect{X: -325, Y: -225, W: 650, H: 450}))
	}
}
