package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func layerSettingsBackend(t *testing.T) (*Client, *[]rpcCall) {
	return rpcBackend(t, func(method string) string {
		switch method {
		case "form/get":
			return `{"id":12,"title":"Clients"}`
		case "account_name/list":
			return `{"list":[{"id":"n-rev","name":"Revenue"}]}`
		case "account_currency/list":
			return `{"list":[{"id":2,"name":"USD"}]}`
		}
		return `{"id":"L"}`
	})
}

// Every setting is written where the Graph Editor and legacy keep it, in one actor/update of the
// layer's data; accounts are resolved by name and the old pair of lists is cleared.
func TestUpdateLayerSettings(t *testing.T) {
	c, calls := layerSettingsBackend(t)
	form, on, off := int64(12), true, false
	start := int64(1730419200)
	accounts := []LayerAccount{{AccountName: "Revenue", Currency: "USD"}, {AccountName: "Revenue", Currency: "USD"}}
	out, err := c.UpdateLayerSettings(context.Background(), LayerSettingsInput{LayerID: "L", DefaultFormID: &form, EdgeCurveStyle: "straight",
		Anisotropy: &off, OuterEdges: &on, InitialCell: "B, 2", Accounts: &accounts, AccountsPeriod: &LayerPeriod{StartDate: &start, TimeZoneOffset: -120}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.Keys, ",") != "accountName,accounts,accountsPeriod,anisotropy,currency,defForm,edgeCurveStyle,initX,initY,outerEdges" {
		t.Fatal(out.Keys)
	}
	last := (*calls)[len(*calls)-1]
	if last.Method != "actor/update" || last.Params["actorId"] != "L" {
		t.Fatal(last)
	}
	data, _ := json.Marshal(last.Params["data"])
	want := `{"accountName":null,"accounts":[{"accountName":"Revenue","currencyId":2,"currencyName":"USD","nameId":"n-rev","title":"Revenue, USD","value":"n-rev_2"}],` +
		`"accountsPeriod":{"startDate":1730419200,"timeZoneOffset":-120},"anisotropy":false,"currency":null,` +
		`"defForm":[{"title":"Clients","type":"form","value":12}],"edgeCurveStyle":[{"value":"straight"}],"initX":50,"initY":50,"outerEdges":true}`
	if string(data) != want {
		t.Fatalf("data %s\nwant %s", data, want)
	}
}

func TestUpdateLayerSettingsClears(t *testing.T) {
	c, calls := layerSettingsBackend(t)
	zero := int64(0)
	none := []LayerAccount{}
	if _, err := c.UpdateLayerSettings(context.Background(), LayerSettingsInput{LayerID: "L", DefaultFormID: &zero, Accounts: &none, AccountsPeriod: &LayerPeriod{}}); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal((*calls)[0].Params["data"])
	// A cleared period is never {}: the offset alone replaces a stored window.
	if len(*calls) != 1 || string(data) != `{"accountName":null,"accounts":[],"accountsPeriod":{"timeZoneOffset":0},"currency":null,"defForm":[]}` {
		t.Fatal(len(*calls), string(data))
	}
}

func TestUpdateLayerSettingsRefusals(t *testing.T) {
	c, calls := layerSettingsBackend(t)
	neg := int64(-1)
	start, end := int64(10), int64(5)
	for _, bad := range []LayerSettingsInput{
		{},
		{LayerID: "L"},
		{LayerID: "L", EdgeCurveStyle: "wavy"},
		{LayerID: "L", DefaultFormID: &neg},
		{LayerID: "L", InitialCell: "C7"},
		{LayerID: "L", AccountsPeriod: &LayerPeriod{StartDate: &start, EndDate: &end}},
		{LayerID: "L", Accounts: &[]LayerAccount{{AccountName: "Revenue"}}},
	} {
		if _, err := c.UpdateLayerSettings(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if len(*calls) != 0 {
		t.Error("a refused change reached the API:", called(*calls))
	}
	// An unknown name is refused before anything is written.
	if _, err := c.UpdateLayerSettings(context.Background(), LayerSettingsInput{LayerID: "L", Accounts: &[]LayerAccount{{AccountName: "Revenue", Currency: "EUR"}}}); err == nil || !strings.Contains(err.Error(), "no account name") || strings.Contains(called(*calls), "actor/update") {
		t.Fatal(err, called(*calls))
	}
}
