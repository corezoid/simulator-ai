package drawing

import (
	"bytes"
	"context"
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

// Render resolves the strokes and paints them, in order, onto a transparent raster of the box. The
// strokes' limits are checked before the raster is allocated; ctx is checked between segments.
func Render(ctx context.Context, strokes []Stroke) (Picture, error) {
	inks, box, err := resolve(strokes)
	if err != nil {
		return Picture{}, err
	}
	w := max(1, int(math.Ceil(box.W*Scale)))
	h := max(1, int(math.Ceil(box.H*Scale)))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	origin := Point{box.X - box.W/2, box.Y - box.H/2}
	toPixels := func(p Point) Point { return Point{(p.X - origin.X) * Scale, (p.Y - origin.Y) * Scale} }
	c := canvas{mask: make([]uint8, w*h), w: w, h: h}
	for _, k := range inks {
		c.dirty = image.Rectangle{}
		r := k.radius * Scale
		for _, piece := range k.pieces {
			px := make([]Point, len(piece))
			for i, p := range piece {
				px[i] = toPixels(p)
			}
			if err := c.strokePiece(ctx, px, r, k.butt); err != nil {
				return Picture{}, err
			}
		}
		for _, head := range k.heads {
			c.fillTriangle([3]Point{toPixels(head[0]), toPixels(head[1]), toPixels(head[2])})
		}
		c.composite(img, k.rgba)
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img); err != nil {
		return Picture{}, err
	}
	return Picture{PNG: out.Bytes(), Box: box, Pixels: image.Point{X: w, Y: h}}, nil
}

// canvas is one stroke's coverage: a byte per pixel, and the region it touched, which is all that
// is composited and cleared afterwards.
type canvas struct {
	mask  []uint8
	w, h  int
	dirty image.Rectangle
}

func (c *canvas) touch(x0, y0, x1, y1 int) {
	c.dirty = c.dirty.Union(image.Rect(x0, y0, x1, y1))
}

// strokePiece covers the pixels within r of the polyline (round joins; round caps, or square-cut
// ends when butt), antialiased over one pixel. Coverage is a max, so joints are not painted twice.
// Each row visits only the band the segment can reach there, so the work is the segment's length
// times its width, never its bounding box.
func (c *canvas) strokePiece(ctx context.Context, line []Point, r float64, butt bool) error {
	if len(line) == 1 {
		line = []Point{line[0], line[0]}
	}
	reach := r + 1
	last := len(line) - 2
	for i := 0; i <= last; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a, b := line[i], line[i+1]
		dx, dy := b.X-a.X, b.Y-a.Y
		l2 := dx*dx + dy*dy
		y0 := clampInt(math.Floor(math.Min(a.Y, b.Y)-reach), c.h)
		y1 := clampInt(math.Ceil(math.Max(a.Y, b.Y)+reach), c.h)
		for y := y0; y < y1; y++ {
			cy := float64(y) + 0.5
			// The part of the segment within reach of this row, and so the x it can touch.
			t0, t1 := 0.0, 1.0
			if math.Abs(dy) > 1e-12 {
				t0, t1 = (cy-reach-a.Y)/dy, (cy+reach-a.Y)/dy
				if t0 > t1 {
					t0, t1 = t1, t0
				}
				t0, t1 = math.Max(t0, 0), math.Min(t1, 1)
				if t0 > t1 {
					continue
				}
			} else if math.Abs(cy-a.Y) > reach {
				continue
			}
			xa, xb := a.X+t0*dx, a.X+t1*dx
			x0 := clampInt(math.Floor(math.Min(xa, xb)-reach), c.w)
			x1 := clampInt(math.Ceil(math.Max(xa, xb)+reach), c.w)
			if x0 >= x1 {
				continue
			}
			c.touch(x0, y, x1, y+1)
			for x := x0; x < x1; x++ {
				cx := float64(x) + 0.5
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
				c.cover(y*c.w+x, r+0.5-d)
			}
		}
	}
	return nil
}

// fillTriangle covers a filled triangle, its edges antialiased over one pixel.
func (c *canvas) fillTriangle(t [3]Point) {
	x0 := clampInt(math.Floor(math.Min(t[0].X, math.Min(t[1].X, t[2].X))-1), c.w)
	x1 := clampInt(math.Ceil(math.Max(t[0].X, math.Max(t[1].X, t[2].X))+1), c.w)
	y0 := clampInt(math.Floor(math.Min(t[0].Y, math.Min(t[1].Y, t[2].Y))-1), c.h)
	y1 := clampInt(math.Ceil(math.Max(t[0].Y, math.Max(t[1].Y, t[2].Y))+1), c.h)
	// The signed distance to each edge, positive inside whatever the winding.
	area := (t[1].X-t[0].X)*(t[2].Y-t[0].Y) - (t[1].Y-t[0].Y)*(t[2].X-t[0].X)
	if area == 0 || x0 >= x1 || y0 >= y1 {
		return
	}
	c.touch(x0, y0, x1, y1)
	sign := math.Copysign(1, area)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			p := Point{float64(x) + 0.5, float64(y) + 0.5}
			inside := math.Inf(1)
			for i := range 3 {
				a, b := t[i], t[(i+1)%3]
				l := math.Hypot(b.X-a.X, b.Y-a.Y)
				if l == 0 {
					continue
				}
				inside = math.Min(inside, sign*((b.X-a.X)*(p.Y-a.Y)-(b.Y-a.Y)*(p.X-a.X))/l)
			}
			c.cover(y*c.w+x, inside+0.5)
		}
	}
}

func (c *canvas) cover(i int, v float64) {
	if v <= 0 {
		return
	}
	if q := uint8(math.Round(math.Min(v, 1) * 255)); q > c.mask[i] {
		c.mask[i] = q
	}
}

func clampInt(v float64, n int) int {
	return int(math.Max(0, math.Min(float64(n), v)))
}

// composite paints the colour through the stroke's coverage over what is there (source-over,
// straight alpha), then clears the coverage it read.
func (c *canvas) composite(img *image.NRGBA, rgba [4]uint8) {
	for y := c.dirty.Min.Y; y < c.dirty.Max.Y; y++ {
		row := c.mask[y*c.w+c.dirty.Min.X : y*c.w+c.dirty.Max.X]
		for j, m := range row {
			if m == 0 {
				continue
			}
			row[j] = 0
			i := y*c.w + c.dirty.Min.X + j
			p := img.Pix[i*4 : i*4+4]
			da := float64(p[3]) / 255
			sa := float64(m) / 255
			oa := sa + da*(1-sa)
			for ch := range 3 {
				p[ch] = uint8(math.Round((float64(rgba[ch])*sa + float64(p[ch])*da*(1-sa)) / oa))
			}
			p[3] = uint8(math.Round(oa * 255))
		}
	}
}
