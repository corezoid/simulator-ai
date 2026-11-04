package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// storedSource is what chart_update reads of a stored data.source; every other key is kept as the
// web app (or sim-api) wrote it.
type storedSource struct {
	ChartSettings
	SourceType    string          `json:"sourceType"`
	Accounts      []storedAccount `json:"accounts"`
	DynamicSource *storedDynamic  `json:"dynamicSource"`
}
type storedAccount struct {
	ActorID    string  `json:"actorId"`
	NameID     string  `json:"nameId"`
	CurrencyID flexInt `json:"currencyId"`
	Color      string  `json:"color"`
	IncomeType string  `json:"incomeType"`
}
type storedDynamic struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Top         flexInt `json:"top"`
	DisplayMode string  `json:"displayMode"`
	Filter      *struct {
		FormID        flexInt `json:"formId"`
		AccountNameID string  `json:"accountNameId"`
		CurrencyID    flexInt `json:"currencyId"`
	} `json:"filter"`
}

// filterMode: actorFilter, or (as the web app reads an unknown type) a dynamic source without manual.
func (s storedSource) filterMode() bool {
	return s.SourceType == "actorFilter" || (s.SourceType != "manual" && s.SourceType != "accounts" && s.DynamicSource != nil)
}

func (s storedSource) filter() ChartFilter {
	if s.DynamicSource == nil || s.DynamicSource.Filter == nil {
		return ChartFilter{}
	}
	f := s.DynamicSource.Filter
	return ChartFilter{FormID: int64(f.FormID), AccountNameID: f.AccountNameID, CurrencyID: int64(f.CurrencyID)}
}

// parseStoredSource reads data.source, a JSON string or an object, with its raw keys.
func parseStoredSource(raw json.RawMessage) (storedSource, map[string]json.RawMessage, error) {
	var s storedSource
	raw = unwrapString(raw)
	keys := rawObject(raw)
	if len(keys) == 0 {
		return s, nil, errors.New("the actor has no chart source (data.source): create the chart with chart_create")
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, nil, fmt.Errorf("the chart source cannot be read: %w", err)
	}
	return s, keys, nil
}

// sourcePlan is the rewritten source; newFilter asks for an ActorFilters actor before it is saved.
type sourcePlan struct {
	keys         map[string]json.RawMessage
	settings     ChartSettings
	sourceType   string
	top          int
	grouping     map[string]json.RawMessage // a group-by display kept from the stored source
	newFilter    *ChartFilter
	filterTitle  string
	filterID     string
	prevFilterID string
	warnings     []string
}

func (p sourcePlan) dynamicSource(id, title string, f ChartFilter) map[string]json.RawMessage {
	b, _ := json.Marshal(newDynamicSource(id, title, p.top, f))
	ds := rawObject(b)
	for k, v := range p.grouping {
		ds[k] = v
	}
	if p.grouping != nil {
		delete(ds, "top") // a group-by chart has no top
	}
	return ds
}

func override(f ChartFilter, in ChartFilterInput) ChartFilter {
	if in.SourceFormID != 0 {
		f.FormID = in.SourceFormID
	}
	if in.AccountNameID != "" {
		f.AccountNameID = in.AccountNameID
	}
	if in.CurrencyID != 0 {
		f.CurrencyID = in.CurrencyID
	}
	return f
}

type filterReader func(ctx context.Context, id string) (string, ChartFilter, error)

// planSource merges the update into the stored source the way legacy updateChart did: settings
// are replaced, a manual chart keeps or replaces its series, an actorFilter chart keeps its filter,
// takes filterActorId, or gets a new filter from changed ids (the others taken from the current).
func planSource(ctx context.Context, in ChartUpdateInput, chart chartActor, readFilter filterReader) (sourcePlan, error) {
	cur, keys, err := parseStoredSource(chart.Data.Source)
	switch {
	case err != nil:
		return sourcePlan{}, err
	case cur.ChartType == "kpiCard" && in.ChartType == "":
		return sourcePlan{}, errors.New("this is a kpiCard chart, which chart_update does not build: pass another chartType or edit it in the web app")
	case cur.Range == "custom" && in.Range == "":
		return sourcePlan{}, errors.New("this chart has a custom date range, which chart_update does not build: pass another range or edit it in the web app")
	}
	p := sourcePlan{keys: keys, settings: ChartSettings{ChartType: orDefault(in.ChartType, cur.ChartType), CounterType: orDefault(in.CounterType, cur.CounterType),
		Range: orDefault(in.Range, cur.Range), OrderValue: orDefault(in.OrderValue, cur.OrderValue)}}
	filterMode := cur.filterMode()
	switch {
	case len(in.Accounts) > 0:
		filterMode = false
	case in.FilterActorID != "" || in.changesFilter():
		filterMode = true
	case !filterMode && (in.Top != 0 || in.FilterTitle != ""):
		return p, errors.New("top and filterTitle belong to an actorFilter chart: this one lists its accounts")
	}
	if filterMode && in.IncomeType != "" {
		return p, errors.New("incomeType belongs to a manual chart: an actorFilter chart has no series of its own")
	}
	if p.warnings, err = p.settings.normalize(); err != nil {
		return p, err
	}
	setJSON(keys, "chartType", p.settings.ChartType)
	setJSON(keys, "counterType", p.settings.CounterType)
	setJSON(keys, "range", p.settings.Range)
	setJSON(keys, "orderValue", p.settings.OrderValue)
	if cur.Range == "custom" {
		setJSON(keys, "rangeDates", dashboardRangeDates{})
	}
	if cur.filterMode() {
		p.prevFilterID = cur.DynamicSource.ID
	}
	if !filterMode {
		return p, p.manual(in, cur)
	}
	return p, p.actorFilter(ctx, in, cur, chart.Title, readFilter)
}

func (p *sourcePlan) legend() map[string]json.RawMessage {
	legend := rawObject(p.keys["legend"])
	if len(legend) == 0 {
		setJSON(legend, "actorTitle", true)
		setJSON(legend, "accountName", true)
		setJSON(legend, "currencyName", true)
	}
	return legend
}

// manual sets new accounts, or keeps the stored series (each with its own keys) and applies
// incomeType to them.
func (p *sourcePlan) manual(in ChartUpdateInput, cur storedSource) error {
	p.sourceType = "manual"
	setJSON(p.keys, "sourceType", "manual")
	delete(p.keys, "dynamicSource")
	var incomes []string
	if len(in.Accounts) > 0 {
		accounts := make([]ChartAccount, len(in.Accounts))
		for i, a := range in.Accounts {
			a.IncomeType = orDefault(a.IncomeType, in.IncomeType)
			accounts[i] = a
		}
		accounts, err := normalizeAccounts(accounts)
		if err != nil {
			return err
		}
		series, _ := dashboardAccounts(accounts)
		setJSON(p.keys, "accounts", series)
		for _, a := range accounts {
			incomes = append(incomes, a.IncomeType)
		}
	} else {
		var series []map[string]json.RawMessage
		if len(cur.Accounts) == 0 || json.Unmarshal(p.keys["accounts"], &series) != nil || len(series) != len(cur.Accounts) {
			return errors.New("this chart has no series: pass accounts, or filterActorId or sourceFormId, accountNameId and currencyId for an actorFilter chart")
		}
		seen := seriesSeen{}
		for i := range series {
			if series[i] == nil {
				series[i] = map[string]json.RawMessage{}
			}
			a := cur.Accounts[i]
			income := orDefault(in.IncomeType, orDefault(a.IncomeType, "total"))
			if err := checkEnum("incomeType", income, chartIncomeTypes); err != nil {
				return err
			}
			// One incomeType for every series can make two of them the same series.
			if err := seen.add(i, a.ActorID, a.NameID, int64(a.CurrencyID), income); err != nil {
				return err
			}
			setJSON(series[i], "incomeType", income)
			if cur.Accounts[i].Color == "" {
				setJSON(series[i], "color", chartSeriesPalette[i%len(chartSeriesPalette)])
			}
			incomes = append(incomes, income)
		}
		setJSON(p.keys, "accounts", series)
	}
	legend := p.legend()
	delete(legend, "incomeType")
	for _, income := range incomes {
		if income != incomes[0] {
			setJSON(legend, "incomeType", true)
		}
	}
	setJSON(p.keys, "legend", legend)
	return nil
}

// actorFilter points the source at its filter: filterActorId, a new filter, or the stored one.
func (p *sourcePlan) actorFilter(ctx context.Context, in ChartUpdateInput, cur storedSource, chartTitle string, readFilter filterReader) error {
	p.sourceType = "actorFilter"
	setJSON(p.keys, "sourceType", "actorFilter")
	setJSON(p.keys, "accounts", []dashboardAccount{})
	setJSON(p.keys, "legend", p.legend())
	var curDS *storedDynamic
	if cur.filterMode() {
		curDS = cur.DynamicSource
	}
	p.top = in.Top
	if p.top == 0 && curDS != nil {
		p.top = int(curDS.Top)
	}
	if p.top == 0 {
		p.top = 20
	}
	groupBy := curDS != nil && curDS.DisplayMode == "groupBy"
	if groupBy && in.Top != 0 {
		return errors.New(`this chart groups its actors by fields (display mode "groupBy"), so it has no top: switch it to top-N in the web app first`)
	}
	// keepGrouping keeps a group-by display for a filter of the same template, whose fields it names.
	keepGrouping := func(formID int64) {
		if !groupBy {
			return
		}
		if formID != cur.filter().FormID {
			p.warnings = append(p.warnings, fmt.Sprintf("the chart grouped actors by fields of template %d; template %d has other fields, so it now shows the top %d actors", cur.filter().FormID, formID, p.top))
			return
		}
		ds := rawObject(p.keys["dynamicSource"])
		p.grouping = map[string]json.RawMessage{"displayMode": ds["displayMode"], "groupByFields": ds["groupByFields"]}
	}
	switch {
	case in.FilterActorID != "":
		title, f, err := readFilter(ctx, in.FilterActorID)
		if err != nil {
			return err
		}
		f = override(f, in.ChartFilterInput)
		keepGrouping(f.FormID)
		p.filterID = in.FilterActorID
		setJSON(p.keys, "dynamicSource", p.dynamicSource(in.FilterActorID, orDefault(in.FilterTitle, title), f))
	case in.changesFilter():
		f := override(cur.filter(), in.ChartFilterInput)
		if !f.complete() || !uuidPattern.MatchString(f.AccountNameID) {
			return errors.New("a new filter needs sourceFormId, accountNameId and currencyId (those left out are taken from the chart's current filter), or pass filterActorId")
		}
		keepGrouping(f.FormID)
		p.newFilter = &f
		p.filterTitle = in.FilterTitle
		if p.filterTitle == "" && curDS != nil {
			p.filterTitle = curDS.Title
		}
		p.filterTitle = orDefault(p.filterTitle, chartTitle)
	default:
		if curDS == nil {
			return errors.New("the chart has no filter to keep: pass filterActorId, or sourceFormId, accountNameId and currencyId")
		}
		ds := rawObject(p.keys["dynamicSource"])
		if !groupBy {
			setJSON(ds, "top", strconv.Itoa(p.top))
		}
		if in.FilterTitle != "" {
			setJSON(ds, "title", in.FilterTitle)
		}
		setJSON(p.keys, "dynamicSource", ds)
		p.filterID = curDS.ID
	}
	return nil
}
