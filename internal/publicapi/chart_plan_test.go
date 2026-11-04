package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const chartID = "0751e107-d31a-4abe-be6a-1aac8f9a450c"

// storedChart is a chart actor whose data.source is the given object, stored as a JSON string.
func storedChart(source string) chartActor {
	a := chartActor{ID: chartID, Title: "Revenue", FormID: 28}
	a.Data.Source, _ = json.Marshal(source)
	return a
}

func noFilter(context.Context, string) (string, ChartFilter, error) {
	return "", ChartFilter{}, errors.New("unexpected filter read")
}

func plannedSource(t *testing.T, p sourcePlan) string {
	t.Helper()
	b, _ := json.Marshal(p.keys)
	return string(b)
}

const (
	manualSource = `{"chartType":"bar","counterType":"amount","range":"lastMonth","orderValue":"desc","sourceType":"manual","yAxisLabel":"UAH",` +
		`"legend":{"actorTitle":true,"accountName":false,"currencyName":true,"incomeType":true},` +
		`"accounts":[{"actorId":"` + actorA + `","nameId":"` + nameN + `","currencyId":"1","incomeType":"credit","accountType":"plan","customName":"Plan"},{"actorId":"` + actorB + `","nameId":"` + nameN + `","currencyId":1,"color":"#111111"}]}`
	filterSource = `{"chartType":"pie","counterType":"amount","range":"lastWeek","orderValue":"asc","sourceType":"actorFilter","accounts":[],` +
		`"dynamicSource":{"id":"f0","title":"Top","top":"5","groupByFields":[],"filter":{"formId":19,"accountNameId":"` + nameN + `","currencyId":2}}}`
	groupSource = `{"chartType":"bar","sourceType":"actorFilter","accounts":[],` +
		`"dynamicSource":{"id":"f0","title":"Top","displayMode":"groupBy","groupByFields":["19:city"],"filter":{"formId":19,"accountNameId":"` + nameN + `","currencyId":2}}}`
)

func TestPlanManualKeepsStoredKeys(t *testing.T) {
	p, err := planSource(context.Background(), ChartUpdateInput{ChartID: chartID, ChartSettings: ChartSettings{ChartType: "line"}, IncomeType: "debit"}, storedChart(manualSource), noFilter)
	if err != nil {
		t.Fatal(err)
	}
	got := plannedSource(t, p)
	for _, want := range []string{`"accountType":"plan"`, `"customName":"Plan"`, `"yAxisLabel":"UAH"`, `"color":"#499894"`, `"color":"#111111"`, `"accountName":false`,
		`"chartType":"line"`, `"orderValue":"default"`, `"range":"lastMonth"`} {
		if !strings.Contains(got, want) {
			t.Errorf("source lacks %s: %s", want, got)
		}
	}
	if strings.Contains(got, `"credit"`) || strings.Contains(got, `"incomeType":true`) || len(p.warnings) != 1 || p.sourceType != "manual" {
		t.Errorf("incomeType, legend or warnings not applied: %s %v", got, p.warnings)
	}
}

// One incomeType for every series must not turn two series into one the web app shows once.
func TestPlanManualRefusesRepeatedSeries(t *testing.T) {
	both := `{"chartType":"bar","sourceType":"manual","accounts":[{"actorId":"` + actorA + `","nameId":"` + nameN + `","currencyId":"1"},` +
		`{"actorId":"` + actorA + `","nameId":"` + nameN + `","currencyId":1,"incomeType":"credit"}]}`
	_, err := planSource(context.Background(), ChartUpdateInput{ChartID: chartID, IncomeType: "credit"}, storedChart(both), noFilter)
	if err == nil || !strings.Contains(err.Error(), "accounts[1] repeats accounts[0]") {
		t.Fatal("two series made one:", err)
	}
	if _, err = planSource(context.Background(), ChartUpdateInput{ChartID: chartID, IncomeType: "debit"}, storedChart(manualSource), noFilter); err != nil {
		t.Fatal("distinct series refused:", err)
	}
}

func TestPlanManualToFilter(t *testing.T) {
	ctx := context.Background()
	if _, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{SourceFormID: 19}}, storedChart(manualSource), noFilter); err == nil || !strings.Contains(err.Error(), "a new filter needs") {
		t.Fatal("an incomplete filter was accepted:", err)
	}
	in := ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{SourceFormID: 19, AccountNameID: nameN, CurrencyID: 2}}
	p, err := planSource(ctx, in, storedChart(manualSource), noFilter)
	if err != nil || p.newFilter == nil || *p.newFilter != (ChartFilter{19, nameN, 2}) || p.filterTitle != "Revenue" || p.top != 20 || p.prevFilterID != "" {
		t.Fatalf("%+v %v", p, err)
	}
	if got := plannedSource(t, p); !strings.Contains(got, `"sourceType":"actorFilter"`) || !strings.Contains(got, `"accounts":[]`) {
		t.Error(got)
	}
	if _, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{Top: 3}}, storedChart(manualSource), noFilter); err == nil {
		t.Error("top accepted for a manual chart")
	}
}

func TestPlanFilterChart(t *testing.T) {
	ctx := context.Background()
	p, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{Top: 7, FilterTitle: "Best"}}, storedChart(filterSource), noFilter)
	if got := plannedSource(t, p); err != nil || p.filterID != "f0" || !strings.Contains(got, `"top":"7"`) || !strings.Contains(got, `"title":"Best"`) || !strings.Contains(got, `"filter":{"formId":19`) {
		t.Fatal(got, err)
	}
	// Only the currency changes: the template and the account name come from the current filter.
	p, err = planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{CurrencyID: 3}}, storedChart(filterSource), noFilter)
	if err != nil || p.newFilter == nil || *p.newFilter != (ChartFilter{19, nameN, 3}) || p.filterTitle != "Top" || p.top != 5 || p.prevFilterID != "f0" {
		t.Fatalf("%+v %v", p, err)
	}
	reader := func(_ context.Context, id string) (string, ChartFilter, error) {
		return "Stored", ChartFilter{21, nameN, 2}, nil
	}
	p, err = planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{FilterActorID: actorB}}, storedChart(filterSource), reader)
	if got := plannedSource(t, p); err != nil || p.filterID != actorB || !strings.Contains(got, `"title":"Stored"`) || !strings.Contains(got, `"formId":21`) {
		t.Fatal(got, err)
	}
	if _, err = planSource(ctx, ChartUpdateInput{ChartID: chartID, IncomeType: "credit"}, storedChart(filterSource), noFilter); err == nil {
		t.Error("incomeType accepted for an actorFilter chart")
	}
	p, err = planSource(ctx, ChartUpdateInput{ChartID: chartID, Accounts: []ChartAccount{{ActorID: actorA, NameID: nameN, CurrencyID: 1}}}, storedChart(filterSource), noFilter)
	if got := plannedSource(t, p); err != nil || p.sourceType != "manual" || strings.Contains(got, "dynamicSource") || p.prevFilterID != "f0" {
		t.Fatal(got, err)
	}
}

func TestPlanGroupBy(t *testing.T) {
	ctx := context.Background()
	if _, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{Top: 3}}, storedChart(groupSource), noFilter); err == nil || !strings.Contains(err.Error(), "groupBy") {
		t.Fatal("top accepted for a group-by chart:", err)
	}
	p, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{CurrencyID: 3}}, storedChart(groupSource), noFilter)
	ds, _ := json.Marshal(p.dynamicSource("f1", p.filterTitle, *p.newFilter))
	if err != nil || !strings.Contains(string(ds), `"displayMode":"groupBy","filter"`) || !strings.Contains(string(ds), `"groupByFields":["19:city"]`) || strings.Contains(string(ds), `"top"`) {
		t.Fatal(string(ds), err)
	}
	p, err = planSource(ctx, ChartUpdateInput{ChartID: chartID, ChartFilterInput: ChartFilterInput{SourceFormID: 20}}, storedChart(groupSource), noFilter)
	if err != nil || p.grouping != nil || len(p.warnings) != 1 || !strings.Contains(p.warnings[0], "template 20 has other fields") {
		t.Fatal(p.warnings, err)
	}
}

func TestPlanRefusals(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ source, want string }{
		{`{"chartType":"kpiCard","accounts":[]}`, "kpiCard"},
		{`{"chartType":"bar","range":"custom","accounts":[]}`, "custom date range"},
		{`{}`, "no chart source"},
	} {
		if _, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, CounterType: "count"}, storedChart(tc.source), noFilter); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.source, err)
		}
	}
	empty := chartActor{ID: chartID}
	if _, err := planSource(ctx, ChartUpdateInput{ChartID: chartID, CounterType: "count"}, empty, noFilter); err == nil {
		t.Error("a chart without data.source was accepted")
	}
}
