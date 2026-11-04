package publicapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMoveViewportSendsTheCellIndex(t *testing.T) {
	c, calls := rpcBackend(t, func(string) string { return `{"commandId":"cmd-1","openTabs":0}` })
	ctx := context.Background()
	zoom := 1.5
	for _, tc := range []struct {
		in   ViewportInput
		sent string
	}{
		// "C, 7" is centred at (100, 300): sim-api's cell {2, 6}, centred at (50·x, 50·y).
		{ViewportInput{LayerID: "L", Action: "pan", Cell: "C, 7"}, `{"accId":"ws","action":"pan","cell":{"x":2,"y":6},"layerId":"L"}`},
		{ViewportInput{LayerID: "L", Action: "zoom", Cell: "-B, -1", Zoom: &zoom, TransitionMs: 300}, `{"accId":"ws","action":"zoom","cell":{"x":-2,"y":-1},"layerId":"L","transitionMs":300,"zoom":1.5}`},
		{ViewportInput{LayerID: "L", Action: "focus", ActorID: "a1"}, `{"accId":"ws","action":"focus","actorId":"a1","layerId":"L"}`},
		{ViewportInput{LayerID: "L", Action: "openLayer", TargetLayerID: "L2"}, `{"accId":"ws","action":"openLayer","layerId":"L","targetLayerId":"L2"}`},
	} {
		out, err := c.MoveViewport(ctx, tc.in)
		if err != nil || out != (ViewportMoved{CommandID: "cmd-1"}) {
			t.Fatal(tc.in, out, err)
		}
		last := (*calls)[len(*calls)-1]
		if sent, _ := json.Marshal(last.Params); last.Method != "graph/viewport/command" || string(sent) != tc.sent {
			t.Errorf("%+v sent %s %s, want %s", tc.in, last.Method, sent, tc.sent)
		}
	}
	before := len(*calls)
	for _, bad := range []ViewportInput{{Action: "fit"}, {LayerID: "L", Action: "pan", Cell: "C7"}} {
		if _, err := c.MoveViewport(ctx, bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if len(*calls) != before {
		t.Error("a refused command reached the API")
	}
}
