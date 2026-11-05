package graphplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// These fields follow UI server/forms/flowchartBlocks.ts and geometry/flowchartShape.ts.
// RawMessage is restricted to the public form's documented arbitrary item values/shape.
type formDocument struct {
	Sections []struct {
		Content []struct {
			ID    string          `json:"id"`
			Value json.RawMessage `json:"value"`
		} `json:"content"`
	} `json:"sections"`
}
type viewDocument struct {
	Shape json.RawMessage `json:"shape"`
	Size  struct {
		W *float64 `json:"w"`
		H *float64 `json:"h"`
	} `json:"size"`
	TextFrame struct {
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
		W *float64 `json:"w"`
		H *float64 `json:"h"`
	} `json:"textFrame"`
}
type shapeDocument struct {
	Type      string      `json:"type"`
	D         string      `json:"d"`
	Primitive []primitive `json:"primitive"`
}
type primitive struct {
	Kind string   `json:"kind"`
	X    *float64 `json:"x"`
	Y    *float64 `json:"y"`
	W    *float64 `json:"w"`
	H    *float64 `json:"h"`
	X1   *float64 `json:"x1"`
	Y1   *float64 `json:"y1"`
	X2   *float64 `json:"x2"`
	Y2   *float64 `json:"y2"`
	RX   *float64 `json:"rx"`
	RY   *float64 `json:"ry"`
	D    string   `json:"d"`
}

func bounded(v *float64) bool {
	return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) && math.Abs(*v) <= 100000
}
func validShape(raw json.RawMessage) bool {
	var s shapeDocument
	if json.Unmarshal(raw, &s) != nil {
		return false
	}
	if s.Type == "path" {
		return strings.TrimSpace(s.D) != "" && len(s.D) <= 65536
	}
	if s.Type != "primitive" || len(s.Primitive) == 0 || len(s.Primitive) > 256 {
		return false
	}
	for _, p := range s.Primitive {
		switch p.Kind {
		case "rect", "roundedRect":
			if !bounded(p.X) || !bounded(p.Y) || !bounded(p.W) || !bounded(p.H) || *p.W < 0 || *p.H < 0 {
				return false
			}
			if p.RX != nil && (!bounded(p.RX) || *p.RX < 0) || p.RY != nil && (!bounded(p.RY) || *p.RY < 0) {
				return false
			}
		case "line":
			if !bounded(p.X1) || !bounded(p.Y1) || !bounded(p.X2) || !bounded(p.Y2) {
				return false
			}
		case "path":
			if strings.TrimSpace(p.D) == "" || len(p.D) > 65536 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func decodeDefinition(def publicapi.GraphBlock) (json.RawMessage, publicapi.GraphNativeSettings, error) {
	invalid := errors.New("native catalog definition is malformed or unsupported")
	empty := publicapi.GraphNativeSettings{}
	if def.ID < 1 || def.ParentID < 1 || len(def.Form) > 256<<10 {
		return nil, empty, invalid
	}
	var form formDocument
	if json.Unmarshal(def.Form, &form) != nil {
		return nil, empty, invalid
	}
	values := map[string]json.RawMessage{}
	for _, section := range form.Sections {
		for _, item := range section.Content {
			if item.ID == "" || len(item.ID) > 255 || len(item.Value) == 0 {
				return nil, empty, invalid
			}
			if _, exists := values[item.ID]; exists {
				return nil, empty, invalid
			}
			values[item.ID] = item.Value
		}
	}
	var blockID, viewJSON string
	if json.Unmarshal(values["blockId"], &blockID) != nil || blockID != def.BlockID || json.Unmarshal(values["view"], &viewJSON) != nil {
		return nil, empty, invalid
	}
	var v viewDocument
	if json.Unmarshal([]byte(viewJSON), &v) != nil || !bounded(v.Size.W) || !bounded(v.Size.H) || *v.Size.W < 1 || *v.Size.H < 1 || !validShape(v.Shape) {
		return nil, empty, invalid
	}
	f := v.TextFrame
	if !bounded(f.X) || !bounded(f.Y) || !bounded(f.W) || !bounded(f.H) || *f.W < 0 || *f.H < 0 {
		return nil, empty, invalid
	}
	values["view"] = json.RawMessage(viewJSON)
	data := map[string]json.RawMessage{}
	for key, value := range values {
		data[fmt.Sprintf("__form__%d:%s", def.ID, key)] = value
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, empty, invalid
	}
	return raw, publicapi.GraphNativeSettings{Width: *v.Size.W, Height: *v.Size.H, BlockID: def.BlockID, Shape: append(json.RawMessage(nil), v.Shape...), TextFrame: publicapi.GraphFrame{X: *f.X, Y: *f.Y, W: *f.W, H: *f.H}}, nil
}
