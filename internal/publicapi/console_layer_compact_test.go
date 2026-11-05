package publicapi

import (
	"context"
	"strings"
	"testing"
)

// A full page answers the layer's totals; compact rows carry no position and no edge actor ids.
func TestLayerGetCompactPageWithTotals(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "actor/get":
			return `{"id":"L","data":{}}`
		case "graph/layer/actors/list":
			return `{"list":[{"laId":7,"id":"a","position":{"x":12.5,"y":0},"actor":{"id":"a","title":"A","formId":3}}]}`
		case "graph/layer/edges/list":
			return `{"list":[{"laId":9,"id":"e","laIdSource":7,"laIdTarget":7,"edge":{"source":"a","target":"a","name":""}}]}`
		case "graph/layer/stats/get":
			return `{"nodes":1500,"edges":40,"holes":0,"holesClosed":0,"edgeHoles":0,"edgeHolesClosed":0}`
		}
		return `{}`
	})
	one := int64(1)
	v, err := c.GetLayer(context.Background(), LayerGetInput{LayerID: "L", Limit: &one})
	if err != nil {
		t.Fatal(err)
	}
	got := jsonOf(t, v)
	if !v.HasMore || v.Total == nil || *v.Total != (LayerTotals{Nodes: 1500, Edges: 40}) || strings.Contains(got, "position") ||
		strings.Contains(got, `"source"`) || strings.Contains(got, `"name"`) || !strings.Contains(got, `"cell":"A, 1"`) {
		t.Fatal(got)
	}
	v, err = c.GetLayer(context.Background(), LayerGetInput{LayerID: "L", Full: true})
	if err != nil || v.HasMore || v.Total != nil || v.Nodes[0].Position == nil || v.Edges[0].Source != "a" {
		t.Fatal(err, jsonOf(t, v))
	}
	if last := (*calls)[len(*calls)-1]; last.Method != "graph/layer/actors/list" || last.Params["limit"] != float64(200) {
		t.Fatalf("%+v", last)
	}
}

func TestSummarizePlacementsFoldsTheTail(t *testing.T) {
	var list []LayerPlacement
	for f := int64(1); f <= maxSummaryForms+3; f++ {
		id := f
		list = append(list, LayerPlacement{LaID: f, ActorID: "a", FormID: &id})
	}
	list = append(list, LayerPlacement{LaID: 99, ActorID: "d"}) // a denied actor: formId 0
	s := SummarizePlacements(list)
	last := s.Forms[len(s.Forms)-1]
	if len(s.Forms) != maxSummaryForms+1 || last != (FormCount{FormID: -1, Count: 4}) || s.Box != nil {
		t.Fatalf("%+v", s.Forms)
	}
}
