// Package drawing flattens strokes described in words into the raster a Graph Editor Pencil session
// becomes on confirm (simulator-ui-kit graph-layer engine/konva/inkSnapshot.ts): a transparent PNG
// of the ink alone at two pixels per graph unit, boxed by the strokes plus half their width
// (engine/drawPaint.ts sessionInkBox). Lines are stroked as the pencil paints them (paint/draw.ts):
// round joins and caps, butt caps on dashes, the edge dash dictionary (dashed [6, 4], dotted [0, 8]
// with round dots), width in graph units, and filled arrow heads of the arrow mark's shape.
package drawing

import (
	"errors"
	"fmt"
	"math"
	"regexp"
)

// Scale is the raster's pixels per graph unit (inkSnapshot.ts INK_SNAPSHOT_SCALE, legacy pixelRatio 2).
const Scale = 2

// The pen's bounds and default (StylePanel.tsx MIN_WIDTH, MAX_WIDTH, DEFAULT_PEN).
const (
	MinWidth     = 1
	MaxWidth     = 20
	DefaultWidth = 3
	DefaultColor = "#667085" // actorHues.slate
)

// Limits of one drawing: the raster stays a few megapixels and its PNG an upload.
const (
	MaxStrokes = 100
	MaxPoints  = 20000
	MaxSide    = 2000 // graph units, so at most 4000 px a side
)

// arrowHeadRatio and arrowHalfAngle are the arrow mark's head (style/defaults.ts arrowHeadRatio,
// drawPaint.ts MARK_ARROW_HALF_ANGLE): a triangle whose slant is 3.5 stroke widths.
const (
	arrowHeadRatio = 3.5
	arrowHalfAngle = 0.42
)

// dash patterns are the edge dictionary's (style/defaults.ts edge.dash), in graph units at zoom 1.
var dashes = map[string][]float64{"dashed": {6, 4}, "dotted": {0, 8}}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Stroke is one line of the drawing, in graph units (x right, y down).
type Stroke struct {
	// Kind is line (through Points, default), rect or ellipse (Center, W, H).
	Kind   string  `json:"kind,omitempty"`
	Points []Point `json:"points,omitempty"`
	Center *Point  `json:"center,omitempty"`
	W      float64 `json:"w,omitempty"`
	H      float64 `json:"h,omitempty"`
	Color  string  `json:"color,omitempty"`
	Width  float64 `json:"width,omitempty"`
	// Style is solid (default), dashed or dotted.
	Style string `json:"style,omitempty"`
	// Smooth bends a line through its points instead of joining them straight; Closed joins the
	// last point back to the first.
	Smooth     bool `json:"smooth,omitempty"`
	Closed     bool `json:"closed,omitempty"`
	ArrowStart bool `json:"arrowStart,omitempty"`
	ArrowEnd   bool `json:"arrowEnd,omitempty"`
}

// Box is the world box of the ink: its centre and size, the confirmed picture's placement.
type Box struct {
	X, Y, W, H float64
}

// ink is one stroke resolved to what is painted: dash pieces of a polyline and filled heads.
type ink struct {
	rgba   [4]uint8
	radius float64
	butt   bool // dashed: the dash ends are cut square
	pieces [][]Point
	heads  [][3]Point
}

// resolve checks the strokes and turns each into ink; the returned box is the union of every
// polyline padded by half its width and of every head.
func resolve(strokes []Stroke) ([]ink, Box, error) {
	if len(strokes) == 0 || len(strokes) > MaxStrokes {
		return nil, Box{}, fmt.Errorf("1..%d strokes required", MaxStrokes)
	}
	total := 0
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	grow := func(p Point, pad float64) {
		minX, minY = math.Min(minX, p.X-pad), math.Min(minY, p.Y-pad)
		maxX, maxY = math.Max(maxX, p.X+pad), math.Max(maxY, p.Y+pad)
	}
	out := make([]ink, 0, len(strokes))
	for i, s := range strokes {
		line, err := outline(s)
		if err != nil {
			return nil, Box{}, fmt.Errorf("stroke %d: %w", i, err)
		}
		if total += len(line); total > MaxPoints {
			return nil, Box{}, fmt.Errorf("more than %d points in all (curves count their segments): draw fewer or simpler strokes", MaxPoints)
		}
		width := s.Width
		if width == 0 {
			width = DefaultWidth
		}
		if width < MinWidth || width > MaxWidth {
			return nil, Box{}, fmt.Errorf("stroke %d: width %v outside %d..%d graph units", i, width, MinWidth, MaxWidth)
		}
		color := s.Color
		if color == "" {
			color = DefaultColor
		}
		if !hexColor.MatchString(color) {
			return nil, Box{}, fmt.Errorf("stroke %d: color must be #RRGGBB", i)
		}
		pattern, dashed := dashes[s.Style]
		if s.Style != "" && s.Style != "solid" && !dashed {
			return nil, Box{}, fmt.Errorf("stroke %d: style is solid, dashed or dotted", i)
		}
		k := ink{rgba: rgbaOf(color), radius: width / 2, butt: s.Style == "dashed", pieces: [][]Point{line}}
		if dashed {
			k.pieces = dash(line, pattern)
		}
		if s.ArrowEnd {
			k.heads = appendHead(k.heads, line, len(line)-1, -1, width)
		}
		if s.ArrowStart {
			k.heads = appendHead(k.heads, line, 0, 1, width)
		}
		for _, p := range line {
			grow(p, width/2)
		}
		for _, h := range k.heads {
			for _, p := range h {
				grow(p, 0)
			}
		}
		out = append(out, k)
	}
	box := Box{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, W: maxX - minX, H: maxY - minY}
	if box.W > MaxSide || box.H > MaxSide {
		return nil, Box{}, fmt.Errorf("the drawing spans %.0f x %.0f graph units, over %d: draw it smaller or in parts", box.W, box.H, MaxSide)
	}
	return out, box, nil
}

// outline is the stroke's centre line as a polyline of two or more points.
func outline(s Stroke) ([]Point, error) {
	for _, p := range s.Points {
		if !finite(p.X) || !finite(p.Y) {
			return nil, errors.New("points must be finite numbers")
		}
	}
	switch s.Kind {
	case "", "line":
		if len(s.Points) < 2 || s.Center != nil {
			return nil, errors.New("a line needs two or more points")
		}
		line := s.Points
		if s.Closed {
			line = append(append([]Point{}, line...), line[0])
		}
		if s.Smooth {
			line = catmullRom(line, s.Closed)
		}
		if travel(line) == 0 {
			return nil, errors.New("every point of the line is the same: a mark needs some length")
		}
		return line, nil
	case "rect", "ellipse":
		if s.Center == nil || len(s.Points) > 0 || !finite(s.Center.X) || !finite(s.Center.Y) || !(s.W > 0) || !(s.H > 0) || !finite(s.W) || !finite(s.H) {
			return nil, errors.New("a rect or ellipse needs center {x, y} and positive w and h, no points")
		}
		c, hw, hh := *s.Center, s.W/2, s.H/2
		if s.Kind == "rect" {
			return []Point{{c.X - hw, c.Y - hh}, {c.X + hw, c.Y - hh}, {c.X + hw, c.Y + hh}, {c.X - hw, c.Y + hh}, {c.X - hw, c.Y - hh}}, nil
		}
		n := int(math.Min(256, math.Max(24, math.Pi*(s.W+s.H)/4)))
		line := make([]Point, n+1)
		for i := 0; i <= n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			line[i] = Point{c.X + hw*math.Cos(a), c.Y + hh*math.Sin(a)}
		}
		return line, nil
	}
	return nil, errors.New("kind is line, rect or ellipse")
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func travel(line []Point) float64 {
	d := 0.0
	for i := 1; i < len(line); i++ {
		d += math.Hypot(line[i].X-line[i-1].X, line[i].Y-line[i-1].Y)
	}
	return d
}

// catmullRom bends the line through every point (uniform Catmull-Rom), a segment about every two
// graph units; a closed line wraps its neighbours around.
func catmullRom(line []Point, closed bool) []Point {
	n := len(line)
	at := func(i int) Point {
		switch {
		case closed && i < 0:
			return line[n-2]
		case closed && i >= n:
			return line[1]
		case i < 0:
			return line[0]
		case i >= n:
			return line[n-1]
		}
		return line[i]
	}
	out := []Point{line[0]}
	for i := 0; i < n-1; i++ {
		p0, p1, p2, p3 := at(i-1), line[i], line[i+1], at(i+2)
		steps := int(math.Min(64, math.Max(1, math.Hypot(p2.X-p1.X, p2.Y-p1.Y)/2)))
		for k := 1; k <= steps; k++ {
			t := float64(k) / float64(steps)
			t2, t3 := t*t, t*t*t
			f := func(a, b, c, d float64) float64 {
				return 0.5 * (2*b + (c-a)*t + (2*a-5*b+4*c-d)*t2 + (3*b-a-3*c+d)*t3)
			}
			out = append(out, Point{f(p0.X, p1.X, p2.X, p3.X), f(p0.Y, p1.Y, p2.Y, p3.Y)})
		}
	}
	return out
}

// dash cuts the line into the pattern's "on" pieces along its length; a zero-length piece is a dot.
func dash(line []Point, pattern []float64) [][]Point {
	var pieces [][]Point
	var cur []Point
	on, left := true, pattern[0]
	phase := 0
	for i := 1; i < len(line); i++ {
		a, b := line[i-1], line[i]
		seg := math.Hypot(b.X-a.X, b.Y-a.Y)
		pos := 0.0
		for seg-pos > 1e-9 || left == 0 {
			at := func(d float64) Point { return Point{a.X + (b.X-a.X)*d/seg, a.Y + (b.Y-a.Y)*d/seg} }
			if on && cur == nil {
				cur = []Point{at(pos)}
			}
			step := math.Min(left, seg-pos)
			pos += step
			left -= step
			if on {
				cur = append(cur, at(pos))
			}
			if left > 1e-9 {
				break
			}
			if on {
				pieces = append(pieces, cur)
				cur = nil
			}
			phase = (phase + 1) % len(pattern)
			on, left = phase%2 == 0, pattern[phase]
			if seg-pos <= 1e-9 && left > 0 {
				break
			}
		}
	}
	if len(cur) > 0 {
		pieces = append(pieces, cur)
	}
	return pieces
}

// appendHead adds a head at line[tip] pointing away from the nearest other point (drawPaint.ts
// headAt); a line whose points all coincide with the tip gets none.
func appendHead(heads [][3]Point, line []Point, tip, step int, width float64) [][3]Point {
	t := line[tip]
	for at := tip + step; at >= 0 && at < len(line); at += step {
		from := line[at]
		if from == t {
			continue
		}
		angle := math.Atan2(t.Y-from.Y, t.X-from.X)
		size := width * arrowHeadRatio
		length, half := size*math.Cos(arrowHalfAngle), size*math.Sin(arrowHalfAngle)
		back := Point{t.X - length*math.Cos(angle), t.Y - length*math.Sin(angle)}
		nx, ny := -math.Sin(angle)*half, math.Cos(angle)*half
		return append(heads, [3]Point{t, {back.X + nx, back.Y + ny}, {back.X - nx, back.Y - ny}})
	}
	return heads
}

func rgbaOf(hex string) [4]uint8 {
	var v [3]uint8
	for i := range v {
		_, _ = fmt.Sscanf(hex[1+2*i:3+2*i], "%02x", &v[i])
	}
	return [4]uint8{v[0], v[1], v[2], 255}
}
