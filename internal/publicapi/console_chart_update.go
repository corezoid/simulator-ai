package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ChartUpdateInput is legacy updateChart: change a chart's settings, its source (other accounts, or
// another filter), and with laId how its placement on layerId is drawn. Empty keeps what is stored.
type ChartUpdateInput struct {
	ChartID string `json:"chartId"`
	ChartSettings
	ChartFilterInput
	// IncomeType sets every series of a manual chart (or the new accounts that leave theirs out).
	IncomeType string         `json:"incomeType,omitempty"`
	Accounts   []ChartAccount `json:"accounts,omitempty"`
	LayerID    string         `json:"layerId,omitempty"`
	LaID       int64          `json:"laId,omitempty"`
	Expand     *bool          `json:"expand,omitempty"`
	Width      float64        `json:"width,omitempty"`
	Height     float64        `json:"height,omitempty"`
}

// ChartUpdated is the chart's resulting source; Warnings are values stored other than as sent and
// a filter the chart stopped using.
type ChartUpdated struct {
	ChartID       string `json:"chartId"`
	SourceType    string `json:"sourceType"`
	FilterActorID string `json:"filterActorId,omitempty"`
	ChartSettings
	LaID     int64    `json:"laId,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func (in ChartUpdateInput) changesSource() bool {
	return in.ChartSettings != (ChartSettings{}) || in.IncomeType != "" || in.Top != 0 || in.FilterActorID != "" || in.FilterTitle != "" ||
		in.changesFilter() || len(in.Accounts) > 0
}

func (in ChartUpdateInput) validate() error {
	switch {
	case !uuidPattern.MatchString(in.ChartID):
		return errors.New("chartId must be the chart actor's id")
	case in.Top < 0 || in.Top > 100 || in.SourceFormID < 0 || in.CurrencyID < 0 || in.LaID < 0 || len(in.Accounts) > 100:
		return errors.New("top is 1..100; sourceFormId, currencyId and laId are positive; at most 100 accounts")
	case !in.changesSource() && in.LaID == 0:
		return errors.New("nothing to change: pass a chart setting, the filter or accounts, and/or laId for the placement")
	case len(in.Accounts) > 0 && (in.FilterActorID != "" || in.FilterTitle != "" || in.changesFilter() || in.Top != 0):
		return errors.New("accounts (a manual chart) exclude the filter params filterActorId, filterTitle, sourceFormId, accountNameId, currencyId and top")
	case in.LaID == 0 && (in.LayerID != "" || in.Expand != nil || in.Width != 0 || in.Height != 0):
		return errors.New("layerId, expand, width and height change the chart's placement: pass its laId too")
	case in.LaID != 0 && !uuidPattern.MatchString(in.LayerID):
		return errors.New("laId needs layerId, the layer it is on")
	case (in.Width == 0) != (in.Height == 0) || (in.Width != 0 && (in.Width < chartWidth || in.Height < chartHeight)):
		return fmt.Errorf("width and height go together, at least %d x %d", chartWidth, chartHeight)
	}
	return nil
}

type actorDataUpdate struct {
	AccID   string          `json:"accId"`
	ActorID string          `json:"actorId"`
	Data    json.RawMessage `json:"data"`
	Fields  []string        `json:"fields"`
}
type layerElementsRequest struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	Nodes   []int64  `json:"nodes"`
	Fields  []string `json:"fields"`
}
type layerElementsWire struct {
	Nodes []struct {
		LaID int64  `json:"laId"`
		ID   string `json:"id"`
	} `json:"nodes"`
}
type actorSettingsRequest struct {
	AccID         string             `json:"accId"`
	ActorID       string             `json:"actorId"`
	LaActorID     int64              `json:"laActorId"`
	LayerSettings chartLayerSettings `json:"layerSettings"`
}

// UpdateChart rewrites a chart's source with its stored keys kept, creating a new ActorFilters
// actor when the filter changes, and with laId merges the chart drawing into that placement.
func (c *Client) UpdateChart(ctx context.Context, in ChartUpdateInput) (ChartUpdated, error) {
	if e := consoleOnly(c); e != nil {
		return ChartUpdated{}, e
	}
	if err := in.validate(); err != nil {
		return ChartUpdated{}, err
	}
	chart, err := c.readChartActor(ctx, in.ChartID)
	if err != nil {
		return ChartUpdated{}, err
	}
	dashboards, err := c.systemFormID(ctx, "Dashboards")
	if err != nil {
		return ChartUpdated{}, err
	}
	if chart.FormID != dashboards {
		return ChartUpdated{}, fmt.Errorf("actor %s is not a chart (an actor of the Dashboards template)", in.ChartID)
	}
	if in.LaID != 0 {
		if err = c.checkChartPlacement(ctx, in); err != nil {
			return ChartUpdated{}, err
		}
	}
	out, err := c.updateChartSource(ctx, in, chart)
	if err != nil || in.LaID == 0 {
		return out, err
	}
	settings := chartLayerSettings{ExpandType: "chart", Expand: in.Expand == nil || *in.Expand, Width: in.Width, Height: in.Height}
	if _, err = call[actorSettingsRequest, json.RawMessage](ctx, c, "graph/layer/actor_settings/save", actorSettingsRequest{AccID: c.workspace(),
		ActorID: in.LayerID, LaActorID: in.LaID, LayerSettings: settings}); err != nil {
		switch {
		case in.changesSource() && out.FilterActorID != "":
			err = fmt.Errorf("%w (the chart's source was saved; it now reads the ActorFilters actor %s)", err, out.FilterActorID)
		case in.changesSource():
			err = fmt.Errorf("%w (the chart's source was saved)", err)
		}
		return ChartUpdated{}, err
	}
	out.LaID = in.LaID
	return out, nil
}

// checkChartPlacement makes sure laId on layerId draws this chart, not another actor.
func (c *Client) checkChartPlacement(ctx context.Context, in ChartUpdateInput) error {
	r, err := call[layerElementsRequest, layerElementsWire](ctx, c, "graph/layer/elements/list", layerElementsRequest{AccID: c.workspace(),
		ActorID: in.LayerID, Nodes: []int64{in.LaID}, Fields: []string{"id"}})
	if err != nil {
		return err
	}
	if len(r.Nodes) != 1 || r.Nodes[0].ID != in.ChartID {
		return fmt.Errorf("laId %d on layer %s is not a placement of the chart %s", in.LaID, in.LayerID, in.ChartID)
	}
	return nil
}

func (c *Client) updateChartSource(ctx context.Context, in ChartUpdateInput, chart chartActor) (ChartUpdated, error) {
	if !in.changesSource() {
		cur, _, err := parseStoredSource(chart.Data.Source)
		if err != nil {
			return ChartUpdated{}, err
		}
		out := ChartUpdated{ChartID: in.ChartID, SourceType: cur.SourceType, ChartSettings: cur.ChartSettings}
		if cur.DynamicSource != nil {
			out.FilterActorID = cur.DynamicSource.ID
		}
		return out, nil
	}
	p, err := planSource(ctx, in, chart, c.readFilterActor)
	if err != nil {
		return ChartUpdated{}, err
	}
	if p.newFilter != nil {
		if p.filterID, err = c.createFilterActor(ctx, p.filterTitle, *p.newFilter); err != nil {
			return ChartUpdated{}, err
		}
		setJSON(p.keys, "dynamicSource", p.dynamicSource(p.filterID, p.filterTitle, *p.newFilter))
	}
	if _, err = call[actorDataUpdate, json.RawMessage](ctx, c, "actor/update", actorDataUpdate{AccID: c.workspace(), ActorID: in.ChartID,
		Data: sourceData(p.keys), Fields: []string{"id"}}); err != nil {
		if p.newFilter != nil {
			err = orphan(err, "ActorFilters actor", p.filterID)
		}
		return ChartUpdated{}, fmt.Errorf("save the chart's source: %w", err)
	}
	out := ChartUpdated{ChartID: in.ChartID, SourceType: p.sourceType, FilterActorID: p.filterID, ChartSettings: p.settings, Warnings: p.warnings}
	if p.prevFilterID != "" && p.prevFilterID != p.filterID {
		out.Warnings = append(out.Warnings, "the chart no longer uses the ActorFilters actor "+p.prevFilterID+": delete it if no other chart does")
	}
	return out, nil
}

func setJSON(m map[string]json.RawMessage, key string, v any) {
	m[key], _ = json.Marshal(v)
}

func rawObject(raw json.RawMessage) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return map[string]json.RawMessage{}
	}
	return m
}
