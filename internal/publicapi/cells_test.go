package publicapi

import "testing"

// The cases of simulator-ui-kit utils/src/__tests__/graphCoordinates.test.ts: the recorded
// legacy addresses first, then the column blocks, the missing zeros and the half-cell bands.
func TestCellOfMatchesTheRulers(t *testing.T) {
	for _, tc := range []struct {
		p    GraphPoint
		cell string
	}{
		{GraphPoint{X: -100, Y: -50}, "-B, -1"},
		{GraphPoint{X: 100, Y: 100}, "C, 3"},
		{GraphPoint{X: 250, Y: -200}, "F, -4"},
		{GraphPoint{X: 550, Y: 50}, "L, 2"},
		{GraphPoint{X: 550, Y: 250}, "L, 6"},
		{GraphPoint{X: 0, Y: 0}, "A, 1"},
		{GraphPoint{X: -50, Y: 0}, "-A, 1"},
		{GraphPoint{X: 25 * 50, Y: 0}, "Z, 1"},
		{GraphPoint{X: 26 * 50, Y: 0}, "A1, 1"},
		{GraphPoint{X: 27 * 50, Y: 0}, "B1, 1"},
		{GraphPoint{X: 52 * 50, Y: 0}, "A2, 1"},
		{GraphPoint{X: -26 * 50, Y: 0}, "-Z, 1"},
		{GraphPoint{X: -27 * 50, Y: 0}, "-A1, 1"},
		{GraphPoint{X: -1, Y: -1}, "A, 1"},
		{GraphPoint{X: -24, Y: -24}, "A, 1"},
		{GraphPoint{X: -25, Y: 0}, "-A, 1"},
		{GraphPoint{X: 0, Y: -25}, "A, 1"},
		{GraphPoint{X: 49, Y: 0}, "B, 1"},
		{GraphPoint{X: 0, Y: 50}, "A, 2"},
		{GraphPoint{X: 0, Y: -50}, "A, -1"},
	} {
		if got := CellOf(tc.p); got != tc.cell {
			t.Errorf("CellOf(%v) = %q, want %q", tc.p, got, tc.cell)
		}
	}
}

func TestCellCenterInvertsCellOf(t *testing.T) {
	for x := -60; x <= 60; x++ {
		for _, y := range []int{-3, -1, 0, 1, 7} {
			p := GraphPoint{X: float64(x * 50), Y: float64(y * 50)}
			back, err := CellCenter(CellOf(p))
			if err != nil || back != p {
				t.Fatalf("%v -> %q -> %v, %v", p, CellOf(p), back, err)
			}
		}
	}
	for cell, want := range map[string]GraphPoint{"C, 7": {X: 100, Y: 300}, "c,7": {X: 100, Y: 300}, "C 7": {X: 100, Y: 300}, " -B1, -3 ": {X: -1400, Y: -150}} {
		if got, err := CellCenter(cell); err != nil || got != want {
			t.Errorf("CellCenter(%q) = %v, %v; want %v", cell, got, err, want)
		}
	}
	for _, bad := range []string{"C7", "C, 0", "AB, 1", "", "-, 1", "4, 1", "C, x", "C, 7, 1"} {
		if _, err := CellCenter(bad); err == nil {
			t.Errorf("CellCenter(%q) accepted", bad)
		}
	}
}
