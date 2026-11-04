package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAttachToActor(t *testing.T) {
	cases := []struct {
		name    string
		in      ActorAttachInput
		methods string // the calls made, in order
		sent    []string
		out     ActorAttached
	}{
		{"actor card", ActorAttachInput{ActorID: "a1", Kind: "actor", TargetID: "a2"}, "actor/update",
			[]string{`{"accId":"ws","actorId":"a1","cardActorId":"a2","fields":["id"]}`}, ActorAttached{ActorID: "a1", Kind: "actor", TargetID: "a2"}},
		{"smart form", ActorAttachInput{ActorID: "a1", Kind: "smartForm", TargetID: "app1"}, "actor/update",
			[]string{`{"accId":"ws","actorId":"a1","appId":"app1","fields":["id"]}`}, ActorAttached{ActorID: "a1", Kind: "smartForm", TargetID: "app1"}},
		{"existing form card", ActorAttachInput{ActorID: "a1", Kind: "form", TargetID: "f0"}, "edge_type/get,edge/create",
			[]string{`{"accId":"ws","edgeTypeId":3,"forceDirection":true,"pinned":true,"source":"a1","target":"f0"}`}, ActorAttached{ActorID: "a1", Kind: "form", TargetID: "f0", EdgeID: "e1"}},
		{"new form card", ActorAttachInput{ActorID: "a1", Kind: "form", FormID: 7, Title: "Order", Data: json.RawMessage(`{"qty":2}`)}, "edge_type/get,batch",
			[]string{`"isAtomic":true`, `"method":"actor/create","params":{"accId":"ws","color":"#`, `"data":{"qty":2},"fields":["id"],"formId":7,"title":"Order"}`,
				`"method":"edge/create","params":{"accId":"ws","edgeTypeId":3,"forceDirection":true,"pinned":true,"source":"a1","target":"$0.id"},"useRefs":true`},
			ActorAttached{ActorID: "a1", Kind: "form", TargetID: "f1", EdgeID: "e2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, calls := rpcBackend(t, func(method string) string {
				switch method {
				case "edge_type/get":
					return `{"id":3,"name":"hierarchy"}`
				case "edge/create":
					return `{"id":"e1"}`
				case "batch":
					return batchOf(`{"id":"0","result":{"id":"f1"}}`, `{"id":"1","result":{"id":"e2"}}`)
				}
				return `{"id":"a1"}`
			})
			out, err := c.AttachToActor(context.Background(), tc.in)
			if err != nil || out != tc.out {
				t.Fatal(out, err)
			}
			var names []string
			for _, call := range *calls {
				names = append(names, call.Method)
			}
			last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
			if strings.Join(names, ",") != tc.methods {
				t.Fatalf("called %v, want %s", names, tc.methods)
			}
			for _, want := range tc.sent {
				if !strings.Contains(string(last), want) {
					t.Errorf("%s lacks %s", last, want)
				}
			}
		})
	}
}

func TestAttachToActorRefusesBeforeAnyCall(t *testing.T) {
	c, calls := rpcBackend(t, func(string) string { return `{}` })
	for _, bad := range []ActorAttachInput{
		{Kind: "actor", TargetID: "a2"},
		{ActorID: "a1", Kind: "actor"},
		{ActorID: "a1", Kind: "actor", TargetID: "a1"},
		{ActorID: "a1", Kind: "form"},
		{ActorID: "a1", Kind: "form", TargetID: "f", FormID: 7},
		{ActorID: "a1", Kind: "smartForm", TargetID: "x", Title: "t"},
		{ActorID: "a1", Kind: "reaction", TargetID: "r"},
	} {
		if _, err := c.AttachToActor(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if len(*calls) != 0 {
		t.Error("a refused attach reached the API:", methods(*calls))
	}
}
