package publicapi

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// A cell is the address the Graph Editor rulers give a point, ported from simulator-ui-kit
// utils/src/graphCoordinates.ts (toCellCoord / toGeomCoord, legacy's arithmetic): a column name and
// a row number, written "C, 7". Cells are 50 graph units; the cell named by an address is centred on
// a multiple of 50, so its grid lines lie at 25 + 50k. Columns run A…Z, then A1…Z1, A2… (a plain
// block suffix, not Excel's AA), and -A, -B… left of the origin; rows run 1, 2… down from it and
// -1, -2… up, with no column or row 0. The rulers are absolute: an address does not move with the
// viewport.

const gridCell = 50.0

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// jsRound is JavaScript's Math.round: halves round up, toward +∞.
func jsRound(v float64) float64 { return math.Floor(v + 0.5) }

func encodeColumn(index int) string {
	block := index / len(alphabet)
	if block == 0 {
		return alphabet[index : index+1]
	}
	return alphabet[index%len(alphabet):index%len(alphabet)+1] + strconv.Itoa(block)
}

// CellOf names the cell holding p, as the Graph Editor writes it beside a node: "C, 7".
func CellOf(p GraphPoint) string {
	column := int(jsRound(math.Abs(p.X) / gridCell))
	negative := p.X < 0
	if negative {
		column--
	}
	if column < 0 { // x in (-25, 0) lies inside A; legacy JS printed "-undefined" here
		column, negative = 0, false
	}
	row := int(jsRound(p.Y / gridCell))
	if p.Y >= 0 {
		row++
	}
	if row == 0 { // y in (-25, 0) lies inside row 1
		row = 1
	}
	name := encodeColumn(column)
	if negative {
		name = "-" + name
	}
	return name + ", " + strconv.Itoa(row)
}

var cellPattern = regexp.MustCompile(`^(-?)([A-Za-z])(\d*)\s*(?:,\s*|\s+)(-?\d+)$`)

// errCell explains the address, since "C7" reads as column C7 on the rulers, not C row 7.
var errCell = errors.New(`a cell is "<column>, <row>" as on the Graph Editor rulers, e.g. "C, 7" (column C, row 7) or "-B1, -3"; columns run A…Z, A1…Z1, A2…, and rows have no 0`)

// CellCenter is the centre, in graph units, of the cell named "C, 7"; the inverse of CellOf on
// whole-cell points.
func CellCenter(cell string) (GraphPoint, error) {
	m := cellPattern.FindStringSubmatch(strings.TrimSpace(cell))
	if m == nil {
		return GraphPoint{}, fmt.Errorf("%q: %w", cell, errCell)
	}
	block := 0
	if m[3] != "" {
		var err error
		if block, err = strconv.Atoi(m[3]); err != nil || block > 1_000_000 {
			return GraphPoint{}, fmt.Errorf("%q: %w", cell, errCell)
		}
	}
	row, err := strconv.Atoi(m[4])
	if err != nil || row == 0 || row > 1_000_000 || row < -1_000_000 {
		return GraphPoint{}, fmt.Errorf("%q: %w", cell, errCell)
	}
	column := float64(strings.IndexByte(alphabet, strings.ToUpper(m[2])[0]) + block*len(alphabet))
	p := GraphPoint{X: column * gridCell, Y: float64(row) * gridCell}
	if m[1] == "-" {
		p.X = -column*gridCell - gridCell
	}
	if row > 0 {
		p.Y -= gridCell
	}
	return p, nil
}
