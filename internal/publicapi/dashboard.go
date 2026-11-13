package publicapi

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// DashboardQuery is what chart_data_get passes on to public dashboard/get besides the chart.
type DashboardQuery struct {
	From           *int64   `json:"from,omitempty"`
	To             *int64   `json:"to,omitempty"`
	Interval       string   `json:"interval,omitempty"`
	TimezoneOffset *int64   `json:"timezoneOffset,omitempty"`
	BalanceOnDate  bool     `json:"balanceOnDate,omitempty"`
	CounterType    string   `json:"counterType,omitempty"`
	ActorIDs       []string `json:"actorIds,omitempty"`
	With           []string `json:"with,omitempty"`
}

// DashboardInput is the chart_data_get tool input: ChartID is the dashboard actor.
type DashboardInput struct {
	ChartID string `json:"chartId"`
	DashboardQuery
}

type dashboardRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
	DashboardQuery
}

// DashboardPoint is one interval bucket: the movement (or transaction count) inside it. Buckets
// without movement are absent.
type DashboardPoint struct {
	BucketStart int64   `json:"bucketStart"`
	Value       float64 `json:"value"`
}

// DashboardSeries follows sim-api service_graph.DashboardAccountValue: one account the chart shows.
type DashboardSeries struct {
	ActorID       string           `json:"actorId"`
	ActorTitle    string           `json:"actorTitle"`
	ActorColor    *string          `json:"actorColor,omitempty"`
	NameID        string           `json:"nameId"`
	AccountName   string           `json:"accountName"`
	CurrencyID    int64            `json:"currencyId"`
	CurrencyName  string           `json:"currencyName"`
	Precision     int64            `json:"precision"`
	IncomeType    string           `json:"incomeType"`
	AccountType   string           `json:"accountType"`
	Color         *string          `json:"color,omitempty"`
	CustomName    *string          `json:"customName,omitempty"`
	ShowTrendLine bool             `json:"showTrendLine"`
	ErrorFormula  bool             `json:"errorFormula"`
	Value         float64          `json:"value"`
	Data          []DashboardPoint `json:"data,omitempty"`
}

// DashboardTotal follows sim-api service_graph.DashboardTotal: every series summed.
type DashboardTotal struct {
	Value float64          `json:"value"`
	Data  []DashboardPoint `json:"data,omitempty"`
}

// DashboardData is the dashboard/get result: the series, and Total with with:["total"].
type DashboardData struct {
	Data  []DashboardSeries `json:"data"`
	Total *DashboardTotal   `json:"total,omitempty"`
}

// DashboardReadsEnabled is true in the console profiles, with the account reads, and the workspace
// graph-read profile, with the graph reads: a chart is read on its layer. Like the parity reads it
// is not in graph-create, and an actor-bound graph-read process has only its layer's counters.
func (c *Client) DashboardReadsEnabled() bool {
	return c.ConsoleEnabled() || (c.config.Profile == "graph-read" && c.config.ActorID == "")
}

// GetDashboard reads what a chart shows through public dashboard/get (actors.readonly, view on
// the chart). Period, interval and with rules are the backend's: its invalidParams says which.
func (c *Client) GetDashboard(ctx context.Context, in DashboardInput) (DashboardData, error) {
	if !c.DashboardReadsEnabled() {
		return DashboardData{}, errors.New("chart data is not available in this profile")
	}
	if strings.TrimSpace(in.ChartID) == "" || len(in.ChartID) > 255 {
		return DashboardData{}, errors.New("chartId required, maximum 255 bytes")
	}
	out, err := call[dashboardRequest, DashboardData](ctx, c, "dashboard/get", dashboardRequest{AccID: c.workspace(), ActorID: in.ChartID, DashboardQuery: in.DashboardQuery})
	if err != nil {
		return DashboardData{}, err
	}
	if out.Data == nil {
		return DashboardData{}, errors.New("invalid dashboard result: no series list")
	}
	for _, s := range out.Data {
		if s.ActorID == "" || s.NameID == "" || s.CurrencyID < 1 {
			return DashboardData{}, errors.New("invalid dashboard result: series without its account")
		}
	}
	if slices.Contains(in.With, "total") && out.Total == nil {
		return DashboardData{}, errors.New("invalid dashboard result: total asked for but missing")
	}
	return out, nil
}
