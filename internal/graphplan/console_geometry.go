package graphplan

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// Graph Editor geometry (UI Kit geometry/grid.ts, tools/diagram.ts): a cell is 50 units, grid lines
// lie on cell boundaries at 25 + 50k, and a placement position is the centre of its box. A flowchart
// block is "border snapped": its top-left corner sits on a grid line, so a block whose sides are
// whole cells fills whole cells. Such a placement must be graphic (areaPicture), or sim-api snaps
// the centre to a multiple of 50 and an even-sized block lands half a cell off the grid.
const (
	gridCell  = 50.0
	gridPhase = gridCell / 2
	// layoutGap separates blocks and ranks: three cells leave room for an edge label.
	layoutGap = 3 * gridCell
	// defaultBlockColor is the editor's flowchart palette default.
	defaultBlockColor = "#539fdf"
)

// gridLine is the grid line nearest to v; an exact half cell goes up, as JS Math.round does in the
// editor's snap (Go math.Round would go away from zero and differ for negative coordinates).
func gridLine(v float64) float64 {
	return math.Floor((v-gridPhase)/gridCell+0.5)*gridCell + gridPhase
}

// gridLineAfter is the first grid line at or after v.
func gridLineAfter(v float64) float64 {
	return math.Ceil((v-gridPhase)/gridCell)*gridCell + gridPhase
}

// GridCenter moves a w×h box centred at p so its top-left corner is on the nearest grid line.
func GridCenter(p publicapi.GraphPoint, w, h float64) publicapi.GraphPoint {
	return publicapi.GraphPoint{X: gridLine(p.X-w/2) + w/2, Y: gridLine(p.Y-h/2) + h/2}
}

// AreaPicture is the legacy flowchart silhouette (UI Kit flowchartShapeSvg / legacy
// createAreaPictureByShape) the editor writes with a block placement: fill = color, stroke = the
// color darkened by a fifth, 2-unit stroke, the block's own box as viewBox.
func AreaPicture(s publicapi.GraphNativeSettings, color string) (publicapi.GraphAreaPicture, error) {
	var shape shapeDocument
	if json.Unmarshal(s.Shape, &shape) != nil || !validShape(s.Shape) {
		return publicapi.GraphAreaPicture{}, fmt.Errorf("flowchart block %s has an unsupported shape", s.BlockID)
	}
	if color == "" {
		color = defaultBlockColor
	}
	fill, stroke := xmlAttr(color), xmlAttr(darken(color, 0.2))
	num := func(v *float64) string { return strconv.FormatFloat(*v, 'f', -1, 64) }
	paint := fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="2"`, fill, stroke)
	var body strings.Builder
	if shape.Type == "path" {
		fmt.Fprintf(&body, `<path d="%s" %s/>`, xmlAttr(shape.D), paint)
	}
	for _, p := range shape.Primitive {
		switch p.Kind {
		case "rect":
			fmt.Fprintf(&body, `<rect x="%s" y="%s" width="%s" height="%s" %s/>`, num(p.X), num(p.Y), num(p.W), num(p.H), paint)
		case "roundedRect":
			rx, ry := 0.0, 0.0
			if p.RX != nil {
				rx, ry = *p.RX, *p.RX
			}
			if p.RY != nil {
				ry = *p.RY
			}
			fmt.Fprintf(&body, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s" ry="%s" %s/>`, num(p.X), num(p.Y), num(p.W), num(p.H), num(&rx), num(&ry), paint)
		case "line":
			fmt.Fprintf(&body, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="2"/>`, num(p.X1), num(p.Y1), num(p.X2), num(p.Y2), stroke)
		case "path":
			fmt.Fprintf(&body, `<path d="%s" %s/>`, xmlAttr(p.D), paint)
		}
	}
	w, h := strconv.FormatFloat(s.Width, 'f', -1, 64), strconv.FormatFloat(s.Height, 'f', -1, 64)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" preserveAspectRatio="none">%s</svg>`, w, h, w, h, body.String())
	return publicapi.GraphAreaPicture{Width: s.Width, Height: s.Height, Img: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))}, nil
}

// xmlAttr keeps the SVG ASCII and the value inside its attribute.
func xmlAttr(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r > 0x7f:
			fmt.Fprintf(&b, "&#%d;", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// darken pulls a #rgb/#rrggbb colour toward black; anything else is returned unchanged.
func darken(color string, amount float64) string {
	trimmed := strings.TrimSpace(color)
	hex := strings.TrimPrefix(trimmed, "#")
	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 8:
		hex = hex[:6] // the editor writes the darkened stroke opaque
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if !strings.HasPrefix(trimmed, "#") || len(hex) != 6 || err != nil {
		return color
	}
	keep := 1 - math.Min(1, amount)
	ch := func(shift uint) int { return int(math.Round(float64((v>>shift)&0xff) * keep)) }
	return fmt.Sprintf("#%02x%02x%02x", ch(16), ch(8), ch(0))
}

// LayoutNode is one block of a console flowchart layout; Width/Height come from its catalog kind.
type LayoutNode struct {
	Key    string
	Width  float64
	Height float64
}

// GridLayout ranks nodes by longest path (return links excluded, as in Build) and lays the ranks
// top to bottom, each rank centred on x = 0, with layoutGap between blocks and ranks. Every block's
// corner is on a grid line, so placements keep exactly these centres. Same input, same layout.
func GridLayout(nodes []LayoutNode, edges []Edge) ([]publicapi.GraphPoint, []string) {
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
	rows := 0
	for _, r := range rank {
		rows = max(rows, r+1)
	}
	heights, widths := make([]float64, rows), make([]float64, rows)
	for i, n := range nodes {
		heights[rank[i]] = max(heights[rank[i]], n.Height)
		if widths[rank[i]] > 0 {
			widths[rank[i]] += layoutGap
		}
		widths[rank[i]] += n.Width
	}
	// Every corner is a grid line even for block sizes that are not whole cells: advances round up
	// to the next line, so placements keep exactly these centres.
	out := make([]publicapi.GraphPoint, len(nodes))
	top := gridPhase
	for r := range rows {
		x := gridLine(-widths[r] / 2)
		for i, n := range nodes {
			if rank[i] != r {
				continue
			}
			// Blocks of one rank share a centre line when the height difference allows it.
			y := top + math.Floor((heights[r]-n.Height)/2/gridCell)*gridCell
			out[i] = publicapi.GraphPoint{X: x + n.Width/2, Y: y + n.Height/2}
			x = gridLineAfter(x + n.Width + layoutGap)
		}
		top = gridLineAfter(top + heights[r] + layoutGap)
	}
	return out, returns
}
