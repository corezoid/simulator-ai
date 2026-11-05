package drawing

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math"
	"strings"
	"testing"
)

func decode(t *testing.T, p Picture) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(p.PNG))
	if err != nil {
		t.Fatal(err)
	}
	n, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("decoded %T", img)
	}
	return n
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The box is the ink's extent padded by half the width, centred, as sessionInkBox boxes a session,
// and the raster is two pixels per unit of it.
func TestRenderBoxAndInk(t *testing.T) {
	p, err := Render(context.Background(), []Stroke{{Points: []Point{{100, 50}, {200, 50}}, Width: 4, Color: "#ff0000"}})
	if err != nil {
		t.Fatal(err)
	}
	if !near(p.Box.X, 150) || !near(p.Box.Y, 50) || !near(p.Box.W, 104) || !near(p.Box.H, 4) || p.Pixels != (image.Point{208, 8}) {
		t.Fatalf("box %+v pixels %v", p.Box, p.Pixels)
	}
	img := decode(t, p)
	if c := img.NRGBAAt(104, 4); c.R != 255 || c.G != 0 || c.A != 255 {
		t.Fatalf("centre pixel %v", c)
	}
	// Round caps: the corners of the box stay clear.
	if c := img.NRGBAAt(0, 0); c.A != 0 {
		t.Fatalf("corner pixel %v", c)
	}
}

func TestDefaultsAreThePen(t *testing.T) {
	p, err := Render(context.Background(), []Stroke{{Points: []Point{{0, 0}, {10, 0}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !near(p.Box.H, DefaultWidth) {
		t.Fatalf("box %+v", p.Box)
	}
	if c := decode(t, p).NRGBAAt(10, 3); c.R != 0x66 || c.G != 0x70 || c.B != 0x85 || c.A != 255 {
		t.Fatalf("slate expected, got %v", c)
	}
}

func TestDashPieces(t *testing.T) {
	line := []Point{{0, 0}, {20, 0}}
	dashed := dash(line, dashes["dashed"])
	if len(dashed) != 2 || !near(dashed[0][len(dashed[0])-1].X, 6) || !near(dashed[1][0].X, 10) || !near(dashed[1][len(dashed[1])-1].X, 16) {
		t.Fatalf("dashed %v", dashed)
	}
	dots := dash([]Point{{0, 0}, {10, 0}, {10, 14}}, dashes["dotted"])
	// A dot every 8 units along the bend: at 0, 8, 16 (6 units down the second segment) and 24, its end.
	if len(dots) != 4 || !near(dots[1][0].X, 8) || !near(dots[2][0].X, 10) || !near(dots[2][0].Y, 6) {
		t.Fatalf("dots %v", dots)
	}
	p, err := Render(context.Background(), []Stroke{{Points: line, Style: "dashed", Width: 2}})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, p)
	// Inside the first dash, then in its gap (x 6..10 units, i.e. 16..20 px with the 1-unit pad).
	if img.NRGBAAt(6, 2).A != 255 || img.NRGBAAt(18, 2).A != 0 {
		t.Fatalf("dash %v gap %v", img.NRGBAAt(6, 2), img.NRGBAAt(18, 2))
	}
}

func TestArrowHeadsGrowTheBox(t *testing.T) {
	p, err := Render(context.Background(), []Stroke{{Points: []Point{{0, 0}, {100, 0}}, Width: 2, ArrowEnd: true}})
	if err != nil {
		t.Fatal(err)
	}
	half := 2 * arrowHeadRatio * math.Sin(arrowHalfAngle)
	if !near(p.Box.H, 2*half) {
		t.Fatalf("box %+v, want height %v", p.Box, 2*half)
	}
	img := decode(t, p)
	// Near the head's back, well off the shaft: inside the head at the end, clear at the start.
	origin := Point{p.Box.X - p.Box.W/2, p.Box.Y - p.Box.H/2}
	px := func(x, y float64) (int, int) { return int((x - origin.X) * Scale), int((y - origin.Y) * Scale) }
	x, y := px(94, -2.4)
	if img.NRGBAAt(x, y).A == 0 {
		t.Fatal("no head at the end")
	}
	if x, y = px(6, -2.4); img.NRGBAAt(x, y).A != 0 {
		t.Fatal("a head at the start")
	}
}

func TestShapes(t *testing.T) {
	p, err := Render(context.Background(), []Stroke{{Kind: "rect", Center: &Point{10, 10}, W: 40, H: 20, Width: 2}, {Kind: "ellipse", Center: &Point{10, 10}, W: 10, H: 10, Width: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !near(p.Box.X, 10) || !near(p.Box.W, 42) || !near(p.Box.H, 22) {
		t.Fatalf("box %+v", p.Box)
	}
	img := decode(t, p)
	if img.NRGBAAt(2, 22).A == 0 || img.NRGBAAt(20, 22).A != 0 {
		t.Fatal("rect outline expected on its edge, not inside")
	}
}

func TestSmoothPassesThroughPoints(t *testing.T) {
	pts := []Point{{0, 0}, {50, 40}, {100, 0}}
	line := catmullRom(pts, false)
	if len(line) < 20 || line[0] != pts[0] || line[len(line)-1] != pts[2] {
		t.Fatalf("%d points, ends %v %v", len(line), line[0], line[len(line)-1])
	}
	hit := false
	for _, p := range line {
		hit = hit || near(p.X, 50) && near(p.Y, 40)
	}
	if !hit {
		t.Fatal("the curve misses its middle point")
	}
}

func TestRefusals(t *testing.T) {
	for _, tc := range []struct {
		strokes []Stroke
		want    string
	}{
		{nil, "strokes required"},
		{[]Stroke{{Points: []Point{{0, 0}}}}, "two or more points"},
		{[]Stroke{{Points: []Point{{1, 1}, {1, 1}}}}, "some length"},
		{[]Stroke{{Points: []Point{{0, 0}, {1, 1}}, Width: 21}}, "width"},
		{[]Stroke{{Points: []Point{{0, 0}, {1, 1}}, Color: "red"}}, "#RRGGBB"},
		{[]Stroke{{Points: []Point{{0, 0}, {1, 1}}, Style: "wavy"}}, "style"},
		{[]Stroke{{Kind: "rect", W: 1, H: 1}}, "center"},
		{[]Stroke{{Kind: "star", Center: &Point{0, 0}, W: 1, H: 1}}, "kind"},
		{[]Stroke{{Points: []Point{{0, 0}, {MaxSide + 1, 0}}}}, "draw it smaller"},
		{[]Stroke{{Points: []Point{{0, 0}, {math.NaN(), 0}}}}, "finite"},
	} {
		if _, err := Render(context.Background(), tc.strokes); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: %v, want %q", tc.strokes, err, tc.want)
		}
	}
}
