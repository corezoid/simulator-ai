package publicapi

import "math"

// Opening a node in place (multi-layers), ported from simulator-ui-kit graph-layer store/nesting.ts
// and app/layerEditor/commands/nestingCommands.ts (expandPatchFor, expandOps, collapseOps) and
// nested/useNestedControls.ts (resize). The open state lives in the placement's layerSettings,
// written with graph/layer/actor_settings/save, which MERGES the sent keys into the stored bag:
//   - expand: the node is open; expandType what it shows: layer (its own layer, a plain actor its
//     ego-layer), chart (a Dashboards actor), script (a Scripts actor, a Smart Form), profile (an
//     actor card, which the Graph Editor cannot draw yet, so it refuses to open one);
//   - flaskView: a layer opens as a tinted flask (legacy default), the other kinds do not;
//   - nestedFrame {left, top, width, height}: the open box in whole cells relative to the node's
//     cell (0, 0) — a key of sim-api's Extra, written by the Graph Editor; the node itself stays
//     one cell, so edges and hit-testing do not change;
//   - expandPushed: whether this open state made room (the layer's anisotropy when it opened); a
//     close, and a resize while open, mirror it, so every push gets exactly one pull. Absent reads
//     as pushed: nodes opened before the key existed were opened by a client that always pushed —
//     except a chart without nestedFrame, which the plugin's earlier chart_create opened in place
//     without pushing anybody (the plugin's one deviation from the Graph Editor's reading).
//
// Where the sources disagree (recorded, not resolved):
//   - offset: legacy (EXP_NODE.offset) and the Graph Editor write how far the box reaches past the
//     cell as {left, right, top, bottom}; sim-api types it {x, y}, so such a value is stored as
//     {x: 0, y: 0} and its doc says not to send it. The box therefore travels as nestedFrame only.
//   - box defaults: legacy EXP_NODE.size is 11x7 cells for a layer, 9x13 for a profile; the Graph
//     Editor's are 13x9 (layer, chart), 11x9 (Smart Form), 9x7 (card). The plugin follows the
//     Graph Editor; a legacy viewer draws the open node at its own default size.
//   - width/height: legacy ExpNode reads them as the open box, the Graph Editor ignores them on an
//     actor and does not write them. The plugin writes the box there too (as chart_create always
//     did), so a legacy viewer draws the node at the size the Graph Editor does.

// Nested kinds and what each is persisted as.
const (
	nestedLayer     = "layer"
	nestedChart     = "chart"
	nestedSmartForm = "smartForm"
	nestedCard      = "actorCard"
)

// nestedMinWidth/Height are the smallest useful open box in cells (NESTED_MIN_CELLS); the maxima
// are the plugin's own bound on one box.
const (
	nestedMinWidth, nestedMinHeight = 5, 4
	nestedMaxCells                  = 60
)

// NestedFrame is the open box in whole cells relative to the node's cell: left/top the first
// occupied cell (<= 0), width/height the counts.
type NestedFrame struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// nestedFrameWire is a stored nestedFrame, read as numbers so a non-integer is refused, not cut.
type nestedFrameWire struct {
	Left   *float64 `json:"left"`
	Top    *float64 `json:"top"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

// openSettings is the part of a placement's layerSettings opening reads.
type openSettings struct {
	Expand       bool             `json:"expand"`
	ExpandType   string           `json:"expandType"`
	ExpandPushed *bool            `json:"expandPushed"`
	Profile      bool             `json:"profile"`
	NestedFrame  *nestedFrameWire `json:"nestedFrame"`
}

// openPatch is what an open, a close or a resize writes: only the keys sent are merged.
type openPatch struct {
	Expand       *bool        `json:"expand,omitempty"`
	ExpandType   string       `json:"expandType,omitempty"`
	FlaskView    *bool        `json:"flaskView,omitempty"`
	FullScreen   *bool        `json:"fullScreen,omitempty"`
	Profile      *bool        `json:"profile,omitempty"`
	NestedFrame  *NestedFrame `json:"nestedFrame,omitempty"`
	ExpandPushed *bool        `json:"expandPushed,omitempty"`
	// Width/Height are the open box in graph units, for legacy ExpNode.
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

// nestedSystemForms are the system templates the kind check needs.
type nestedSystemForms struct{ Scripts, Dashboards int64 }

// nestedKindOf resolves what a node opens into (nestedKindOf): a profile is a card, a Dashboards
// actor a chart, a Scripts actor a Smart Form, everything else a layer.
func nestedKindOf(formID int64, s openSettings, forms nestedSystemForms) string {
	switch {
	case s.Profile || s.ExpandType == "profile":
		return nestedCard
	case forms.Dashboards > 0 && formID == forms.Dashboards:
		return nestedChart
	case forms.Scripts > 0 && formID == forms.Scripts:
		return nestedSmartForm
	}
	return nestedLayer
}

// expandContentFor is the expandType a kind is stored as.
func expandContentFor(kind string) string {
	switch kind {
	case nestedChart:
		return "chart"
	case nestedSmartForm:
		return "script"
	case nestedCard:
		return "profile"
	}
	return "layer"
}

// kindFromExpandType reads the kind an open node was stored as.
func kindFromExpandType(expandType string) string {
	switch expandType {
	case "profile", "cardView":
		return nestedCard
	case "script":
		return nestedSmartForm
	case "chart":
		return nestedChart
	}
	return nestedLayer
}

// nestedDefaultCells is each kind's opening box in cells (odd counts, so the box is centred).
func nestedDefaultCells(kind string) (w, h int) {
	switch kind {
	case nestedSmartForm:
		return 11, 9
	case nestedCard:
		return 9, 7
	}
	return 13, 9
}

// centredFrame is a w x h box centred on the node's cell.
func centredFrame(w, h int) NestedFrame {
	return NestedFrame{Left: -(w / 2), Top: -(h / 2), Width: w, Height: h}
}

// usable reports whether a frame is whole, at least the minimum and holds the node's own cell.
func (f NestedFrame) usable() bool {
	return f.Width >= nestedMinWidth && f.Height >= nestedMinHeight && f.Left <= 0 && f.Left+f.Width > 0 &&
		f.Top <= 0 && f.Top+f.Height > 0
}

// storedFrame is the stored nestedFrame when it is usable (nestedGeometryOf).
func (s openSettings) storedFrame() (NestedFrame, bool) {
	w := s.NestedFrame
	if w == nil || w.Left == nil || w.Top == nil || w.Width == nil || w.Height == nil {
		return NestedFrame{}, false
	}
	vals := []float64{*w.Left, *w.Top, *w.Width, *w.Height}
	for _, v := range vals {
		if v != math.Trunc(v) || math.Abs(v) > 1e6 {
			return NestedFrame{}, false
		}
	}
	f := NestedFrame{Left: int(vals[0]), Top: int(vals[1]), Width: int(vals[2]), Height: int(vals[3])}
	return f, f.usable()
}

// frameOf is the box a node of this expandType opens to: its saved frame, else the kind's default.
func (s openSettings) frameOf(expandType string) NestedFrame {
	if f, ok := s.storedFrame(); ok {
		return f
	}
	return centredFrame(nestedDefaultCells(kindFromExpandType(expandType)))
}

// nestedBoxAt is the open box in graph units around the node centred at (x, y).
func nestedBoxAt(x, y float64, f NestedFrame) graphRect {
	return graphRect{X: x + (float64(f.Left)-0.5)*gridCell, Y: y + (float64(f.Top)-0.5)*gridCell,
		W: float64(f.Width) * gridCell, H: float64(f.Height) * gridCell}
}

// pushed reports whether an open node made room when it opened. Absent reads as yes (the Graph
// Editor's rule: its earlier clients always pushed), except for the plugin's own earlier charts:
// chart_create wrote {expand, expandType: chart, width, height} with no nestedFrame and pushed
// nobody, so pulling the layer back for one would draw it into the chart's box.
func (s openSettings) pushed() bool {
	if s.ExpandPushed != nil {
		return *s.ExpandPushed
	}
	return !(s.ExpandType == "chart" && s.NestedFrame == nil)
}

// openNode is a placement as opening needs it: its centre, collapsed body and open state.
type openNode struct {
	X, Y      float64
	SizeGrade string
	Settings  openSettings
}

func (n openNode) collapsed() graphRect { return actorBox(n.X, n.Y, n.SizeGrade) }

// box is the box the node occupies while open.
func (n openNode) box() graphRect {
	return nestedBoxAt(n.X, n.Y, n.Settings.frameOf(n.Settings.ExpandType))
}

// expandPlan opens a node of kind, with frame when given (else its saved frame, else the default):
// the bag to write and, on a layer with anisotropy, the make-room directive (expandPatchFor +
// expandOps). The bag records the setting as expandPushed whether or not anybody moved.
func expandPlan(n openNode, kind string, frame *NestedFrame, anisotropy bool) (openPatch, *OffsetDirective) {
	expandType := expandContentFor(kind)
	f := n.Settings.frameOf(expandType)
	if frame != nil {
		f = *frame
	}
	grown := nestedBoxAt(n.X, n.Y, f)
	yes, flask := true, kind == nestedLayer
	patch := openPatch{Expand: &yes, ExpandType: expandType, FlaskView: &flask, NestedFrame: &f, ExpandPushed: &anisotropy, Width: grown.W, Height: grown.H}
	if !anisotropy {
		return patch, nil
	}
	return patch, deriveOffsetDirective(n.collapsed(), grown)
}

// collapsePlan closes an open node: the open flags cleared and, when its open pushed, the inverse
// directive from the open box back to the body, which returns the neighbours (collapseOps).
// nestedFrame stays, so reopening restores the size.
func collapsePlan(n openNode) (openPatch, *OffsetDirective) {
	no := false
	patch := openPatch{Expand: &no, FullScreen: &no, FlaskView: &no, Profile: &no}
	if !n.Settings.pushed() {
		return patch, nil
	}
	return patch, deriveOffsetDirective(n.box(), n.collapsed())
}

// resizePlan gives an open node a new frame; the open state's own expandPushed, not the live
// setting, decides whether the neighbours move with it (useNestedControls.resize).
func resizePlan(n openNode, frame NestedFrame) (openPatch, *OffsetDirective) {
	after := nestedBoxAt(n.X, n.Y, frame)
	patch := openPatch{NestedFrame: &frame, Width: after.W, Height: after.H}
	if !n.Settings.pushed() {
		return patch, nil
	}
	return patch, deriveOffsetDirective(n.box(), after)
}
