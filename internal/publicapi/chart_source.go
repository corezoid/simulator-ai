package publicapi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A chart is an actor of the Dashboards system template whose data.source holds its configuration
// as a JSON string, the shape the web app's dashboard wizard writes (legacy createChart). sim-api's
// dashboard/get reads the same string and fills what is left out (accountType fact, displayMode
// top). An actorFilter chart charts the top actors an ActorFilters actor selects; a manual one
// lists its series.

var (
	// The wizard's chart types without kpiCard, which needs a single-source config not built here.
	chartTypes        = []string{"line", "bar", "stackedBar", "pie", "doughnut", "funnel", "table", "polarArea", "radar"}
	chartCounterTypes = []string{"amount", "count"}
	// The wizard's ranges without custom, which needs rangeDates.
	chartRanges = []string{"allTime", "lastMinute", "last10Minutes", "lastHour", "today", "yesterday", "lastWeek", "lastWeekExcludeToday",
		"last14Days", "last14DaysExcludeToday", "lastMonth", "lastMonthExcludeToday", "previousMonth", "lastYear", "lastYearExcludeToday",
		"realTime", "lineRealTime"}
	chartOrderValues = []string{"default", "desc", "asc"}
	chartIncomeTypes = []string{"total", "credit", "debit"}
	// Interval charts: no sorting, no allTime, lineRealTime instead of realTime.
	intervalChartTypes = []string{"line", "stackedBar"}
	// chartSeriesPalette colours series the caller leaves uncoloured, in the wizard's order.
	chartSeriesPalette = []string{"#499894", "#59A14F", "#D37295", "#4E79A7", "#FF9D9A", "#B07AA1", "#B6992D", "#F1CE63", "#BAB0AC", "#79706E"}
	uuidPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// ChartSettings are the chart-wide choices; empty keeps the default (or, on update, the current).
type ChartSettings struct {
	ChartType   string `json:"chartType,omitempty"`
	CounterType string `json:"counterType,omitempty"`
	Range       string `json:"range,omitempty"`
	OrderValue  string `json:"orderValue,omitempty"`
}

// ChartAccount is one series of a manual chart: an account (name + currency) of an actor.
type ChartAccount struct {
	ActorID    string `json:"actorId"`
	NameID     string `json:"nameId"`
	CurrencyID int64  `json:"currencyId"`
	Color      string `json:"color,omitempty"`
	IncomeType string `json:"incomeType,omitempty"`
}

// ChartFilter is what an ActorFilters actor selects: actors of a template, ranked by one account.
type ChartFilter struct {
	FormID        int64  `json:"formId"`
	AccountNameID string `json:"accountNameId"`
	CurrencyID    int64  `json:"currencyId"`
}

func (f ChartFilter) complete() bool {
	return f.FormID > 0 && f.AccountNameID != "" && f.CurrencyID > 0
}

type dashboardSource struct {
	DefaultAccount         *json.RawMessage        `json:"defaultAccount"`
	Accounts               []dashboardAccount      `json:"accounts"`
	CounterType            string                  `json:"counterType"`
	ChartType              string                  `json:"chartType"`
	SourceType             string                  `json:"sourceType"`
	Range                  string                  `json:"range"`
	RangeDates             dashboardRangeDates     `json:"rangeDates"`
	ShowTotal              bool                    `json:"showTotal"`
	OrderValue             string                  `json:"orderValue"`
	Legend                 dashboardLegend         `json:"legend"`
	DisplayChartDataLabels bool                    `json:"displayChartDataLabels"`
	DynamicSource          *dashboardDynamicSource `json:"dynamicSource,omitempty"`
	ChartViewMode          string                  `json:"chartViewMode"`
}

type dashboardAccount struct {
	ActorID    string `json:"actorId"`
	Color      string `json:"color"`
	IncomeType string `json:"incomeType"`
	CurrencyID int64  `json:"currencyId"`
	NameID     string `json:"nameId"`
}

type dashboardRangeDates struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

// dashboardLegend: incomeType must be on when two series differ only by it, or they share a label.
type dashboardLegend struct {
	ActorTitle   bool `json:"actorTitle"`
	AccountName  bool `json:"accountName"`
	CurrencyName bool `json:"currencyName"`
	IncomeType   bool `json:"incomeType,omitempty"`
}

// dashboardDynamicSource points an actorFilter chart at its ActorFilters actor. top is a string,
// as the wizard writes it; filter is the wizard's snapshot of the filter, which sim-api ignores.
type dashboardDynamicSource struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Top           string           `json:"top"`
	GroupByFields []string         `json:"groupByFields"`
	Filter        *dashboardFilter `json:"filter,omitempty"`
}

type dashboardFilter struct {
	FormID        int64   `json:"formId"`
	SelectedForms []int64 `json:"selectedForms"`
	AccountNameID string  `json:"accountNameId"`
	CurrencyID    int64   `json:"currencyId"`
	QFormID       string  `json:"qFormId"`
	IsUat         bool    `json:"isUat"`
}

func checkEnum(param, value string, allowed []string) error {
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("%s %q is not supported: use one of %s", param, value, strings.Join(allowed, ", "))
	}
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// normalize fills defaults, checks every enum the web app reads back, and rewrites what the wizard
// would rewrite for the chart type; each rewrite is reported as a warning.
func (s *ChartSettings) normalize() ([]string, error) {
	s.ChartType, s.CounterType = orDefault(s.ChartType, "line"), orDefault(s.CounterType, "amount")
	s.Range, s.OrderValue = orDefault(s.Range, "lastHour"), orDefault(s.OrderValue, "default")
	for _, c := range []struct {
		param, value string
		allowed      []string
	}{{"chartType", s.ChartType, chartTypes}, {"counterType", s.CounterType, chartCounterTypes}, {"range", s.Range, chartRanges}, {"orderValue", s.OrderValue, chartOrderValues}} {
		if err := checkEnum(c.param, c.value, c.allowed); err != nil {
			return nil, err
		}
	}
	var warnings []string
	switch {
	case slices.Contains(intervalChartTypes, s.ChartType) && s.Range == "allTime":
		return nil, fmt.Errorf("range allTime is not supported for chartType %s: pick a bounded range such as lastMonth", s.ChartType)
	case slices.Contains(intervalChartTypes, s.ChartType) && s.Range == "realTime":
		s.Range = "lineRealTime"
		warnings = append(warnings, "range realTime is stored as lineRealTime for chartType "+s.ChartType)
	case !slices.Contains(intervalChartTypes, s.ChartType) && s.Range == "lineRealTime":
		s.Range = "realTime"
		warnings = append(warnings, "range lineRealTime is stored as realTime for chartType "+s.ChartType)
	}
	if slices.Contains(intervalChartTypes, s.ChartType) && s.OrderValue != "default" {
		warnings = append(warnings, fmt.Sprintf("orderValue %s ignored: chartType %s is not sortable", s.OrderValue, s.ChartType))
		s.OrderValue = "default"
	}
	return warnings, nil
}

// seriesSeen refuses a series the web app would show once: it keys series by actor, account name,
// currency and incomeType.
type seriesSeen map[string]int

func (seen seriesSeen) add(i int, actorID, nameID string, currencyID int64, incomeType string) error {
	key := fmt.Sprintf("%s_%s_%d_%s", actorID, nameID, currencyID, incomeType)
	if j, dup := seen[key]; dup {
		return fmt.Errorf("accounts[%d] repeats accounts[%d]: the chart would show it once", i, j)
	}
	seen[key] = i
	return nil
}

// normalizeAccounts defaults incomeType and colour, and refuses a series without its ids or a
// repeated one.
func normalizeAccounts(in []ChartAccount) ([]ChartAccount, error) {
	out := slices.Clone(in)
	seen := seriesSeen{}
	for i := range out {
		a := &out[i]
		a.IncomeType = orDefault(a.IncomeType, "total")
		a.Color = orDefault(a.Color, chartSeriesPalette[i%len(chartSeriesPalette)])
		if err := checkEnum(fmt.Sprintf("accounts[%d].incomeType", i), a.IncomeType, chartIncomeTypes); err != nil {
			return nil, err
		}
		if !uuidPattern.MatchString(a.ActorID) || !uuidPattern.MatchString(a.NameID) || a.CurrencyID < 1 {
			return nil, fmt.Errorf("accounts[%d] needs actorId (actor id), nameId (account name id) and a positive currencyId", i)
		}
		if err := seen.add(i, a.ActorID, a.NameID, a.CurrencyID, a.IncomeType); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func dashboardAccounts(accounts []ChartAccount) ([]dashboardAccount, bool) {
	out := make([]dashboardAccount, len(accounts))
	mixed := false
	for i, a := range accounts {
		out[i] = dashboardAccount{ActorID: a.ActorID, Color: a.Color, IncomeType: a.IncomeType, CurrencyID: a.CurrencyID, NameID: a.NameID}
		mixed = mixed || a.IncomeType != accounts[0].IncomeType
	}
	return out, mixed
}

func newDynamicSource(filterID, title string, top int, f ChartFilter) *dashboardDynamicSource {
	ds := &dashboardDynamicSource{ID: filterID, Title: title, Top: strconv.Itoa(top), GroupByFields: []string{}}
	if f.FormID > 0 {
		ds.Filter = &dashboardFilter{FormID: f.FormID, SelectedForms: []int64{f.FormID}, AccountNameID: f.AccountNameID, CurrencyID: f.CurrencyID, QFormID: strconv.FormatInt(f.FormID, 10)}
	}
	return ds
}

// newSource is a new chart's source: manual with accounts, else actorFilter on filterID.
func newSource(s ChartSettings, accounts []ChartAccount, dynamic *dashboardDynamicSource) dashboardSource {
	src := dashboardSource{Accounts: []dashboardAccount{}, CounterType: s.CounterType, ChartType: s.ChartType, SourceType: "actorFilter", Range: s.Range,
		ShowTotal: true, OrderValue: s.OrderValue, Legend: dashboardLegend{ActorTitle: true, AccountName: true, CurrencyName: true},
		DisplayChartDataLabels: true, DynamicSource: dynamic, ChartViewMode: "default"}
	if len(accounts) > 0 {
		src.SourceType, src.DynamicSource = "manual", nil
		src.Accounts, src.Legend.IncomeType = dashboardAccounts(accounts)
	}
	return src
}

// filterData is an ActorFilters actor's data as the web app writes it: filter as a JSON string.
func filterData(f ChartFilter) json.RawMessage {
	filter, _ := json.Marshal(map[string]any{"formId": f.FormID, "selectedForms": []int64{f.FormID}, "accountNameId": f.AccountNameID, "currencyId": f.CurrencyID})
	data, _ := json.Marshal(map[string]any{"filter": string(filter), "fields": "[]", "defaultFields": `["title","id","ref","owner","createdAt","updatedAt","balance"]`,
		"formFieldExportFormat": "idAndTitle", "cacheFilterResult": false})
	return data
}

// flexInt reads a number some writers store as a numeric string.
type flexInt int64

func (v *flexInt) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		b = []byte(strings.TrimSpace(s))
		if len(b) == 0 {
			*v = 0
			return nil
		}
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("not an integer: %s", b)
	}
	*v = flexInt(n)
	return nil
}

// unwrapString returns the JSON a value holds as a string, or the value itself when it is not one.
func unwrapString(raw json.RawMessage) json.RawMessage {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return json.RawMessage(s)
	}
	return raw
}

// readFilter reads an ActorFilters actor's data.filter, a JSON string or an object.
func readFilter(raw json.RawMessage) ChartFilter {
	var f struct {
		FormID        flexInt `json:"formId"`
		AccountNameID string  `json:"accountNameId"`
		CurrencyID    flexInt `json:"currencyId"`
	}
	if len(raw) == 0 || json.Unmarshal(unwrapString(raw), &f) != nil {
		return ChartFilter{}
	}
	return ChartFilter{FormID: int64(f.FormID), AccountNameID: f.AccountNameID, CurrencyID: int64(f.CurrencyID)}
}
