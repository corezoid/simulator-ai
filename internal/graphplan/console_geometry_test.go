package graphplan

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// onLine reports whether v is a Graph Editor grid line (25 + 50k).
func onLine(v float64) bool { return math.Mod(math.Abs(v-gridPhase), gridCell) == 0 }

func TestGridCenterPutsCornersOnGridLines(t *testing.T) {
	for _, tc := range []struct{ x, y, w, h float64 }{{0, 0, 200, 100}, {0, 200, 200, 50}, {13, -61, 100, 100}, {-240, 330, 200, 150}} {
		c := GridCenter(publicapi.GraphPoint{X: tc.x, Y: tc.y}, tc.w, tc.h)
		if !onLine(c.X-tc.w/2) || !onLine(c.Y-tc.h/2) || !onLine(c.X+tc.w/2) || !onLine(c.Y+tc.h/2) {
			t.Errorf("%v -> %v: box edges are off the grid", tc, c)
		}
		if math.Abs(c.X-tc.x) > gridCell/2 || math.Abs(c.Y-tc.y) > gridCell/2 {
			t.Errorf("%v -> %v moved more than half a cell", tc, c)
		}
		if again := GridCenter(c, tc.w, tc.h); again != c {
			t.Errorf("not idempotent: %v -> %v", c, again)
		}
	}
	// sim-api snaps a non-graphic centre to 50: an even-sized block would straddle cells.
	// Exact half cells round up like the editor's JS Math.round, for negative corners too.
	if c := GridCenter(publicapi.GraphPoint{}, 200, 100); c.X != 25 || c.Y != 25 {
		t.Fatal("200x100 at origin:", c)
	}
	if c := GridCenter(publicapi.GraphPoint{}, 100, 100); c.X != 25 || c.Y != 25 {
		t.Fatal("100x100 at origin:", c)
	}
}

func TestGridLayoutHasNoOverlapAndWholeCells(t *testing.T) {
	nodes := []LayoutNode{{"start", 200, 50}, {"check", 200, 150}, {"yes", 200, 100}, {"no", 100, 100}, {"end", 200, 50}}
	edges := []Edge{{"a", "start", "check", ""}, {"b", "check", "yes", ""}, {"c", "check", "no", ""}, {"d", "yes", "end", ""}, {"e", "no", "end", ""}, {"f", "end", "start", ""}}
	pos, returns := GridLayout(nodes, edges)
	if strings.Join(returns, ",") != "f" {
		t.Fatal("return edges:", returns)
	}
	type box struct{ l, t, r, b float64 }
	boxes := make([]box, len(nodes))
	for i, n := range nodes {
		boxes[i] = box{pos[i].X - n.Width/2, pos[i].Y - n.Height/2, pos[i].X + n.Width/2, pos[i].Y + n.Height/2}
		if !onLine(boxes[i].l) || !onLine(boxes[i].t) || !onLine(boxes[i].r) || !onLine(boxes[i].b) {
			t.Errorf("%s is off the grid: %+v", n.Key, boxes[i])
		}
	}
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if a.l < b.r+layoutGap && b.l < a.r+layoutGap && a.t < b.b+layoutGap && b.t < a.b+layoutGap {
				t.Errorf("%s and %s are closer than %v", nodes[i].Key, nodes[j].Key, layoutGap)
			}
		}
	}
	if !(pos[0].Y < pos[1].Y && pos[1].Y < pos[2].Y && pos[2].Y == pos[3].Y && pos[3].Y < pos[4].Y) {
		t.Fatal("ranks are not top to bottom:", pos)
	}
	if pos[0].X != pos[1].X || pos[1].X != pos[4].X {
		t.Fatal("a single-block rank chain is not on one vertical line:", pos)
	}
	again, _ := GridLayout(nodes, edges)
	for i := range pos {
		if again[i] != pos[i] {
			t.Fatal("layout is not deterministic")
		}
	}
}

func TestGridLayoutKeepsOddSizesOnGridLines(t *testing.T) {
	nodes := []LayoutNode{{"a", 120, 60}, {"b", 120, 60}, {"c", 70, 30}}
	pos, _ := GridLayout(nodes, []Edge{{"x", "a", "b", ""}, {"y", "a", "c", ""}})
	for i, n := range nodes {
		if !onLine(pos[i].X-n.Width/2) || !onLine(pos[i].Y-n.Height/2) || GridCenter(pos[i], n.Width, n.Height) != pos[i] {
			t.Errorf("%s at %v moves when placed", n.Key, pos[i])
		}
	}
	if pos[1].X+nodes[1].Width/2+layoutGap > pos[2].X-nodes[2].Width/2 {
		t.Fatal("blocks of one rank overlap:", pos)
	}
}

func TestAreaPictureMatchesEditorSvg(t *testing.T) {
	s := publicapi.GraphNativeSettings{Width: 200, Height: 50, BlockID: "startStop", Shape: json.RawMessage(`{"type":"primitive","primitive":[{"kind":"roundedRect","x":0,"y":0,"w":200,"h":50,"rx":25},{"kind":"line","x1":0,"y1":0,"x2":10,"y2":10}]}`)}
	p, err := AreaPicture(s, "#17B26A")
	if err != nil || p.Width != 200 || p.Height != 50 || !strings.HasPrefix(p.Img, "data:image/svg+xml;base64,") {
		t.Fatal(p, err)
	}
	svg, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(p.Img, "data:image/svg+xml;base64,"))
	want := `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="50" viewBox="0 0 200 50" preserveAspectRatio="none"><rect x="0" y="0" width="200" height="50" rx="25" ry="25" fill="#17B26A" stroke="#128e55" stroke-width="2"/><line x1="0" y1="0" x2="10" y2="10" stroke="#128e55" stroke-width="2"/></svg>`
	if string(svg) != want {
		t.Fatalf("svg\n%s\nwant\n%s", svg, want)
	}
	if _, err = AreaPicture(publicapi.GraphNativeSettings{Shape: json.RawMessage(`{"type":"ellipse"}`)}, ""); err == nil {
		t.Fatal("unsupported shape accepted")
	}
	if darken("red", 0.2) != "red" || darken("#fff", 0.5) != "#808080" || darken(" #17B26Aff ", 0.2) != "#128e55" {
		t.Fatal("darken")
	}
}

func TestFormColorIsHexOnly(t *testing.T) {
	for form, want := range map[string]string{`{"color":"#2970ff"}`: "#2970ff", `{"color":"#abc"}`: "#abc", `{"color":"blue"}`: "", `{"color":"#1973e8ff"}`: "", `{}`: ""} {
		if got := formColor(json.RawMessage(form)); got != want {
			t.Errorf("%s: %q, want %q", form, got, want)
		}
	}
}
