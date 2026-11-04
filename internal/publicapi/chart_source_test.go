package publicapi

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	actorA = "a55cd656-e33e-425c-9971-b0a258bfd914"
	actorB = "6b1f0d2e-4c7a-4f5b-9e83-2d6a1c0b7e45"
	nameN  = "5e0b9c27-41d8-4a3f-8c6e-92f1a7d04b13"
)

func TestChartSettingsNormalize(t *testing.T) {
	for _, tc := range []struct {
		in       ChartSettings
		want     ChartSettings
		warnings int
		err      string
	}{
		{ChartSettings{}, ChartSettings{"line", "amount", "lastHour", "default"}, 0, ""},
		{ChartSettings{ChartType: "bar", Range: "lineRealTime", OrderValue: "desc"}, ChartSettings{"bar", "amount", "realTime", "desc"}, 1, ""},
		{ChartSettings{ChartType: "stackedBar", Range: "realTime", OrderValue: "asc"}, ChartSettings{"stackedBar", "amount", "lineRealTime", "default"}, 2, ""},
		{ChartSettings{ChartType: "line", Range: "allTime"}, ChartSettings{}, 0, "range allTime is not supported"},
		{ChartSettings{ChartType: "kpiCard"}, ChartSettings{}, 0, `chartType "kpiCard" is not supported`},
		{ChartSettings{CounterType: "turnover"}, ChartSettings{}, 0, `counterType "turnover"`},
		{ChartSettings{Range: "custom"}, ChartSettings{}, 0, `range "custom"`},
	} {
		s := tc.in
		w, err := s.normalize()
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("%+v: %v, want %s", tc.in, err, tc.err)
			}
			continue
		}
		if err != nil || s != tc.want || len(w) != tc.warnings {
			t.Errorf("%+v: %+v %v %v", tc.in, s, w, err)
		}
	}
}

func TestChartAccounts(t *testing.T) {
	out, err := normalizeAccounts([]ChartAccount{{ActorID: actorA, NameID: nameN, CurrencyID: 1}, {ActorID: actorA, NameID: nameN, CurrencyID: 1, IncomeType: "credit", Color: "#000000"}})
	if err != nil || out[0].IncomeType != "total" || out[0].Color != chartSeriesPalette[0] || out[1].Color != "#000000" {
		t.Fatal(out, err)
	}
	for _, bad := range [][]ChartAccount{
		{{ActorID: actorA, NameID: nameN, CurrencyID: 1}, {ActorID: actorA, NameID: nameN, CurrencyID: 1, IncomeType: "total"}},
		{{ActorID: "Order 1", NameID: nameN, CurrencyID: 1}},
		{{ActorID: actorA, NameID: nameN}},
		{{ActorID: actorA, NameID: nameN, CurrencyID: 1, IncomeType: "net"}},
	} {
		if _, err := normalizeAccounts(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

// A new chart's data holds its source as a JSON string, the wizard's shape for either kind.
func TestNewChartSource(t *testing.T) {
	settings := ChartSettings{"bar", "amount", "lastMonth", "desc"}
	accounts, _ := normalizeAccounts([]ChartAccount{{ActorID: actorA, NameID: nameN, CurrencyID: 1}, {ActorID: actorB, NameID: nameN, CurrencyID: 1, IncomeType: "debit"}})
	var data struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(sourceData(newSource(settings, accounts, nil)), &data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"sourceType":"manual"`, `"legend":{"actorTitle":true,"accountName":true,"currencyName":true,"incomeType":true}`, `"defaultAccount":null`,
		`"rangeDates":{"from":null,"to":null}`, `"accounts":[{"actorId":"` + actorA + `","color":"#499894","incomeType":"total","currencyId":1,"nameId":"` + nameN + `"}`} {
		if !strings.Contains(data.Source, want) {
			t.Errorf("manual source lacks %s: %s", want, data.Source)
		}
	}
	src, _ := json.Marshal(newSource(settings, nil, newDynamicSource("f1", "Top orders", 5, ChartFilter{FormID: 19, AccountNameID: nameN, CurrencyID: 2})))
	for _, want := range []string{`"sourceType":"actorFilter"`, `"accounts":[]`, `"dynamicSource":{"id":"f1","title":"Top orders","top":"5","groupByFields":[],"filter":{"formId":19,"selectedForms":[19],"accountNameId":"` + nameN + `","currencyId":2,"qFormId":"19","isUat":false}}`} {
		if !strings.Contains(string(src), want) {
			t.Errorf("actorFilter source lacks %s: %s", want, src)
		}
	}
}

func TestReadFilter(t *testing.T) {
	for _, raw := range []string{`"{\"formId\":19,\"accountNameId\":\"n\",\"currencyId\":\"2\"}"`, `{"formId":"19","accountNameId":"n","currencyId":2}`} {
		if f := readFilter(json.RawMessage(raw)); f != (ChartFilter{19, "n", 2}) {
			t.Errorf("%s read as %+v", raw, f)
		}
	}
	if f := readFilter(json.RawMessage(`"not json"`)); f != (ChartFilter{}) {
		t.Errorf("garbage read as %+v", f)
	}
	var data struct{ Filter string }
	_ = json.Unmarshal(filterData(ChartFilter{19, nameN, 2}), &data)
	if readFilter(json.RawMessage(`"`+strings.ReplaceAll(data.Filter, `"`, `\"`)+`"`)) != (ChartFilter{19, nameN, 2}) {
		t.Errorf("filterData round trip: %s", data.Filter)
	}
}
