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

// layer_digest answers a layer in one call: forms, hierarchy levels, each actor's fields and the
// balances of the pairs the layer shows, their totals and the money between its actors, reading the
// cards in one batch.
func TestLayerDigest(t *testing.T) {
	cards := map[string]string{
		"co":    `{"actor":{"title":"Company","formId":1,"formTitle":"Company"},"data":[{"title":"Staff","value":240}],"accounts":[{"nameId":"n1","name":"Money","currencyId":5,"currency":"UAH","type":"fact","debit":1500000,"credit":2000000},{"nameId":"n2","name":"Rating","currencyId":6,"currency":"count","type":"fact","debit":0,"credit":3}],"neighbors":[{"edgeTypeId":9,"edgeType":"transfersAmount_n1_5","role":"transactions","direction":"out","neighbors":[{"actorId":"dev","weight":"900000"},{"actorId":"dev","weight":"100000"},{"actorId":"outside","weight":"5"}]}]}`,
		"dev":   `{"actor":{"title":"Development","formId":2,"formTitle":"Department"},"data":[{"title":"Head","value":"` + strings.Repeat("x", 120) + `"}],"accounts":[{"nameId":"n1","name":"Money","currencyId":5,"currency":"UAH","type":"fact","debit":300000,"credit":900000}],"neighbors":[{"edgeTypeId":1,"edgeType":"hierarchy","role":"hierarchy","direction":"in","neighbors":[{"actorId":"co"}]}]}`,
		"proj":  `{"actor":{"title":"Portal","formId":3,"formTitle":"Project"},"data":[],"accounts":[],"neighbors":[{"edgeTypeId":1,"edgeType":"hierarchy","role":"hierarchy","direction":"in","neighbors":[{"actorId":"dev"}]}]}`,
		"other": `{"actor":{"title":"Sales","formId":2,"formTitle":"Department"},"data":[],"accounts":[{"nameId":"n1","name":"Money","currencyId":5,"currency":"UAH","type":"fact","debit":0,"credit":0}],"neighbors":[{"edgeTypeId":1,"edgeType":"hierarchy","role":"hierarchy","direction":"in","neighbors":[{"actorId":"co"}]}]}`,
	}
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch method {
		case "actor/get":
			return `{"id":"L","title":"Company map","data":{"accounts":[{"value":"n1_5","title":"Money, UAH"}]}}`
		case "graph/layer/stats/get":
			return `{"nodes":4,"edges":3}`
		case "graph/layer/actors/list":
			if string(p["offset"]) != "0" && p["offset"] != nil {
				return `{"list":[]}`
			}
			rows := []string{}
			for i, id := range []string{"co", "dev", "proj", "other"} {
				rows = append(rows, fmt.Sprintf(`{"id":%q,"laId":%d,"actor":{"title":%q}}`, id, i+1, id))
			}
			return `{"list":[` + strings.Join(rows, ",") + `]}`
		case "ontology/get":
			return `{"edgeTypes":[{"id":1,"name":"hierarchy","role":"hierarchy"},{"id":9,"name":"transfersAmount_n1_5","role":"transactions"}]}`
		case "actor/context/get":
			var id string
			_ = json.Unmarshal(p["actorId"], &id)
			if string(p["edgeTypeIds"]) != "[1,9]" {
				t.Errorf("card of %s read along %s", id, p["edgeTypeIds"])
			}
			return cards[id]
		}
		return `{}`
	})
	r := send("layer_digest", `{"layerId":"L"}`)
	for _, want := range []string{`"levels":[{"level":0,"count":1},{"level":1,"count":2},{"level":2,"count":1}]`,
		`"balances":[{"account":"Money","currency":"UAH","balance":"500000"}]`, `"balance":"600000"`,
		`"totals":[{"account":"Money","currency":"UAH","balance":"1100000","actors":2}]`,
		`"transfers":[{"from":"co","fromTitle":"Company","to":"dev","toTitle":"Development","amount":"1000000","account":"Money","currency":"UAH","edges":2}]`,
		`"parentId":"dev"`, "xxx…", `"stats":{"nodes":4,"edges":3}`} {
		if !strings.Contains(r, want) {
			t.Fatalf("missing %s in %s", want, r)
		}
	}
	if strings.Contains(r, "Rating") || strings.Contains(r, `"outside"`) {
		t.Fatal(r)
	}
	batches := 0
	for _, c := range *calls {
		if c.Method == "actor/context/get" {
			batches++
		}
	}
	if batches != 4 {
		t.Fatalf("%d card reads", batches)
	}
	if r = send("layer_digest", `{"layerId":"L","full":true}`); !strings.Contains(r, `"account":"Rating"`) || !strings.Contains(r, strings.Repeat("x", 120)) {
		t.Fatal(r)
	}
}
