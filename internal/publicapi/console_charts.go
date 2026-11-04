package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// ChartFilterInput makes or names the ActorFilters actor of an actorFilter chart: an existing one
// by FilterActorID, or a new one from SourceFormID, AccountNameID and CurrencyID (titled
// FilterTitle). Top is how many actors the chart shows.
type ChartFilterInput struct {
	FilterActorID string `json:"filterActorId,omitempty"`
	FilterTitle   string `json:"filterTitle,omitempty"`
	SourceFormID  int64  `json:"sourceFormId,omitempty"`
	AccountNameID string `json:"accountNameId,omitempty"`
	CurrencyID    int64  `json:"currencyId,omitempty"`
	Top           int    `json:"top,omitempty"`
}

func (f ChartFilterInput) given() ChartFilter {
	return ChartFilter{FormID: f.SourceFormID, AccountNameID: f.AccountNameID, CurrencyID: f.CurrencyID}
}

// changesFilter: any of the three ids asks for a new ActorFilters actor.
func (f ChartFilterInput) changesFilter() bool {
	return f.SourceFormID != 0 || f.AccountNameID != "" || f.CurrencyID != 0
}

// ChartCreateInput is legacy createChart: a chart on a layer, of Accounts (manual) or of a filter.
type ChartCreateInput struct {
	LayerID     string `json:"layerId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ChartSettings
	ChartFilterInput
	Accounts []ChartAccount `json:"accounts,omitempty"`
	Position GraphPoint     `json:"position"`
}

// ChartCreated names what was made; Warnings are settings stored other than as sent.
type ChartCreated struct {
	ChartID       string   `json:"chartId"`
	FilterActorID string   `json:"filterActorId,omitempty"`
	LaID          int64    `json:"laId"`
	Warnings      []string `json:"warnings,omitempty"`
}

// chartLayerSettings draws a placement as an open chart. Legacy wrote the box as offset
// {left: 300, right: 300, top: 200, bottom: 200} around the 50-unit cell; sim-api types offset as
// {x, y} and stores that shape as {0, 0}, so the same 650 x 450 box is sent as width and height.
type chartLayerSettings struct {
	ExpandType string  `json:"expandType"`
	Expand     bool    `json:"expand"`
	Width      float64 `json:"width,omitempty"`
	Height     float64 `json:"height,omitempty"`
}

const chartWidth, chartHeight = 650, 450

type chartPlacementData struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Position      GraphPoint         `json:"position"`
	LayerSettings chartLayerSettings `json:"layerSettings"`
}
type chartPlacementItem struct {
	Action string             `json:"action"`
	Data   chartPlacementData `json:"data"`
}
type chartPlacementRequest struct {
	AccID   string               `json:"accId"`
	ActorID string               `json:"actorId"`
	Items   []chartPlacementItem `json:"items"`
}

// chartActor is the projection the chart tools read of a chart or an ActorFilters actor.
type chartActor struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	FormID int64  `json:"formId"`
	Data   struct {
		Source json.RawMessage `json:"source"`
		Filter json.RawMessage `json:"filter"`
	} `json:"data"`
}

func (c *Client) readChartActor(ctx context.Context, id string) (chartActor, error) {
	return call[actorGetRequest, chartActor](ctx, c, "actor/get", actorGetRequest{AccID: c.config.Workspace, ActorID: id, Fields: []string{"id", "title", "formId", "data"}})
}

// readFilterActor returns an existing ActorFilters actor's title and what it selects.
func (c *Client) readFilterActor(ctx context.Context, id string) (string, ChartFilter, error) {
	if !uuidPattern.MatchString(id) {
		return "", ChartFilter{}, errors.New("filterActorId must be the ActorFilters actor's id")
	}
	a, err := c.readChartActor(ctx, id)
	if err != nil {
		return "", ChartFilter{}, fmt.Errorf("read the ActorFilters actor %s: %w", id, err)
	}
	return a.Title, readFilter(a.Data.Filter), nil
}

// createFilterActor makes the ActorFilters actor an actorFilter chart reads its actors from.
func (c *Client) createFilterActor(ctx context.Context, title string, f ChartFilter) (string, error) {
	formID, err := c.systemFormID(ctx, "ActorFilters")
	if err != nil {
		return "", err
	}
	a, err := c.CreateActor(ctx, ActorCreateInput{FormID: formID, Title: title, Data: filterData(f)})
	if err != nil {
		return "", fmt.Errorf("create the ActorFilters actor: %w", err)
	}
	return *a.ID, nil
}

// sourceData is a chart actor's data: its source as a JSON string, as the wizard stores it.
func sourceData(source any) json.RawMessage {
	b, _ := json.Marshal(source)
	data, _ := json.Marshal(map[string]string{"source": string(b)})
	return data
}

func (in ChartCreateInput) validate() error {
	switch {
	case !uuidPattern.MatchString(in.LayerID):
		return errors.New("layerId must be the layer actor's id")
	case !validTitle(in.Title, false) || utf8.RuneCountInString(in.Description) > 10000:
		return errors.New("a 1..1000 character title required")
	case in.Top < 0 || in.Top > 100:
		return errors.New("top is 1..100")
	case len(in.Accounts) > 100:
		return errors.New("at most 100 accounts")
	case len(in.Accounts) > 0 && (in.FilterActorID != "" || in.FilterTitle != "" || in.changesFilter() || in.Top != 0):
		return errors.New("accounts (a manual chart) exclude the filter params filterActorId, filterTitle, sourceFormId, accountNameId, currencyId and top")
	case len(in.Accounts) == 0 && in.FilterActorID != "" && in.changesFilter():
		return errors.New("filterActorId reuses a filter: leave out sourceFormId, accountNameId and currencyId")
	case len(in.Accounts) == 0 && in.FilterActorID == "" && (!in.given().complete() || !uuidPattern.MatchString(in.AccountNameID)):
		return errors.New("pass accounts, filterActorId, or sourceFormId, accountNameId (account name id) and currencyId for a new filter")
	}
	return nil
}

// CreateChart creates a chart actor of the Dashboards template on a layer, opened as a chart. An
// actorFilter chart reuses filterActorId or first creates its ActorFilters actor.
func (c *Client) CreateChart(ctx context.Context, in ChartCreateInput) (ChartCreated, error) {
	if e := consoleOnly(c); e != nil {
		return ChartCreated{}, e
	}
	if err := in.validate(); err != nil {
		return ChartCreated{}, err
	}
	warnings, err := in.ChartSettings.normalize()
	if err != nil {
		return ChartCreated{}, err
	}
	accounts, err := normalizeAccounts(in.Accounts)
	if err != nil {
		return ChartCreated{}, err
	}
	dashboards, err := c.systemFormID(ctx, "Dashboards")
	if err != nil {
		return ChartCreated{}, err
	}
	out := ChartCreated{Warnings: warnings}
	var dynamic *dashboardDynamicSource
	newFilter := "" // an ActorFilters actor made by this call
	if len(accounts) == 0 {
		top := in.Top
		if top == 0 {
			top = 20
		}
		title, filter := in.FilterTitle, in.given()
		if in.FilterActorID != "" {
			stored, f, err := c.readFilterActor(ctx, in.FilterActorID)
			if err != nil {
				return ChartCreated{}, err
			}
			title, filter, out.FilterActorID = orDefault(title, stored), f, in.FilterActorID
		} else {
			title = orDefault(title, in.Title)
			if newFilter, err = c.createFilterActor(ctx, title, filter); err != nil {
				return ChartCreated{}, err
			}
			out.FilterActorID = newFilter
		}
		dynamic = newDynamicSource(out.FilterActorID, title, top, filter)
	}
	chart, err := c.CreateActor(ctx, ActorCreateInput{FormID: dashboards, Title: in.Title, Description: in.Description,
		Data: sourceData(newSource(in.ChartSettings, accounts, dynamic))})
	if err != nil {
		return ChartCreated{}, orphan(fmt.Errorf("create the chart actor: %w", err), "ActorFilters actor", newFilter)
	}
	out.ChartID = *chart.ID
	placed, err := call[chartPlacementRequest, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", chartPlacementRequest{AccID: c.config.Workspace, ActorID: in.LayerID,
		Items: []chartPlacementItem{{Action: "create", Data: chartPlacementData{ID: out.ChartID, Type: "node", Position: in.Position,
			LayerSettings: chartLayerSettings{ExpandType: "chart", Expand: true, Width: chartWidth, Height: chartHeight}}}}})
	if err == nil && len(placed.Nodes) != 1 {
		err = errors.New("graph/layer/actors/manage returned no placement")
	}
	if err != nil {
		return ChartCreated{}, orphan(fmt.Errorf("place the chart on layer %s: %w", in.LayerID, err), "chart actor", out.ChartID)
	}
	out.LaID = placed.Nodes[0].LaID
	return out, nil
}

// orphan names an actor a failed call left behind, so the agent can place or delete it.
func orphan(err error, what, id string) error {
	if id == "" {
		return err
	}
	return fmt.Errorf("%w (the %s %s was created and is unused: delete it or retry with it)", err, what, id)
}
