package publicapi

import (
	"encoding/json"
	"testing"
)

// The expected values below are what simulator-ui-kit graph-layer expandOps / collapseOps
// (app/layerEditor/commands/nestingCommands.ts) produce for the same placements, bundled with
// esbuild and run under node: the bag keys the plugin writes and the layer.offset directive.
func TestExpandPlanMatchesGraphEditor(t *testing.T) {
	forms := nestedSystemForms{Scripts: 9, Dashboards: 3}
	for _, tc := range []struct {
		name       string
		node       openNode
		formID     int64
		anisotropy bool
		bag        string
		directive  string
	}{
		{"layer", openNode{X: 100, Y: 150}, 7, true,
			`{"expand":true,"expandType":"layer","flaskView":true,"nestedFrame":{"left":-6,"top":-4,"width":13,"height":9},"expandPushed":true}`,
			`{"bounds":{"left":75,"right":125,"top":125,"bottom":175},"offset":{"top":200,"right":300,"bottom":200,"left":300}}`},
		{"layer S2", openNode{X: 100, Y: 150, SizeGrade: "S2"}, 7, true,
			`{"expand":true,"expandType":"layer","flaskView":true,"nestedFrame":{"left":-6,"top":-4,"width":13,"height":9},"expandPushed":true}`,
			`{"bounds":{"left":60,"right":140,"top":110,"bottom":190},"offset":{"top":200,"right":300,"bottom":200,"left":300}}`},
		{"layer S3", openNode{X: 100, Y: 150, SizeGrade: "S3"}, 7, true,
			`{"expand":true,"expandType":"layer","flaskView":true,"nestedFrame":{"left":-6,"top":-4,"width":13,"height":9},"expandPushed":true}`,
			`{"bounds":{"left":50,"right":150,"top":100,"bottom":200},"offset":{"top":200,"right":300,"bottom":200,"left":300}}`},
		{"smart form", openNode{}, 9, true,
			`{"expand":true,"expandType":"script","flaskView":false,"nestedFrame":{"left":-5,"top":-4,"width":11,"height":9},"expandPushed":true}`,
			`{"bounds":{"left":-25,"right":25,"top":-25,"bottom":25},"offset":{"top":200,"right":250,"bottom":200,"left":250}}`},
		{"chart", openNode{X: -250, Y: 50}, 3, true,
			`{"expand":true,"expandType":"chart","flaskView":false,"nestedFrame":{"left":-6,"top":-4,"width":13,"height":9},"expandPushed":true}`,
			`{"bounds":{"left":-275,"right":-225,"top":25,"bottom":75},"offset":{"top":200,"right":300,"bottom":200,"left":300}}`},
		{"no anisotropy", openNode{X: 100, Y: 150}, 7, false,
			`{"expand":true,"expandType":"layer","flaskView":true,"nestedFrame":{"left":-6,"top":-4,"width":13,"height":9},"expandPushed":false}`, `null`},
		{"saved frame", openNode{X: 100, Y: 150, Settings: stored(-2, -1, 7, 5)}, 7, true,
			`{"expand":true,"expandType":"layer","flaskView":true,"nestedFrame":{"left":-2,"top":-1,"width":7,"height":5},"expandPushed":true}`,
			`{"bounds":{"left":75,"right":125,"top":125,"bottom":175},"offset":{"top":50,"right":200,"bottom":150,"left":100}}`},
	} {
		patch, d := expandPlan(tc.node, nestedKindOf(tc.formID, tc.node.Settings, forms), nil, tc.anisotropy)
		// width/height are the plugin's addition for legacy ExpNode: the box the frame describes.
		if f := patch.NestedFrame; patch.Width != float64(f.Width)*gridCell || patch.Height != float64(f.Height)*gridCell {
			t.Errorf("%s legacy box %vx%v", tc.name, patch.Width, patch.Height)
		}
		patch.Width, patch.Height = 0, 0
		if got := jsonOf(t, patch); got != tc.bag {
			t.Errorf("%s bag %s\nwant %s", tc.name, got, tc.bag)
		}
		if got := jsonOf(t, d); got != tc.directive {
			t.Errorf("%s directive %s\nwant %s", tc.name, got, tc.directive)
		}
	}
	// A profile is an actor card: expandOps writes nothing for it.
	if nestedKindOf(7, openSettings{Profile: true}, forms) != nestedCard {
		t.Error("a profile placement is not a card")
	}
}

func TestCollapsePlanMatchesGraphEditor(t *testing.T) {
	yes, no := true, false
	open := openNode{X: 100, Y: 150, Settings: stored(-2, -1, 7, 5)}
	open.Settings.Expand, open.Settings.ExpandType, open.Settings.ExpandPushed = true, "layer", &yes
	patch, d := collapsePlan(open)
	if got := jsonOf(t, patch); got != `{"expand":false,"flaskView":false,"fullScreen":false,"profile":false}` {
		t.Error("bag", got)
	}
	if got := jsonOf(t, d); got != `{"bounds":{"left":-25,"right":325,"top":75,"bottom":325},"offset":{"top":-50,"right":-200,"bottom":-150,"left":-100}}` {
		t.Error("pull back", got)
	}
	// Opened quietly: closed quietly, whatever the layer's setting is now.
	open.Settings.ExpandPushed = &no
	if _, d := collapsePlan(open); d != nil {
		t.Error("a quiet open pulled its neighbours", jsonOf(t, d))
	}
	// Opened before expandPushed existed (always pushed), as a Smart Form with no saved frame.
	legacy := openNode{X: 100, Y: 150, Settings: openSettings{Expand: true, ExpandType: "script"}}
	if _, d := collapsePlan(legacy); jsonOf(t, d) != `{"bounds":{"left":-175,"right":375,"top":-75,"bottom":375},"offset":{"top":-200,"right":-250,"bottom":-200,"left":-250}}` {
		t.Error("legacy pull back", jsonOf(t, d))
	}
}

// A resize while open moves the neighbours by the change of the box, when the open pushed.
func TestResizePlan(t *testing.T) {
	open := openNode{X: 0, Y: 0, Settings: openSettings{Expand: true, ExpandType: "layer"}}
	patch, d := resizePlan(open, centredFrame(15, 9))
	if jsonOf(t, patch) != `{"nestedFrame":{"left":-7,"top":-4,"width":15,"height":9},"width":750,"height":450}` ||
		jsonOf(t, d) != `{"bounds":{"left":-325,"right":325,"top":-225,"bottom":225},"offset":{"top":0,"right":50,"bottom":0,"left":50}}` {
		t.Error(jsonOf(t, patch), jsonOf(t, d))
	}
}

func TestStoredFrameRefusesUnusable(t *testing.T) {
	for _, s := range []openSettings{stored(1, -1, 7, 5), stored(-2, -1, 4, 5), stored(-8, -1, 7, 5), {}} {
		if _, ok := s.storedFrame(); ok {
			t.Errorf("%+v accepted", s.NestedFrame)
		}
	}
	half := 0.5
	s := stored(-2, -1, 7, 5)
	s.NestedFrame.Left = &half
	if _, ok := s.storedFrame(); ok {
		t.Error("a fractional frame accepted")
	}
}

func stored(left, top, width, height float64) openSettings {
	return openSettings{NestedFrame: &nestedFrameWire{Left: &left, Top: &top, Width: &width, Height: &height}}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
