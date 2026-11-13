package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// collapseBackend answers the collapse reads: one whole fold (10) hiding 11 and 12, one fold by
// condition (20) hiding 21, a fan (30) of 14 children with 31..34 folded, and a fold (99) gone.
func collapseBackend(t *testing.T, state string) (*Client, *[]rpcCall) {
	t.Helper()
	return rpcBackend(t, func(method string) string {
		switch method {
		case "graph/layer/collapse/state/get", "graph/layer/collapse/state/update":
			return state
		case "graph/layer/extent/get":
			return `{"extent":{"x1":0,"y1":0,"x2":1000,"y2":500}}`
		case "graph/layer/collapse/resolve":
			return `{"hidden":[{"laId":11,"root":10},{"laId":12,"root":10},{"laId":21,"root":20},` +
				`{"laId":31,"root":30,"fan":true,"via":31},{"laId":32,"root":30,"fan":true,"via":32},{"laId":33,"root":30,"fan":true,"via":33},{"laId":34,"root":30,"fan":true,"via":34}],` +
				`"roots":[{"laId":10,"conditional":false,"visible":true,"hiddenCount":2,"sharedVisibleCount":0,"childCount":1,"matchedChildCount":1},` +
				`{"laId":20,"conditional":true,"visible":true,"hiddenCount":1,"sharedVisibleCount":0,"childCount":3,"matchedChildCount":1}],` +
				`"unknownRoots":[99],"fans":[{"laId":30,"childCount":14,"shown":[],"centroid":{"x":1,"y":1}}],"truncated":false}`
		case "graph/layer/elements/list":
			return `{"nodes":[{"laId":10,"id":"a10","actor":{"title":"ACME"}},{"laId":11,"id":"a11","actor":{"title":"Order 1"}},` +
				`{"laId":12,"accessDenied":true},{"laId":20,"id":"a20","actor":{"title":"Beta"}},{"laId":30,"id":"a30","actor":{"title":"Hub"}}]}`
		case "graph/layer/collapse/children":
			return `{"items":[{"laId":31,"actor":{"id":"a31","title":"Leaf 31","formId":5}},{"laId":32,"accessDenied":true}],"hasMore":true}`
		case "form/get":
			return `{"title":"Order"}`
		}
		return `{}`
	})
}

const savedState = `{"roots":[10,99],"conditional":[{"laId":20,"condition":{"formId":5,"q":"stage=paid"},"summary":"Order: stage=paid"}],` +
	`"fanOpen":[],"fanShown":[{"laId":30,"children":[35]}],"maxVisibleChildren":10}`

func TestGetLayerCollapseResolvesTheSavedState(t *testing.T) {
	c, calls := collapseBackend(t, savedState)
	view, err := c.GetLayerCollapse(context.Background(), CollapseGetInput{LayerID: "L"})
	if err != nil {
		t.Fatal(err)
	}
	if m := methodsOf(*calls); m != "graph/layer/collapse/state/get,graph/layer/extent/get,graph/layer/collapse/resolve,graph/layer/elements/list" {
		t.Fatal(m)
	}
	resolve, _ := json.Marshal((*calls)[2].Params)
	want := `{"accId":"ws","actorId":"L","conditional":[{"condition":{"formId":5,"q":"stage=paid"},"laId":20,"summary":"Order: stage=paid"}],` +
		`"fanShown":[{"children":[35],"laId":30}],"maxVisibleChildren":10,"roots":[10,99],"zone":{"x1":-50,"x2":1050,"y1":-50,"y2":550}}`
	if string(resolve) != want {
		t.Fatalf("resolve sent %s", resolve)
	}
	if view.Owner != "you" || view.HiddenTotal != 7 || view.MaxVisibleChildren != 10 || len(view.Folds) != 3 || len(view.Fans) != 1 {
		t.Fatalf("%+v", view)
	}
	if f := view.Folds[0]; f.Title != "ACME" || f.HiddenCount != 2 || f.Conditional {
		t.Fatalf("%+v", f)
	}
	if f := view.Folds[1]; f.Title != "Beta" || f.Condition == nil || f.Condition.Q != "stage=paid" || f.Summary != "Order: stage=paid" {
		t.Fatalf("%+v", f)
	}
	if f := view.Folds[2]; f.LaID != 99 || !f.Gone {
		t.Fatalf("%+v", f)
	}
	if f := view.Fans[0]; f.Title != "Hub" || f.HiddenCount != 4 || f.ChildCount != 14 || f.Opened {
		t.Fatalf("%+v", f)
	}
	if view.FanOpen == nil || len(view.FanOpen) != 0 {
		t.Fatalf("fanOpen %v", view.FanOpen)
	}
}

func TestGetLayerCollapseListsWhatHidesUnder(t *testing.T) {
	c, calls := collapseBackend(t, savedState)
	view, err := c.GetLayerCollapse(context.Background(), CollapseGetInput{LayerID: "L", LaID: 10, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	// The second hidden placement of fold 10, which the caller may not see.
	if u := view.Under; u == nil || u.Kind != "fold" || len(u.Items) != 1 || u.Items[0].LaID != 12 || !u.Items[0].AccessDenied || u.HasMore {
		t.Fatalf("%+v", view.Under)
	}
	view, err = c.GetLayerCollapse(context.Background(), CollapseGetInput{LayerID: "L", LaID: 30, Q: " Leaf "})
	if err != nil {
		t.Fatal(err)
	}
	children, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	if string(children) != `{"accId":"ws","actorId":"L","exclude":[35],"laId":30,"limit":10,"maxVisibleChildren":10,"offset":0,"q":"Leaf"}` {
		t.Fatalf("children sent %s", children)
	}
	if u := view.Under; u == nil || u.Kind != "fan" || len(u.Items) != 2 || u.Items[0].Title != "Leaf 31" || !u.Items[1].AccessDenied || !u.HasMore {
		t.Fatalf("%+v", view.Under)
	}
	for _, bad := range []CollapseGetInput{{LayerID: "L", LaID: 11}, {LayerID: "L", LaID: 10, Q: "x"}, {LayerID: "L", Limit: 5}, {}} {
		if _, err := c.GetLayerCollapse(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestGetLayerCollapseWithNothingFoldedReadsOnlyTheState(t *testing.T) {
	// A fan opened before the limit was turned off is still the user's: reported, so it can be closed.
	c, calls := collapseBackend(t, `{"roots":[],"conditional":[],"fanOpen":[30],"fanShown":[],"maxVisibleChildren":0}`)
	view, err := c.GetLayerCollapse(context.Background(), CollapseGetInput{LayerID: "L"})
	if err != nil || len(view.Folds) != 0 || len(view.Fans) != 0 || len(view.FanOpen) != 1 || methodsOf(*calls) != "graph/layer/collapse/state/get" {
		t.Fatal(view, err, methodsOf(*calls))
	}
}

func TestUpdateLayerCollapseNamesAConditionalFold(t *testing.T) {
	c, calls := collapseBackend(t, `{"roots":[10],"conditional":[],"fanOpen":[],"fanShown":[],"maxVisibleChildren":0,"changed":true,"openTabs":1}`)
	from := 500.0
	out, err := c.UpdateLayerCollapse(context.Background(), CollapseUpdateInput{LayerID: "L", Expand: []int64{7}, Collapse: []int64{10},
		Conditional: []CollapseConditional{{LaID: 20, Condition: CollapseCondition{FormID: 5, Q: "stage=paid", AccountNameID: "n", CurrencyID: 1, AmountFrom: &from}}}})
	if err != nil || !out.Changed || out.OpenTabs != 1 || out.LayerID != "L" {
		t.Fatal(out, err)
	}
	sent, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	want := `{"accId":"ws","actorId":"L","collapse":[10],"conditional":[{"condition":{"accountNameId":"n","amountFrom":500,"currencyId":1,"formId":5,"q":"stage=paid"},` +
		`"laId":20,"summary":"Order: stage=paid; balance ≥ 500"}],"expand":[7]}`
	if m := methodsOf(*calls); m != "form/get,graph/layer/collapse/state/update" || string(sent) != want {
		t.Fatalf("%s sent %s", m, sent)
	}
}

func TestUpdateLayerCollapseRefusesAnAPIKey(t *testing.T) {
	c, err := New(Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.UpdateLayerCollapse(context.Background(), CollapseUpdateInput{LayerID: "L", ExpandAll: true}); !errors.Is(err, errCollapseAPIKey) {
		t.Fatal(err)
	}
	if c.collapseOwner() != "apiUser" {
		t.Fatal(c.collapseOwner())
	}
	host, _ := New(Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "ApiKey k"})
	if _, err = host.UpdateLayerCollapse(context.Background(), CollapseUpdateInput{LayerID: "L", ExpandAll: true}); !errors.Is(err, errCollapseAPIKey) {
		t.Fatal(err)
	}
}

func TestCollapseSummary(t *testing.T) {
	to := 10.0
	if s := collapseSummary("Order", CollapseCondition{FormID: 1}); s != "Order" {
		t.Fatal(s)
	}
	if s := collapseSummary("Order", CollapseCondition{FormID: 1, AmountTo: &to}); s != "Order: balance ≤ 10" {
		t.Fatal(s)
	}
	if s := collapseSummary(strings.Repeat("я", 600), CollapseCondition{}); len([]rune(s)) != 500 {
		t.Fatal(len([]rune(s)))
	}
}

func TestLayerSettingsMaxVisibleChildren(t *testing.T) {
	c, calls := rpcBackend(t, func(string) string { return `{"id":"L"}` })
	for _, v := range []int{0, 25} {
		if _, err := c.UpdateLayerSettings(context.Background(), LayerSettingsInput{LayerID: "L", MaxVisibleChildren: &v}); err != nil {
			t.Fatal(err)
		}
	}
	sent, _ := json.Marshal((*calls)[1].Params["data"])
	if string(sent) != `{"maxVisibleChildren":25}` {
		t.Fatal(string(sent))
	}
	bad := 201
	if _, err := c.UpdateLayerSettings(context.Background(), LayerSettingsInput{LayerID: "L", MaxVisibleChildren: &bad}); err == nil || len(*calls) != 2 {
		t.Fatal("201 accepted", err)
	}
}
