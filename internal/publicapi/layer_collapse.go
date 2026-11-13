package publicapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// A person's collapse of a layer is their own Graph Editor view: the actors they folded whole or by
// condition (only the direct children a condition matches fold) and the wide fans they opened. sim-api
// keeps it in the person's user settings (graph/layer/collapse/state/get|update); what it hides is
// computed by graph/layer/collapse/resolve; a fan's hidden children are listed by
// graph/layer/collapse/children. The layer's fan limit ("show at most N children") is the layer's
// own setting (data.maxVisibleChildren), shared by every viewer: layer_settings_update writes it.

// CollapseCondition is which direct children a fold by condition takes (sim-api e_graph.CollapseCondition).
type CollapseCondition struct {
	FormID        int64    `json:"formId"`
	Q             string   `json:"q,omitempty"`
	AccountNameID string   `json:"accountNameId,omitempty"`
	CurrencyID    int64    `json:"currencyId,omitempty"`
	IncomeType    string   `json:"incomeType,omitempty"`
	AmountFrom    *float64 `json:"amountFrom,omitempty"`
	AmountTo      *float64 `json:"amountTo,omitempty"`
}

// CollapseConditional is one fold by condition as saved.
type CollapseConditional struct {
	LaID      int64             `json:"laId"`
	Condition CollapseCondition `json:"condition"`
	Summary   string            `json:"summary,omitempty"`
}

// CollapseFanShown is one fan's children shown one by one.
type CollapseFanShown struct {
	LaID     int64   `json:"laId"`
	Children []int64 `json:"children"`
}

// collapseState is graph/layer/collapse/state/get's answer (and update's, with Changed and OpenTabs).
type collapseState struct {
	Roots              []int64               `json:"roots"`
	Conditional        []CollapseConditional `json:"conditional"`
	FanOpen            []int64               `json:"fanOpen"`
	FanShown           []CollapseFanShown    `json:"fanShown"`
	MaxVisibleChildren int                   `json:"maxVisibleChildren"`
	Changed            *bool                 `json:"changed,omitempty"`
	OpenTabs           int                   `json:"openTabs"`
}

// CollapseGetInput reads a layer's collapse; with LaID also one page of what hides under it.
type CollapseGetInput struct {
	LayerID string `json:"layerId"`
	LaID    int64  `json:"laId,omitempty"`
	Q       string `json:"q,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Offset  int    `json:"offset,omitempty"`
}

// CollapseFold is one collapsed actor and what it folds.
type CollapseFold struct {
	LaID        int64              `json:"laId"`
	Title       string             `json:"title,omitempty"`
	Conditional bool               `json:"conditional"`
	Condition   *CollapseCondition `json:"condition,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	// Visible is false when another collapsed actor hides this one; Gone when it left the layer.
	Visible            bool `json:"visible"`
	Gone               bool `json:"gone,omitempty"`
	HiddenCount        int  `json:"hiddenCount"`
	ChildCount         int  `json:"childCount"`
	MatchedChildCount  int  `json:"matchedChildCount"`
	SharedVisibleCount int  `json:"sharedVisibleCount"`
}

// CollapseFan is one wide fan: a parent with more direct children than the layer's limit.
type CollapseFan struct {
	LaID        int64  `json:"laId"`
	Title       string `json:"title,omitempty"`
	ChildCount  int    `json:"childCount"`
	HiddenCount int    `json:"hiddenCount"`
	Opened      bool   `json:"opened,omitempty"`
}

// CollapseHiddenItem is one placement hidden under a fold or a fan.
type CollapseHiddenItem struct {
	LaID         int64  `json:"laId"`
	ActorID      string `json:"actorId,omitempty"`
	Title        string `json:"title,omitempty"`
	AccessDenied bool   `json:"accessDenied,omitempty"`
}

// CollapseHidden is one page of what hides under LaID.
type CollapseHidden struct {
	LaID    int64                `json:"laId"`
	Kind    string               `json:"kind"`
	Items   []CollapseHiddenItem `json:"items"`
	HasMore bool                 `json:"hasMore"`
}

// CollapseView is a layer's collapse as the person sees it.
type CollapseView struct {
	LayerID string `json:"layerId"`
	// Owner is whose view this is: "you" (the person of the token), or "apiUser" with an API key.
	Owner              string             `json:"owner"`
	MaxVisibleChildren int                `json:"maxVisibleChildren"`
	Folds              []CollapseFold     `json:"folds"`
	Fans               []CollapseFan      `json:"fans"`
	FanShown           []CollapseFanShown `json:"fanShown,omitempty"`
	HiddenTotal        int                `json:"hiddenTotal"`
	Truncated          bool               `json:"truncated,omitempty"`
	Under              *CollapseHidden    `json:"under,omitempty"`
}

type layerRef struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
}

type collapseZone struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

type collapseResolveRequest struct {
	AccID              string                `json:"accId"`
	ActorID            string                `json:"actorId"`
	Roots              []int64               `json:"roots"`
	Conditional        []CollapseConditional `json:"conditional,omitempty"`
	MaxVisibleChildren int                   `json:"maxVisibleChildren,omitempty"`
	Zone               *collapseZone         `json:"zone,omitempty"`
	FanOpen            []int64               `json:"fanOpen,omitempty"`
	FanShown           []CollapseFanShown    `json:"fanShown,omitempty"`
}

type collapseResolved struct {
	Hidden []struct {
		LaID int64 `json:"laId"`
		Root int64 `json:"root"`
		Fan  bool  `json:"fan"`
	} `json:"hidden"`
	Roots []struct {
		LaID               int64 `json:"laId"`
		Conditional        bool  `json:"conditional"`
		Visible            bool  `json:"visible"`
		HiddenCount        int   `json:"hiddenCount"`
		SharedVisibleCount int   `json:"sharedVisibleCount"`
		ChildCount         int   `json:"childCount"`
		MatchedChildCount  int   `json:"matchedChildCount"`
	} `json:"roots"`
	UnknownRoots []int64 `json:"unknownRoots"`
	Fans         []struct {
		LaID       int64 `json:"laId"`
		ChildCount int   `json:"childCount"`
	} `json:"fans"`
	Truncated bool `json:"truncated"`
}

// CollapseReadsEnabled says the read tools are on in the console and in the unbound graph-read profile,
// as chart_data_get; graph-create stays the strict create catalog.
func (c *Client) CollapseReadsEnabled() bool {
	return c.ConsoleEnabled() || (c.config.Profile == "graph-read" && c.config.ActorID == "")
}

// collapseOwner is whose collapse the credential reads and writes: a person's token is that person's
// view, an API key the API user's own, which no person sees.
func (c *Client) collapseOwner() string {
	if c.CredentialMode() == "apiKey" || strings.HasPrefix(c.config.Authorization, "ApiKey ") {
		return "apiUser"
	}
	return "you"
}

// GetLayerCollapse reads the layer's collapse and resolves what it hides.
func (c *Client) GetLayerCollapse(ctx context.Context, in CollapseGetInput) (CollapseView, error) {
	if !c.CollapseReadsEnabled() {
		return CollapseView{}, errors.New("layer collapse is not available in this profile")
	}
	if !validID(in.LayerID) {
		return CollapseView{}, errors.New("layerId required: the current layer of the request's context")
	}
	ws := c.workspace()
	state, err := call[layerRef, collapseState](ctx, c, "graph/layer/collapse/state/get", layerRef{AccID: ws, ActorID: in.LayerID})
	if err != nil {
		return CollapseView{}, fmt.Errorf("graph/layer/collapse/state/get: %w", err)
	}
	view := CollapseView{LayerID: in.LayerID, Owner: c.collapseOwner(), MaxVisibleChildren: state.MaxVisibleChildren,
		Folds: []CollapseFold{}, Fans: []CollapseFan{}, FanShown: state.FanShown}
	if len(state.Roots)+len(state.Conditional) == 0 && state.MaxVisibleChildren == 0 {
		return view, c.collapseUnder(ctx, &view, in, nil, nil)
	}
	req := collapseResolveRequest{AccID: ws, ActorID: in.LayerID, Roots: nonNil(state.Roots), Conditional: state.Conditional}
	if state.MaxVisibleChildren > 0 {
		zone, err := c.layerZone(ctx, in.LayerID)
		if err != nil {
			return CollapseView{}, err
		}
		if zone != nil {
			req.MaxVisibleChildren, req.Zone, req.FanOpen, req.FanShown = state.MaxVisibleChildren, zone, state.FanOpen, state.FanShown
		}
	}
	if len(req.Roots)+len(req.Conditional) == 0 && req.Zone == nil {
		return view, c.collapseUnder(ctx, &view, in, nil, nil)
	}
	resolved, err := call[collapseResolveRequest, collapseResolved](ctx, c, "graph/layer/collapse/resolve", req)
	if err != nil {
		return CollapseView{}, fmt.Errorf("graph/layer/collapse/resolve: %w", err)
	}
	view.HiddenTotal, view.Truncated = len(resolved.Hidden), resolved.Truncated
	conditions := map[int64]CollapseConditional{}
	for _, cond := range state.Conditional {
		conditions[cond.LaID] = cond
	}
	for _, r := range resolved.Roots {
		fold := CollapseFold{LaID: r.LaID, Conditional: r.Conditional, Visible: r.Visible, HiddenCount: r.HiddenCount,
			ChildCount: r.ChildCount, MatchedChildCount: r.MatchedChildCount, SharedVisibleCount: r.SharedVisibleCount}
		if cond, ok := conditions[r.LaID]; ok && r.Conditional {
			fold.Condition, fold.Summary = &cond.Condition, cond.Summary
		}
		view.Folds = append(view.Folds, fold)
	}
	for _, laID := range resolved.UnknownRoots {
		view.Folds = append(view.Folds, CollapseFold{LaID: laID, Gone: true})
	}
	fanHidden := map[int64]int{}
	for _, h := range resolved.Hidden {
		if h.Fan {
			fanHidden[h.Root]++
		}
	}
	for _, f := range resolved.Fans {
		view.Fans = append(view.Fans, CollapseFan{LaID: f.LaID, ChildCount: f.ChildCount, HiddenCount: fanHidden[f.LaID], Opened: slices.Contains(state.FanOpen, f.LaID)})
	}
	if err := c.collapseTitles(ctx, &view); err != nil {
		return CollapseView{}, err
	}
	return view, c.collapseUnder(ctx, &view, in, &resolved, state.FanShown)
}

// layerZone is the layer's whole extent, the zone whose fans resolve evaluates; nil for an empty layer.
func (c *Client) layerZone(ctx context.Context, layerID string) (*collapseZone, error) {
	out, err := call[layerRef, struct {
		Extent *collapseZone `json:"extent"`
	}](ctx, c, "graph/layer/extent/get", layerRef{AccID: c.workspace(), ActorID: layerID})
	if err != nil {
		return nil, fmt.Errorf("graph/layer/extent/get: %w", err)
	}
	if out.Extent != nil {
		// The extent bounds the placements' positions; a cell of margin keeps one on the edge inside.
		out.Extent.X1, out.Extent.Y1, out.Extent.X2, out.Extent.Y2 = out.Extent.X1-gridCell, out.Extent.Y1-gridCell, out.Extent.X2+gridCell, out.Extent.Y2+gridCell
	}
	return out.Extent, nil
}

func nonNil(v []int64) []int64 {
	if v == nil {
		return []int64{}
	}
	return v
}
