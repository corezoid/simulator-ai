package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The edge tools refuse a money edge (eval write-10): drawing one by name before any write, and
// re-pointing or relinking an existing one, found by its type id through ontology/get.
func TestMoneyEdgesRefused(t *testing.T) {
	ontology := `{"edgeTypes":[{"id":1,"name":"hierarchy","role":"hierarchy"},{"id":9,"name":"transfersAmount_n1_5","role":"transactions"}]}`
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch method {
		case "ontology/get":
			return ontology
		case "edge/get":
			if string(p["edgeId"]) == `"money"` {
				return `{"id":"money","edgeTypeId":9}`
			}
			return `{"id":"plain","edgeTypeId":1}`
		}
		return `{"id":"e1"}`
	})
	for tool, args := range map[string]string{
		"edge_create":  `{"source":"a","target":"b","edgeType":"transfersAmount_n1_5"}`,
		"edges_create": `{"edges":[{"source":"a","target":"b"},{"source":"a","target":"b","edgeType":"transfersAmount"}]}`,
	} {
		*calls = nil
		if r := send(tool, args); !strings.Contains(r, "is drawn by the platform from a transfer") || len(*calls) != 0 {
			t.Fatalf("%s: %d calls: %s", tool, len(*calls), r)
		}
	}
	for _, tc := range []struct{ tool, args, method string }{
		{"edge_endpoints_update", `{"edgeId":"money","target":"c"}`, ""},
		{"edge_update", `{"edgeId":"money","linkedActorId":"x"}`, ""},
		{"edge_update", `{"edgeId":"money","name":"paid"}`, "edge/update"},
		{"edge_endpoints_update", `{"edgeId":"plain","target":"c"}`, "edge/endpoints/update"},
	} {
		*calls = nil
		r := send(tc.tool, tc.args)
		wrote := (*calls)[len(*calls)-1].Method
		if tc.method == "" && (!strings.Contains(r, "transfersAmount_n1_5 edge") || strings.HasPrefix(wrote, "edge/") && wrote != "edge/get") ||
			tc.method != "" && wrote != tc.method {
			t.Fatalf("%s %s: last %s: %s", tc.tool, tc.args, wrote, r)
		}
	}
}

// layer_digest answers a layer in one call: forms, hierarchy levels, each actor's fields, each
// actor's balance (credit − debit: the company funded with 2 000 000 holds 500 000, not 2 000 000),
// their sums per template, and every payer → receiver pair on any money account with the number of
// transfers, from one batch of cards read over every edge type (eval r11251105 layer-01, layer-07).
func TestLayerDigest(t *testing.T) {
	money := func(nameID, cur, debit, credit string) string {
		return `{"nameId":"` + nameID + `","name":"Money` + cur + `","currencyId":` + cur + `,"currency":"C` + cur + `","type":"fact","debit":` + debit + `,"credit":` + credit + `}`
	}
	out := func(edgeType string, to ...string) string {
		var ns []string
		for i := 0; i < len(to); i += 2 {
			ns = append(ns, `{"actorId":"`+to[i]+`","weight":"`+to[i+1]+`"}`)
		}
		return `{"edgeTypeId":9,"edgeType":"` + edgeType + `","role":"transactions","direction":"out","neighbors":[` + strings.Join(ns, ",") + `]}`
	}
	parentOf := func(id string) string {
		return `{"edgeTypeId":1,"edgeType":"hierarchy","role":"hierarchy","direction":"in","neighbors":[{"actorId":"` + id + `"}]}`
	}
	cards := map[string]string{
		"co": `{"actor":{"title":"Company","formId":1,"formTitle":"Company"},"data":[{"title":"Staff","value":240}],"accounts":[` + money("n1", "5", "1500000", "2000000") + `,{"nameId":"n2","name":"Rating","currencyId":6,"currency":"count","type":"fact","debit":0,"credit":3}],` +
			`"neighbors":[` + out("transfersAmount_n1_5", "dev", "900000", "sales", "400000", "outside", "5") + `,` + out("transfersAmount_n1_5", "sales", "200000") + `,` +
			// The card's 200-neighbor budget cut this money type to nothing: it is read again alone.
			`{"edgeTypeId":12,"edgeType":"transfersAmount_n9_5","role":"transactions","direction":"out","hasMore":true,"neighbors":[]}]}`,
		"dev": `{"actor":{"title":"Development","formId":2,"formTitle":"Department"},"data":[{"title":"Head","value":"` + strings.Repeat("x", 120) + `"}],"accounts":[` + money("n1", "5", "300000", "900000") + `],` +
			`"neighbors":[` + parentOf("co") + `,` + out("transfersAmount_n1_5", "acme", "100000", "acme", "150000", "nl", "50000") + `,` + out("transfersAmount_n7_8", "nl", "3") + `]}`,
		"sales": `{"actor":{"title":"Sales","formId":2,"formTitle":"Department"},"data":[],"accounts":[` + money("n1", "5", "280000", "600000") + `],"neighbors":[` + parentOf("co") + `]}`,
		"acme":  `{"actor":{"title":"Acme","formId":3,"formTitle":"Counterparty"},"data":[],"accounts":[` + money("n1", "5", "0", "250000") + `],"neighbors":[` + parentOf("co") + `]}`,
		"nl":    `{"actor":{"title":"Nordline","formId":3,"formTitle":"Counterparty"},"data":[],"accounts":[` + money("n1", "5", "0", "50000") + `,` + money("n7", "8", "0", "3") + `],"neighbors":[` + parentOf("co") + `]}`,
	}
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch method {
		case "actor/get":
			return `{"id":"L","title":"Company map","data":{"accounts":[{"value":"n1_5","title":"Money5, C5"}]}}`
		case "graph/layer/stats/get":
			return `{"nodes":5,"edges":4}`
		case "graph/layer/actors/list":
			if string(p["offset"]) != "0" && p["offset"] != nil {
				return `{"list":[]}`
			}
			rows := []string{}
			for i, id := range []string{"co", "dev", "sales", "acme", "nl"} {
				rows = append(rows, fmt.Sprintf(`{"id":%q,"laId":%d,"actor":{"title":%q}}`, id, i+1, id))
			}
			return `{"list":[` + strings.Join(rows, ",") + `]}`
		case "actor/context/get":
			var id string
			_ = json.Unmarshal(p["actorId"], &id)
			if id == "co" && string(p["edgeTypeIds"]) == "[12]" && string(p["fields"]) == `["neighbors"]` {
				return `{"neighbors":[{"edgeTypeId":12,"edgeType":"transfersAmount_n9_5","role":"transactions","direction":"out","neighbors":[{"actorId":"acme","weight":"7"}]}]}`
			}
			if p["edgeTypeIds"] != nil || string(p["perType"]) != "50" {
				t.Errorf("card of %s read along %s, perType %s", id, p["edgeTypeIds"], p["perType"])
			}
			return cards[id]
		}
		t.Errorf("unexpected %s", method)
		return `{}`
	})
	r := send("layer_digest", `{"layerId":"L"}`)
	for _, want := range []string{`"levels":[{"level":0,"count":1},{"level":1,"count":4}]`,
		`"balances":[{"actorId":"co","title":"Company","form":"Company","account":"Money5","currency":"C5","balance":"500000"},{"actorId":"dev","title":"Development","form":"Department","account":"Money5","currency":"C5","balance":"600000"},{"actorId":"sales","title":"Sales","form":"Department","account":"Money5","currency":"C5","balance":"320000"},{"actorId":"acme","title":"Acme","form":"Counterparty","account":"Money5","currency":"C5","balance":"250000"},{"actorId":"nl","title":"Nordline","form":"Counterparty","account":"Money5","currency":"C5","balance":"50000"}],"balanceSums"`,
		`{"form":"Department","account":"Money5","currency":"C5","sumOfBalances":"920000","actors":2}`,
		`{"from":"co","fromTitle":"Company","to":"dev","toTitle":"Development","amount":"900000","account":"Money5","currency":"C5","count":1}`,
		`{"from":"co","fromTitle":"Company","to":"sales","toTitle":"Sales","amount":"600000","account":"Money5","currency":"C5","count":2}`,
		`{"from":"dev","fromTitle":"Development","to":"acme","toTitle":"Acme","amount":"250000","account":"Money5","currency":"C5","count":2}`,
		`{"from":"dev","fromTitle":"Development","to":"nl","toTitle":"Nordline","amount":"3","account":"Money8","currency":"C8","count":1}`,
		`{"from":"co","fromTitle":"Company","to":"acme","toTitle":"Acme","amount":"7","account":"n9_5","count":1}`,
		"xxx…", `"stats":{"nodes":5,"edges":4}`} {
		if !strings.Contains(r, want) {
			t.Fatalf("missing %s in %s", want, r)
		}
	}
	for _, bad := range []string{"2000000", "Rating", `"outside"`, "totals", "edge type", "types"} {
		if strings.Contains(r, bad) {
			t.Fatalf("%s in %s", bad, r)
		}
	}
	if len(*calls) != 5+len(cards) { // layer, stats, two placement pages, one card each, the cut money type again
		t.Fatalf("%d calls", len(*calls))
	}
	if r = send("layer_digest", `{"layerId":"L","full":true}`); !strings.Contains(r, `"account":"Rating"`) || !strings.Contains(r, strings.Repeat("x", 120)) {
		t.Fatal(r)
	}
}
