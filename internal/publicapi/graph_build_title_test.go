package publicapi

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A build repeated in a new turn (a new operationId) finds the layer of its title and builds
// nothing; allowDuplicateTitle makes the second layer; a title that only contains it is not it
// (eval build-01).
func TestGraphBuildReusesLayerTitle(t *testing.T) {
	c, f := fakeClient(t)
	in := GraphBuildInput{Nodes: gridNodes("n", 3, GraphPoint{X: -25, Y: -25})}
	first := build(t, c, GraphBuildTarget{NewLayer: "Org chart", OperationID: "turn-1"}, in)
	if !first.Done || first.LayerID == "" {
		t.Fatalf("%+v", first)
	}
	count := func() int { all, _, _ := f.Snapshot(""); return len(all) }
	actors := count()
	_, err := c.OpenGraphBuild(context.Background(), GraphBuildTarget{NewLayer: "  org  CHART ", OperationID: "turn-2"})
	var taken *LayerTitleTaken
	if !errors.As(err, &taken) || taken.LayerID != first.LayerID || taken.Count != 1 || !strings.Contains(err.Error(), "layerId "+first.LayerID) {
		t.Fatalf("%v", err)
	}
	if got := count(); got != actors {
		t.Fatalf("the repeat wrote %d actors", got-actors)
	}
	// The same operation still finds its own layer; another title and a deliberate duplicate are new.
	for _, target := range []GraphBuildTarget{{NewLayer: "Org chart", OperationID: "turn-1"}, {NewLayer: "Org chart 2", OperationID: "turn-3"},
		{NewLayer: "Org chart", OperationID: "turn-4", AllowDuplicateTitle: true}} {
		b, err := c.OpenGraphBuild(context.Background(), target)
		if err != nil || (target.OperationID == "turn-1") != (b.LayerID == first.LayerID) {
			t.Fatalf("%+v: %v %+v", target, err, b)
		}
	}
}
