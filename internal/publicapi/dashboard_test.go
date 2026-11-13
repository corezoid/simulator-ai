package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// dashboardExample is the "Daily series with a total" result of sim-api's public dashboard/get
// page, with a field this client does not know.
const dashboardExample = `{"data":[{"actorId":"acf617f6-1414-423b-ace6-a31341f41d8b","actorTitle":"Chaban Andrey","nameId":"b37efbf5-681b-4876-9021-47639da2e093",` +
	`"accountName":"MCP API Calls","currencyId":68933782,"currencyName":"Requests","precision":2,"incomeType":"total","accountType":"fact",` +
	`"showTrendLine":false,"errorFormula":false,"value":0,"internal":"dropped","data":[{"bucketStart":1788210000000,"value":1},{"bucketStart":1788296400000,"value":519.25}]}],` +
	`"total":{"value":0,"data":[{"bucketStart":1788210000000,"value":1},{"bucketStart":1788296400000,"value":519.25}]}}`

// dashboardBackend answers dashboard/get with member (a JSON-RPC result or error member) and
// records the params it was sent.
func dashboardBackend(t *testing.T, profile, actor, member string) (*Client, *[]map[string]json.RawMessage) {
	t.Helper()
	sent := &[]map[string]json.RawMessage{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string                     `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.Method != "dashboard/get" {
			t.Errorf("unexpected request %s %v", q.Method, err)
		}
		*sent = append(*sent, q.Params)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,%s}`, q.ID, member)
	}))
	t.Cleanup(api.Close)
	c, err := New(Config{Origin: api.URL, Workspace: "ws", APIKey: "fixture", Profile: profile, ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	return c, sent
}

func TestGetDashboardMapsRequestAndResult(t *testing.T) {
	c, sent := dashboardBackend(t, "graph-read", "", `"result":`+dashboardExample)
	from, to, offset := int64(1788210000000), int64(1788382799000), int64(-180)
	got, err := c.GetDashboard(context.Background(), DashboardInput{ChartID: "chart", DashboardQuery: DashboardQuery{
		From: &from, To: &to, Interval: "day", TimezoneOffset: &offset, With: []string{"total"}, ActorIDs: []string{"acf617f6-1414-423b-ace6-a31341f41d8b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	params := (*sent)[0]
	want := map[string]string{"accId": `"ws"`, "actorId": `"chart"`, "from": "1788210000000", "to": "1788382799000", "interval": `"day"`,
		"timezoneOffset": "-180", "with": `["total"]`, "actorIds": `["acf617f6-1414-423b-ace6-a31341f41d8b"]`}
	if len(params) != len(want) {
		t.Fatalf("sent %d params: %v", len(params), params)
	}
	for k, v := range want {
		if string(params[k]) != v {
			t.Errorf("%s sent as %s, want %s", k, params[k], v)
		}
	}
	points := []DashboardPoint{{1788210000000, 1}, {1788296400000, 519.25}}
	expected := DashboardData{Data: []DashboardSeries{{ActorID: "acf617f6-1414-423b-ace6-a31341f41d8b", ActorTitle: "Chaban Andrey", NameID: "b37efbf5-681b-4876-9021-47639da2e093",
		AccountName: "MCP API Calls", CurrencyID: 68933782, CurrencyName: "Requests", Precision: 2, IncomeType: "total", AccountType: "fact", Data: points}},
		Total: &DashboardTotal{Value: 0, Data: points}}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("mapped %+v", got)
	}
	out, _ := json.Marshal(got)
	if strings.Contains(string(out), "internal") || !strings.Contains(string(out), `"showTrendLine":false,"errorFormula":false,"value":0`) {
		t.Fatalf("result shape %s", out)
	}

	// Only the chart: no optional param is sent, and a dashboard without series is an empty list.
	c, sent = dashboardBackend(t, "console", "", `"result":{"data":[]}`)
	if got, err = c.GetDashboard(context.Background(), DashboardInput{ChartID: "chart"}); err != nil || got.Data == nil || len(got.Data) != 0 || got.Total != nil {
		t.Fatal(got, err)
	}
	if len((*sent)[0]) != 2 {
		t.Fatalf("sent %v", (*sent)[0])
	}
}

func TestGetDashboardErrors(t *testing.T) {
	for _, reason := range []string{"invalidParams", "accessDenied", "notFound"} {
		c, _ := dashboardBackend(t, "graph-read", "", fmt.Sprintf(`"error":{"code":-32602,"message":"refused","data":{"reason":%q}}`, reason))
		_, err := c.GetDashboard(context.Background(), DashboardInput{ChartID: "chart", DashboardQuery: DashboardQuery{With: []string{"series"}}})
		var api *APIError
		if !errors.As(err, &api) || api.Reason != reason {
			t.Fatalf("%s surfaced as %v", reason, err)
		}
	}
	for _, result := range []string{
		`{"data":null}`, `{}`, `{"data":[{"actorId":"","nameId":"n","currencyId":1}]}`, `{"data":[{"actorId":"a","nameId":"n","currencyId":0}]}`,
		`{"data":[{"actorId":"a","nameId":"n","currencyId":1,"value":"1"}]}`, `{"data":[]}`, // the last asked for total and got none
	} {
		c, _ := dashboardBackend(t, "console", "", `"result":`+result)
		if _, err := c.GetDashboard(context.Background(), DashboardInput{ChartID: "chart", DashboardQuery: DashboardQuery{With: []string{"total"}}}); err == nil {
			t.Fatalf("invalid result accepted: %s", result)
		}
	}
}

func TestGetDashboardProfiles(t *testing.T) {
	for _, tc := range []struct {
		profile, actor string
		enabled        bool
	}{{"console", "", true}, {"console-full", "", true}, {"graph-read", "", true}, {"read", "", false}, {"graph-create", "", false}, {"graph-read", "layer", false}, {"smart-forms", "", false}} {
		c, sent := dashboardBackend(t, tc.profile, tc.actor, `"result":{"data":[]}`)
		if c.DashboardReadsEnabled() != tc.enabled {
			t.Errorf("%s/%s enabled %t", tc.profile, tc.actor, !tc.enabled)
		}
		_, err := c.GetDashboard(context.Background(), DashboardInput{ChartID: "chart"})
		if (err == nil) != tc.enabled || (!tc.enabled && len(*sent) != 0) {
			t.Errorf("%s/%s: %v, %d calls", tc.profile, tc.actor, err, len(*sent))
		}
	}
	c, sent := dashboardBackend(t, "graph-read", "", `"result":{"data":[]}`)
	for _, id := range []string{"", "  ", strings.Repeat("a", 256)} {
		if _, err := c.GetDashboard(context.Background(), DashboardInput{ChartID: id}); err == nil {
			t.Fatalf("chartId %q accepted", id)
		}
	}
	if len(*sent) != 0 {
		t.Fatal("refused chartId reached the backend")
	}
}
