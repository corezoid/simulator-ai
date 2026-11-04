package publicapi

import (
	"context"
	"testing"
)

// The route table builds the legacy buildLink paths on the configured origin, with the short
// workspace segment and every id escaped as one path segment. None of these calls the API.
func TestBuildLinkPaths(t *testing.T) {
	c, err := New(Config{Origin: "https://sim.example.com/", Workspace: "f54fd368-6ebf-427a-89a3-72283e320f6a", Profile: "console", Authorization: "Bearer x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in   LinkInput
		want string
	}{
		{LinkInput{Entity: "actor", ID: "a1"}, "/actors_graph/f54fd368/view/a1"},
		{LinkInput{Entity: "actorList"}, "/actors_graph/f54fd368/list"},
		{LinkInput{Entity: "graph"}, "/actors_graph/f54fd368/graph"},
		{LinkInput{Entity: "layer", ID: "0"}, "/actors_graph/f54fd368/graph/0/layers"},
		{LinkInput{Entity: "layer", ID: "g1", Mode: "trees", FocusID: "l1"}, "/actors_graph/f54fd368/graph/g1/trees/l1"},
		{LinkInput{Entity: "event"}, "/events/f54fd368/list"},
		{LinkInput{Entity: "event", StreamID: "s1", SecondaryID: "s2"}, "/events/f54fd368/list/s1/s2"},
		{LinkInput{Entity: "event", SecondaryID: "s2"}, "/events/f54fd368/list"},
		{LinkInput{Entity: "chat"}, "/chats/f54fd368"},
		{LinkInput{Entity: "chat", ID: "c1"}, "/chats/f54fd368/list/chats/c1?tab=chat"},
		{LinkInput{Entity: "form"}, "/form/f54fd368/edit"},
		{LinkInput{Entity: "form", ID: "42"}, "/form/f54fd368/edit/42"},
		{LinkInput{Entity: "transaction", ID: "t1"}, "/actors_graph/f54fd368/transactions/view/t1"},
		{LinkInput{Entity: "transfer", ID: "t2"}, "/actors_graph/f54fd368/transfers/view/t2"},
		{LinkInput{Entity: "accounts"}, "/actors_graph/f54fd368/accounts"},
		{LinkInput{Entity: "chart", ID: "d1"}, "/dashboard/f54fd368/view/d1"},
		{LinkInput{Entity: "meeting", ID: "a1"}, "/meetingRoom/f54fd368/a1"},
		{LinkInput{Entity: "settings"}, "/settings/f54fd368"},
		{LinkInput{Entity: "actor", ID: "../../settings/x?y"}, "/actors_graph/f54fd368/view/..%2F..%2Fsettings%2Fx%3Fy"},
	} {
		got, err := c.BuildLink(context.Background(), tc.in)
		if err != nil || got.URL != "https://sim.example.com"+tc.want {
			t.Errorf("%+v: %q %v, want %s", tc.in, got.URL, err, tc.want)
		}
	}
	for _, in := range []LinkInput{{Entity: "actor"}, {Entity: "layer"}, {Entity: "chart"}, {Entity: "nope"}, {Entity: "layer", ID: "0", Mode: "x"}, {Entity: "smartForm", Ref: "leave"}} {
		if _, err := c.BuildLink(context.Background(), in); err == nil {
			t.Errorf("%+v accepted", in)
		}
	}
}
