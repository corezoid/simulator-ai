package drawing

import (
	"bytes"
	"image"
	"image/png"
	"math"
)

// Picture is a flattened drawing: the PNG and the world box it is placed in.
type Picture struct {
	PNG    []byte
	Box    Box
	Pixels image.Point // the PNG's width and height
}

// Render resolves the strokes and paints them, in order, onto a transparent raster of the box.
func Render(strokes []Stroke) (Picture, error) {
	inks, box, err := resolve(strokes)
	if err != nil {
		return Picture{}, err
	}
	w := max(1, int(math.Ceil(box.W*Scale)))
	h := max(1, int(math.Ceil(box.H*Scale)))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	origin := Point{box.X - box.W/2, box.Y - box.H/2}
	toPixels := func(p Point) Point { return Point{(p.X - origin.X) * Scale, (p.Y - origin.Y) * Scale} }
	mask := make([]float32, w*h)
	for _, k := range inks {
		clear(mask)
		r := k.radius * Scale
		for _, piece := range k.pieces {
			px := make([]Point, len(piece))
			for i, p := range piece {
				px[i] = toPixels(p)
			}
			strokePiece(mask, w, h, px, r, k.butt)
		}
		for _, head := range k.heads {
			fillTriangle(mask, w, h, [3]Point{toPixels(head[0]), toPixels(head[1]), toPixels(head[2])})
		}
		composite(img, mask, k.rgba)
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img); err != nil {
		return Picture{}, err
	}
	return Picture{PNG: out.Bytes(), Box: box, Pixels: image.Point{X: w, Y: h}}, nil
}

// strokePiece covers the pixels within r of the polyline (round joins; round caps, or square-cut
// ends when butt), antialiased over one pixel. Coverage is a max, so joints are not painted twice.
func strokePiece(mask []float32, w, h int, line []Point, r float64, butt bool) {
	if len(line) == 1 {
		line = []Point{line[0], line[0]}
	}
	last := len(line) - 2
	for i := 0; i <= last; i++ {
		a, b := line[i], line[i+1]
		x0, x1 := clampInt(math.Floor(math.Min(a.X, b.X)-r-1), w), clampInt(math.Ceil(math.Max(a.X, b.X)+r+1), w)
		y0, y1 := clampInt(math.Floor(math.Min(a.Y, b.Y)-r-1), h), clampInt(math.Ceil(math.Max(a.Y, b.Y)+r+1), h)
		dx, dy := b.X-a.X, b.Y-a.Y
		l2 := dx*dx + dy*dy
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				cx, cy := float64(x)+0.5, float64(y)+0.5
				t := 0.0
				if l2 > 0 {
					t = ((cx-a.X)*dx + (cy-a.Y)*dy) / l2
				}
				if butt && (i == 0 && t < 0 || i == last && t > 1) {
					// Past a dash's end: only the perpendicular band up to the cut, softened by a pixel.
					along := t * math.Sqrt(l2)
					if t > 1 {
						along = (t - 1) * math.Sqrt(l2)
					}
					if math.Abs(along) >= 0.5 {
						continue
					}
				}
				tc := math.Max(0, math.Min(1, t))
				d := math.Hypot(cx-(a.X+tc*dx), cy-(a.Y+tc*dy))
				cover(mask, y*w+x, r+0.5-d)
			}
		}
	}
}

// fillTriangle covers a filled triangle, its edges antialiased over one pixel.
func fillTriangle(mask []float32, w, h int, t [3]Point) {
	x0 := clampInt(math.Floor(math.Min(t[0].X, math.Min(t[1].X, t[2].X))-1), w)
	x1 := clampInt(math.Ceil(math.Max(t[0].X, math.Max(t[1].X, t[2].X))+1), w)
	y0 := clampInt(math.Floor(math.Min(t[0].Y, math.Min(t[1].Y, t[2].Y))-1), h)
	y1 := clampInt(math.Ceil(math.Max(t[0].Y, math.Max(t[1].Y, t[2].Y))+1), h)
	// The signed distance to each edge, positive inside whatever the winding.
	area := (t[1].X-t[0].X)*(t[2].Y-t[0].Y) - (t[1].Y-t[0].Y)*(t[2].X-t[0].X)
	if area == 0 {
		return
	}
	sign := math.Copysign(1, area)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := Point{float64(x) + 0.5, float64(y) + 0.5}
			inside := math.Inf(1)
			for i := range 3 {
				a, b := t[i], t[(i+1)%3]
				l := math.Hypot(b.X-a.X, b.Y-a.Y)
				if l == 0 {
					continue
				}
				inside = math.Min(inside, sign*((b.X-a.X)*(c.Y-a.Y)-(b.Y-a.Y)*(c.X-a.X))/l)
			}
			cover(mask, y*w+x, inside+0.5)
		}
	}
}

func cover(mask []float32, i int, v float64) {
	if v <= 0 {
		return
	}
	if v > 1 {
		v = 1
	}
	if float32(v) > mask[i] {
		mask[i] = float32(v)
	}
}

func clampInt(v float64, n int) int {
	return int(math.Max(0, math.Min(float64(n), v)))
}

// composite paints the colour through the mask over what is there (source-over, straight alpha).
func composite(img *image.NRGBA, mask []float32, rgba [4]uint8) {
	for i, a := range mask {
		if a == 0 {
			continue
		}
		p := img.Pix[i*4 : i*4+4]
		da := float64(p[3]) / 255
		sa := float64(a)
		oa := sa + da*(1-sa)
		for c := range 3 {
			v := (float64(rgba[c])*sa + float64(p[c])*da*(1-sa)) / oa
			p[c] = uint8(math.Round(v))
		}
		p[3] = uint8(math.Round(oa * 255))
	}
}
