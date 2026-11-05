package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// routeBackend answers route/build with the path of the start Smart Form route it was asked for.
func routeBackend(t *testing.T) (*Client, *[]rpcCall) {
	paths := map[string]string{"workspace.actors": "/actors", "workspace.graphs-edit": "/graphs/g1", "workspace.events": "/events?eventId=e1&tab=chat",
		"workspace.chats": "/chats", "workspace.templates-edit": "/templates/42", "order.confirm": "/confirm/o-1"}
	var calls *[]rpcCall
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "route/key/get":
			return `{"appKey":"ab12cd","activeReleaseId":7}`
		case "route/build":
			var p struct {
				Name string `json:"name"`
			}
			raw, _ := json.Marshal((*calls)[len(*calls)-1].Params)
			_ = json.Unmarshal(raw, &p)
			if path, ok := paths[p.Name]; ok {
				return `{"href":"/sf/ab12cd` + path + `"}`
			}
			return `"error":{"code":-32602,"message":"route.unknownName","data":{"reason":"invalidParams"}}`
		}
		return `{}`
	})
	return c, calls
}

func TestRuntimeLinksOpenInTheCurrentSmartForm(t *testing.T) {
	c, calls := routeBackend(t)
	ctx := context.Background()
	for _, tc := range []struct {
		in   LinkInput
		want string
		sent string // substring of the route/build params
	}{
		{LinkInput{Entity: "actor", ID: "a1", AppKey: "AB12CD"}, "/sf/ab12cd/actors#b=actors-list:panel%3Aws%3Aa1", `"name":"workspace.actors","sourceTarget":{"appKey":"ab12cd","kind":"dynamic","path":"/"}`},
		{LinkInput{Entity: "layer", GraphID: "g1", ID: "l1", FocusID: "a1", AppKey: "ab12cd"}, "/sf/ab12cd/graphs/g1#b=graphEditor:g1%3Al1%3Aa1", `"name":"workspace.graphs-edit","params":{"graphId":"g1"}`},
		{LinkInput{Entity: "event", ID: "e1", Query: map[string]string{"tab": "chat"}, AppKey: "ab12cd"}, "/sf/ab12cd/events?eventId=e1&tab=chat", `"query":{"eventId":"e1","tab":"chat"}`},
		{LinkInput{Entity: "chat", ID: "c 1", AppKey: "ab12cd"}, "/sf/ab12cd/chats#b=chats-list:panel%3Aws%3Ac%201", `"name":"workspace.chats"`},
		{LinkInput{Entity: "form", ID: "42", AppID: "app1"}, "/sf/ab12cd/templates/42", `"params":{"templateId":42}`},
		// A form of its own: the route its pages declare.
		{LinkInput{Entity: "actor", AppKey: "ab12cd", Route: "order.confirm", Params: map[string]json.RawMessage{"order": json.RawMessage(`"o-1"`)}}, "/sf/ab12cd/confirm/o-1", `"name":"order.confirm","params":{"order":"o-1"}`},
	} {
		got, err := c.BuildLink(ctx, tc.in)
		if err != nil || got != (Link{URL: tc.want}) {
			t.Errorf("%+v: %+v %v, want %s", tc.in, got, err, tc.want)
			continue
		}
		sent, _ := json.Marshal((*calls)[len(*calls)-1].Params)
		if !strings.Contains(string(sent), tc.sent) {
			t.Errorf("%+v sent %s, want %s", tc.in, sent, tc.sent)
		}
	}
	if m := methods(*calls); !strings.Contains(strings.Join(m, "\n"), `route/key/get {"accId":"ws","actorId":"app1"}`) {
		t.Error("appId was not turned into its key:", m)
	}
}

func TestRuntimeLinkFallbacksAndRefusals(t *testing.T) {
	c, calls := routeBackend(t)
	ctx := context.Background()
	// No runtime page for a chart: the legacy URL, marked, without asking route/build.
	before := len(*calls)
	got, err := c.BuildLink(ctx, LinkInput{Entity: "chart", ID: "d1", AppKey: "ab12cd"})
	if err != nil || !got.Legacy || !strings.HasSuffix(got.URL, "/dashboard/ws/view/d1") || len(*calls) != before {
		t.Fatal(got, err)
	}
	if _, err := c.BuildLink(ctx, LinkInput{Entity: "actor", AppKey: "ab12cd", Route: "nope"}); err == nil || !strings.Contains(err.Error(), "routing.routes") {
		t.Fatal("an unknown route name:", err)
	}
	for _, bad := range []LinkInput{
		{Entity: "layer", ID: "l1", AppKey: "ab12cd"},
		{Entity: "actor", AppKey: "ab12cd"},
		{Entity: "form", ID: "x", AppKey: "ab12cd"},
		{Entity: "actor", ID: "a1", AppKey: "/sf/ab"},
	} {
		if _, err := c.BuildLink(ctx, bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestEncodeURIComponent(t *testing.T) {
	if got := encodeURIComponent("panel:ws 1/a&b=c~'()*!._-"); got != "panel%3Aws%201%2Fa%26b%3Dc~'()*!._-" {
		t.Fatal(got)
	}
}

// No key for appId's Smart Form: the legacy URL, marked; a bad appKey given by the model still fails.
func TestRuntimeLinkWithoutAKey(t *testing.T) {
	c, _ := rpcBackend(t, func(method string) string {
		if method == "route/key/get" {
			return notFound
		}
		return `{}`
	})
	got, err := c.BuildLink(context.Background(), LinkInput{Entity: "actor", ID: "a1", AppID: "app1"})
	if err != nil || !got.Legacy || !strings.HasSuffix(got.URL, "/actors_graph/ws/view/a1") {
		t.Fatal(got, err)
	}
	if _, err := c.BuildLink(context.Background(), LinkInput{Entity: "actor", AppID: "app1"}); err == nil {
		t.Fatal("an actor link without id was built")
	}
}
