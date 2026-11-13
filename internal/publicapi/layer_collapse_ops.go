package publicapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// maxCollapseTitles bounds one title read (graph/layer/elements/list takes at most 500 nodes).
const maxCollapseTitles = 500

type titledNode struct {
	LaID         int64  `json:"laId"`
	ID           string `json:"id"`
	AccessDenied bool   `json:"accessDenied"`
	Actor        *struct {
		Title *string `json:"title"`
	} `json:"actor"`
}

// nodeTitles reads the actors of node placements: laId → node. Placements the caller may not see
// come back accessDenied, without a title.
func (c *Client) nodeTitles(ctx context.Context, layerID string, laIDs []int64) (map[int64]titledNode, error) {
	out := map[int64]titledNode{}
	if len(laIDs) == 0 {
		return out, nil
	}
	r, err := call[elementsListRequest, struct {
		Nodes []titledNode `json:"nodes"`
	}](ctx, c, "graph/layer/elements/list", elementsListRequest{AccID: c.workspace(), ActorID: layerID, Nodes: laIDs, Fields: []string{"id", "title"}})
	if err != nil {
		return nil, fmt.Errorf("graph/layer/elements/list: %w", err)
	}
	for _, n := range r.Nodes {
		out[n.LaID] = n
	}
	return out, nil
}

func (n titledNode) title() string {
	if n.Actor == nil || n.Actor.Title == nil {
		return ""
	}
	return *n.Actor.Title
}

// collapseTitles names the folded actors and the fan parents (the first maxCollapseTitles).
func (c *Client) collapseTitles(ctx context.Context, view *CollapseView) error {
	var ids []int64
	for _, f := range view.Folds {
		if !f.Gone && len(ids) < maxCollapseTitles {
			ids = append(ids, f.LaID)
		}
	}
	for _, f := range view.Fans {
		if len(ids) < maxCollapseTitles && !slices.Contains(ids, f.LaID) {
			ids = append(ids, f.LaID)
		}
	}
	nodes, err := c.nodeTitles(ctx, view.LayerID, ids)
	if err != nil {
		return err
	}
	for i := range view.Folds {
		view.Folds[i].Title = nodes[view.Folds[i].LaID].title()
	}
	for i := range view.Fans {
		view.Fans[i].Title = nodes[view.Fans[i].LaID].title()
	}
	return nil
}

// collapseUnder fills view.Under with one page of what hides under in.LaID: a fan's hidden children
// (graph/layer/collapse/children, which pages and searches by title), or the placements a fold
// hides, nearest root first as resolve reports them.
func (c *Client) collapseUnder(ctx context.Context, view *CollapseView, in CollapseGetInput, resolved *collapseResolved, shown []CollapseFanShown) error {
	if in.LaID == 0 {
		if in.Q != "" || in.Limit != 0 || in.Offset != 0 {
			return errors.New("q, limit and offset page the placements hidden under laId: name laId")
		}
		return nil
	}
	limit := in.Limit
	if limit == 0 {
		limit = 10
	}
	for _, f := range view.Fans {
		if f.LaID != in.LaID || f.Opened {
			continue
		}
		var exclude []int64
		for _, s := range shown {
			if s.LaID == in.LaID {
				exclude = s.Children
			}
		}
		page, err := call[map[string]any, struct {
			Items []struct {
				LaID         int64 `json:"laId"`
				AccessDenied bool  `json:"accessDenied"`
				Actor        *struct {
					ID    string  `json:"id"`
					Title *string `json:"title"`
				} `json:"actor"`
			} `json:"items"`
			HasMore bool `json:"hasMore"`
		}](ctx, c, "graph/layer/collapse/children", map[string]any{"accId": c.workspace(), "actorId": view.LayerID, "laId": in.LaID,
			"maxVisibleChildren": view.MaxVisibleChildren, "exclude": nonNil(exclude), "q": strings.TrimSpace(in.Q), "limit": limit, "offset": in.Offset})
		if err != nil {
			return fmt.Errorf("graph/layer/collapse/children: %w", err)
		}
		under := &CollapseHidden{LaID: in.LaID, Kind: "fan", Items: []CollapseHiddenItem{}, HasMore: page.HasMore}
		for _, it := range page.Items {
			item := CollapseHiddenItem{LaID: it.LaID, AccessDenied: it.AccessDenied}
			if it.Actor != nil {
				item.ActorID = it.Actor.ID
				if it.Actor.Title != nil {
					item.Title = *it.Actor.Title
				}
			}
			under.Items = append(under.Items, item)
		}
		view.Under = under
		return nil
	}
	if in.Q != "" {
		return errors.New("q searches a fan's hidden children; laId is not a folded fan")
	}
	var hidden []int64
	if resolved != nil {
		for _, h := range resolved.Hidden {
			if !h.Fan && h.Root == in.LaID {
				hidden = append(hidden, h.LaID)
			}
		}
	}
	folded := slices.ContainsFunc(view.Folds, func(f CollapseFold) bool { return f.LaID == in.LaID && !f.Gone })
	if !folded {
		return fmt.Errorf("laId %d is neither a collapsed actor nor a folded fan of this layer (see folds and fans)", in.LaID)
	}
	under := &CollapseHidden{LaID: in.LaID, Kind: "fold", Items: []CollapseHiddenItem{}}
	if in.Offset < len(hidden) {
		end := min(in.Offset+limit, len(hidden))
		under.HasMore = end < len(hidden)
		page := hidden[in.Offset:end]
		nodes, err := c.nodeTitles(ctx, view.LayerID, page)
		if err != nil {
			return err
		}
		for _, laID := range page {
			n := nodes[laID]
			under.Items = append(under.Items, CollapseHiddenItem{LaID: laID, ActorID: n.ID, Title: n.title(), AccessDenied: n.AccessDenied})
		}
	}
	view.Under = under
	return nil
}

// CollapseCountInput previews a fold by condition.
type CollapseCountInput struct {
	LayerID   string            `json:"layerId"`
	LaID      int64             `json:"laId"`
	Condition CollapseCondition `json:"condition"`
}

// CollapseCount is how many direct children the condition matches, of all of them.
type CollapseCount struct {
	Count      int  `json:"count"`
	ChildCount int  `json:"childCount"`
	Capped     bool `json:"capped,omitempty"`
}

// CountLayerCollapse previews which direct children a condition would fold (graph/layer/collapse/count).
func (c *Client) CountLayerCollapse(ctx context.Context, in CollapseCountInput) (CollapseCount, error) {
	if !c.CollapseReadsEnabled() {
		return CollapseCount{}, errors.New("layer collapse is not available in this profile")
	}
	if !validID(in.LayerID) {
		return CollapseCount{}, errors.New("layerId required: the current layer of the request's context")
	}
	return call[map[string]any, CollapseCount](ctx, c, "graph/layer/collapse/count",
		map[string]any{"accId": c.workspace(), "actorId": in.LayerID, "laId": in.LaID, "condition": in.Condition})
}

// CollapseUpdateInput changes the person's collapse (graph/layer/collapse/state/update).
type CollapseUpdateInput struct {
	LayerID     string                `json:"layerId"`
	ExpandAll   bool                  `json:"expandAll,omitempty"`
	Expand      []int64               `json:"expand,omitempty"`
	Collapse    []int64               `json:"collapse,omitempty"`
	Conditional []CollapseConditional `json:"conditional,omitempty"`
	FanReset    bool                  `json:"fanReset,omitempty"`
	FanOpen     []int64               `json:"fanOpen,omitempty"`
	FanClose    []int64               `json:"fanClose,omitempty"`
}

// CollapseUpdated is the person's whole collapse after the change.
type CollapseUpdated struct {
	LayerID            string                `json:"layerId"`
	Roots              []int64               `json:"roots"`
	Conditional        []CollapseConditional `json:"conditional"`
	FanOpen            []int64               `json:"fanOpen"`
	FanShown           []CollapseFanShown    `json:"fanShown"`
	MaxVisibleChildren int                   `json:"maxVisibleChildren"`
	Changed            bool                  `json:"changed"`
	// OpenTabs counts the person's tabs with the layer open, which show the change at once; 0: it
	// shows when they open the layer.
	OpenTabs int `json:"openTabs"`
}

type collapseUpdateRequest struct {
	AccID       string                `json:"accId"`
	ActorID     string                `json:"actorId"`
	ExpandAll   bool                  `json:"expandAll,omitempty"`
	Expand      []int64               `json:"expand,omitempty"`
	Collapse    []int64               `json:"collapse,omitempty"`
	Conditional []CollapseConditional `json:"conditional,omitempty"`
	FanReset    bool                  `json:"fanReset,omitempty"`
	FanOpen     []int64               `json:"fanOpen,omitempty"`
	FanClose    []int64               `json:"fanClose,omitempty"`
}

// errCollapseAPIKey: a collapse is a person's own view; an API key would change the API user's.
var errCollapseAPIKey = errors.New("collapse is the person's own Graph Editor view, and this process runs with an API key: " +
	"a change would fold the API user's view, which no person sees. Read it with layer_collapse_get; to fold for a " +
	"person, run with that person's token (AI Console, or sign in with login)")

// UpdateLayerCollapse changes the person's own collapse of the layer. Refused with an API key.
func (c *Client) UpdateLayerCollapse(ctx context.Context, in CollapseUpdateInput) (CollapseUpdated, error) {
	if e := consoleOnly(c); e != nil {
		return CollapseUpdated{}, e
	}
	if c.collapseOwner() != "you" {
		return CollapseUpdated{}, errCollapseAPIKey
	}
	if !validID(in.LayerID) {
		return CollapseUpdated{}, errors.New("layerId required: the current layer of the request's context")
	}
	summaries := map[int64]string{}
	for i, cond := range in.Conditional {
		if strings.TrimSpace(cond.Summary) != "" {
			continue
		}
		title, ok := summaries[cond.Condition.FormID]
		if !ok {
			form, err := call[map[string]any, struct {
				Title string `json:"title"`
			}](ctx, c, "form/get", map[string]any{"accId": c.workspace(), "formId": cond.Condition.FormID})
			if err != nil {
				return CollapseUpdated{}, fmt.Errorf("conditional[%d]: form/get %d: %w", i, cond.Condition.FormID, err)
			}
			title = form.Title
			summaries[cond.Condition.FormID] = title
		}
		in.Conditional[i].Summary = collapseSummary(title, cond.Condition)
	}
	req := collapseUpdateRequest{AccID: c.workspace(), ActorID: in.LayerID, ExpandAll: in.ExpandAll, Expand: in.Expand, Collapse: in.Collapse,
		Conditional: in.Conditional, FanReset: in.FanReset, FanOpen: in.FanOpen, FanClose: in.FanClose}
	out, err := call[collapseUpdateRequest, collapseState](ctx, c, "graph/layer/collapse/state/update", req)
	if err != nil {
		return CollapseUpdated{}, fmt.Errorf("graph/layer/collapse/state/update: %w", err)
	}
	return CollapseUpdated{LayerID: in.LayerID, Roots: out.Roots, Conditional: out.Conditional, FanOpen: out.FanOpen, FanShown: out.FanShown,
		MaxVisibleChildren: out.MaxVisibleChildren, Changed: out.Changed != nil && *out.Changed, OpenTabs: out.OpenTabs}, nil
}

// collapseSummary is the words a fold by condition is shown with when the caller gave none:
// "<template>: <q>" and the balance bounds, at most 500 characters (the backend's bound).
func collapseSummary(title string, cond CollapseCondition) string {
	parts := []string{}
	if q := strings.TrimSpace(cond.Q); q != "" {
		parts = append(parts, q)
	}
	switch {
	case cond.AmountFrom != nil && cond.AmountTo != nil:
		parts = append(parts, fmt.Sprintf("balance %g..%g", *cond.AmountFrom, *cond.AmountTo))
	case cond.AmountFrom != nil:
		parts = append(parts, fmt.Sprintf("balance ≥ %g", *cond.AmountFrom))
	case cond.AmountTo != nil:
		parts = append(parts, fmt.Sprintf("balance ≤ %g", *cond.AmountTo))
	}
	s := title
	if len(parts) > 0 {
		s += ": " + strings.Join(parts, "; ")
	}
	if r := []rune(s); len(r) > 500 {
		s = string(r[:499]) + "…"
	}
	return s
}
