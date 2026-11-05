package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func holeBackend(t *testing.T) (*Client, *[]rpcCall) {
	return rpcBackend(t, func(method string) string {
		switch method {
		case "actor/get":
			return `{"id":"h1","formId":7,"hole":true,"data":{"status":"done","owner":"Ann"}}`
		case "edge/get":
			return `{"id":"he1","source":"a1","target":"a2","edgeTypeId":3,"hole":true}`
		case "actor/hole/merge":
			return `{"holeActorId":"h1","actorId":"a9","changes":[{"layerId":"L","removedLaId":4,"addedLaId":5}]}`
		case "edge/hole/merge":
			return `{"holeEdgeId":"he1","edgeId":"e9","changes":[{"layerId":"L","removedLeId":6,"addedLeId":7}]}`
		case "batch":
			return batchOf(`{"id":"0","result":{"id":"n1"}}`, `{"id":"1","result":{"holeActorId":"h1","changes":[{"layerId":"L","removedLaId":4,"addedLaId":8}],"closingProgress":{"matched":1,"total":2},"conditions":[{"key":"status"}],"conditionErrors":["owner"]}}`)
		}
		return `{}`
	})
}

func called(calls []rpcCall) string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Method
	}
	return strings.Join(names, ",")
}

func lastParams(calls []rpcCall) string {
	b, _ := json.Marshal(calls[len(calls)-1].Params)
	return string(b)
}

func TestCloseHoleWithExistingActor(t *testing.T) {
	c, calls := holeBackend(t)
	out, err := c.CloseHole(context.Background(), HoleCloseInput{LayerID: "L", HoleActorID: "h1", ActorID: "a9", HoleLaID: 4})
	if err != nil || out.ActorID != "a9" || out.Created || len(out.Changes) != 1 || out.Changes[0].AddedLaID != 5 || out.Progress != nil {
		t.Fatal(out, err)
	}
	if got := called(*calls); got != "actor/hole/merge" {
		t.Fatal(got)
	}
	if got := lastParams(*calls); got != `{"accId":"ws","actorId":"a9","holeActorId":"h1","holeLaId":4,"layerId":"L"}` {
		t.Fatal(got)
	}
}

func TestCloseHoleWithNewActor(t *testing.T) {
	c, calls := holeBackend(t)
	out, err := c.CloseHole(context.Background(), HoleCloseInput{LayerID: "L", HoleActorID: "h1", ForceMerge: true,
		NewActor: &HoleNewActor{Title: "Ann's order", Data: json.RawMessage(`{"owner":"Bob","qty":2}`)}})
	if err != nil || out.ActorID != "n1" || !out.Created || out.Changes[0].AddedLaID != 8 || out.Progress == nil || out.Progress.Total != 2 || len(out.ConditionErrors) != 1 {
		t.Fatal(out, err)
	}
	if got := called(*calls); got != "actor/get,batch" {
		t.Fatal(got)
	}
	sent := lastParams(*calls)
	for _, want := range []string{`"isAtomic":true`, `"method":"actor/create"`, `"formId":7`, `"title":"Ann's order"`,
		// The hole's data with the caller's keys on top: its conditions are met unless overridden.
		`"data":{"owner":"Bob","qty":2,"status":"done"}`,
		`"method":"actor/hole/merge","params":{"accId":"ws","actorId":"$0.id","forceMerge":true,"holeActorId":"h1","layerId":"L"},"useRefs":true`} {
		if !strings.Contains(sent, want) {
			t.Errorf("%s lacks %s", sent, want)
		}
	}
}

func TestCloseHoleRefusals(t *testing.T) {
	c, calls := holeBackend(t)
	for _, bad := range []HoleCloseInput{
		{HoleActorID: "h1", ActorID: "a1"},
		{LayerID: "L", ActorID: "a1"},
		{LayerID: "L", HoleActorID: "h1"},
		{LayerID: "L", HoleActorID: "h1", ActorID: "a1", NewActor: &HoleNewActor{Title: "x"}},
		{LayerID: "L", HoleActorID: "h1", ActorID: "h1"},
		{LayerID: "L", HoleActorID: "h1", NewActor: &HoleNewActor{}},
	} {
		if _, err := c.CloseHole(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if len(*calls) != 0 {
		t.Error("a refused close reached the API:", called(*calls))
	}
	// A real actor named as the hole is refused before anything is created.
	notHole, calls := rpcBackend(t, func(string) string { return `{"id":"a1","formId":7,"hole":false}` })
	if _, err := notHole.CloseHole(context.Background(), HoleCloseInput{LayerID: "L", HoleActorID: "a1", NewActor: &HoleNewActor{Title: "x"}}); err == nil || called(*calls) != "actor/get" {
		t.Fatal(err, called(*calls))
	}
}

func TestCloseEdgeHole(t *testing.T) {
	c, calls := holeBackend(t)
	out, err := c.CloseEdgeHole(context.Background(), EdgeHoleCloseInput{LayerID: "L", HoleEdgeID: "he1", EdgeID: "e9"})
	if err != nil || out.EdgeID != "e9" || out.Created || out.Changes[0].AddedLeID != 7 || called(*calls) != "edge/hole/merge" {
		t.Fatal(out, err, called(*calls))
	}

	c, calls = rpcBackend(t, func(method string) string {
		if method == "edge/get" {
			return `{"id":"he1","source":"a1","target":"a2","edgeTypeId":3,"hole":true}`
		}
		return batchOf(`{"id":"0","result":{"id":"e2"}}`, `{"id":"1","result":{"changes":[{"layerId":"L","removedLeId":6,"addedLeId":9}]}}`)
	})
	out, err = c.CloseEdgeHole(context.Background(), EdgeHoleCloseInput{LayerID: "L", HoleEdgeID: "he1", Name: "pays"})
	if err != nil || out.EdgeID != "e2" || !out.Created || out.Changes[0].AddedLeID != 9 || called(*calls) != "edge/get,batch" {
		t.Fatal(out, err, called(*calls))
	}
	sent := lastParams(*calls)
	for _, want := range []string{`"method":"edge/create","params":{"accId":"ws","edgeTypeId":3,"forceDirection":true,"name":"pays","source":"a1","target":"a2"}`,
		`"method":"edge/hole/merge","params":{"accId":"ws","edgeId":"$0.id","holeEdgeId":"he1","layerId":"L"},"useRefs":true`} {
		if !strings.Contains(sent, want) {
			t.Errorf("%s lacks %s", sent, want)
		}
	}
	if _, err := c.CloseEdgeHole(context.Background(), EdgeHoleCloseInput{LayerID: "L", HoleEdgeID: "he1", EdgeID: "e9", Name: "x"}); err == nil {
		t.Error("name with an existing edge accepted")
	}
}
