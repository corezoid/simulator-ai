package drawing

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// zigzag is a line of n points bouncing between two corners of a square of the given side.
func zigzag(n int, side float64) []Point {
	pts := make([]Point, n)
	for i := range pts {
		if i%2 == 1 {
			pts[i] = Point{side, side}
		}
	}
	return pts
}

func refusedFast(t *testing.T, strokes []Stroke, want string) {
	t.Helper()
	start := time.Now()
	_, err := Render(context.Background(), strokes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("%v, want %q", err, want)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("refused after %v: the limit must hold before any painting", d)
	}
}

// A long zigzag was hours of painting (every segment's bounding box): the line length is refused
// before anything is cut or painted, solid or dotted.
func TestInkLengthIsBounded(t *testing.T) {
	refusedFast(t, []Stroke{{Points: zigzag(MaxStrokePoints, 1000), Width: MaxWidth}}, "graph units of line")
	refusedFast(t, []Stroke{{Points: zigzag(MaxStrokePoints, 1000), Style: "dotted"}}, "graph units of line")
	var many []Stroke
	for range 30 {
		many = append(many, Stroke{Points: zigzag(200, 1000), Style: "dashed"})
	}
	refusedFast(t, many, "graph units of line")
}

func TestPointsAreBoundedBeforeSmoothing(t *testing.T) {
	refusedFast(t, []Stroke{{Points: make([]Point, MaxStrokePoints+1)}}, "more than 2000 points")
	var many []Stroke
	for range 11 {
		many = append(many, Stroke{Points: zigzag(MaxStrokePoints, 10)})
	}
	refusedFast(t, many, "more than 20000 points in all")
	// 1000 long segments, each smoothed into 64: the curve's points are counted too.
	refusedFast(t, []Stroke{{Points: zigzag(1000, 30), Smooth: true}}, "curves count their segments")
}

func TestAreaIsBoundedBeforeTheRaster(t *testing.T) {
	refusedFast(t, []Stroke{{Points: []Point{{0, 0}, {1500, 1500}}}}, "square units")
	refusedFast(t, []Stroke{{Points: []Point{{0, 0}, {MaxSide + 10, 0}}}}, "draw it smaller")
}

// The most the limits allow still paints in well under a second or two: each row of a segment
// visits only its band.
func TestWorstAllowedDrawingIsQuick(t *testing.T) {
	side := math.Sqrt(MaxArea) - 2*MaxWidth
	per := int(MaxInk / (side * math.Sqrt2))
	var strokes []Stroke
	for _, style := range []string{"solid", "dotted"} {
		strokes = append(strokes, Stroke{Points: zigzag(per/2, side), Width: MaxWidth, Style: style})
	}
	start := time.Now()
	p, err := Render(context.Background(), strokes)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
	t.Logf("%v px in %v, %d KiB", p.Pixels, time.Since(start), len(p.PNG)>>10)
}

func TestRenderStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, []Stroke{{Points: []Point{{0, 0}, {10, 10}}}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// A repeated point has no length to measure a dash along: it is skipped, never a NaN.
func TestDashSkipsRepeatedPoints(t *testing.T) {
	for _, style := range []string{"dashed", "dotted"} {
		pieces := dash([]Point{{0, 0}, {0, 0}, {10, 0}, {10, 0}}, dashes[style])
		if len(pieces) == 0 {
			t.Fatal(style, "no pieces")
		}
		for _, piece := range pieces {
			for _, p := range piece {
				if math.IsNaN(p.X) || math.IsNaN(p.Y) {
					t.Fatal(style, pieces)
				}
			}
		}
	}
}
