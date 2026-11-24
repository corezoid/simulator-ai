package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestNumberTokens(t *testing.T) {
	for title, want := range map[string]string{"kb-eval Договір №22": "№22", "Invoice INV-7 (2026)": "INV-7,2026", "Acme": "", "Act 1, 2, 3": "1,2"} {
		if got := strings.Join(numberTokens(title), ","); got != want {
			t.Errorf("%q: %q, want %q", title, got, want)
		}
	}
}

// The card lists the payments of the actor: those it is a party to and those whose comment names its
// number, in one batch; a payment between actors nobody may see is only counted (eval transfers-14).
func TestCardPayments(t *testing.T) {
	leg := func(id, title string) string {
		return fmt.Sprintf(`{"actorId":%q,"actorTitle":%q,"amount":"50000","accountName":"Money","currencyName":"UAH"}`, id, title)
	}
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch {
		case method == "actor/context/get":
			return `{"actor":{"id":"c22","title":"Contract №22"},"data":[]}`
		case method == "transfer/filter" && p["query"] != nil:
			if string(p["query"]) != `"№22"` {
				t.Errorf("comment searched for %s", p["query"])
			}
			return `{"list":[{"id":"t1","type":"completed","comment":"Payment for contract №22","createdAt":2,"from":[` + leg("dev", "Development") + `],"to":[` + leg("nl", "Nordline") + `]},` +
				`{"id":"t0","createdAt":1,"comment":"refund №22","from":[{"actorId":"gone","amount":"5"}],"to":[{"actorId":"gone2","amount":"5"}]},` +
				`{"id":"t9","createdAt":3,"comment":"Payment for contract №221","from":[` + leg("dev", "Development") + `],"to":[` + leg("nl", "Nordline") + `]}]}`
		case method == "transfer/filter":
			return `{"list":[]}`
		}
		return `{}`
	})
	r := send("entity_context_get", `{"actorId":"c22","fields":["actor","payments"]}`)
	if !strings.Contains(r, `"payments":[{"id":"t1","createdAt":2,"type":"completed","comment":"Payment for contract №22","account":"Money","currency":"UAH","from":[{"actorId":"dev","title":"Development","amount":"50000"}],"to":[{"actorId":"nl","title":"Nordline","amount":"50000"}],"via":"comment"}]`) ||
		!strings.Contains(r, `"paymentsHidden":1`) || strings.Contains(r, `"t9"`) {
		t.Fatal(r)
	}
	for _, c := range *calls {
		if c.Method == "actor/context/get" && string(c.Params["fields"]) != `["actor"]` {
			t.Fatalf("payments sent to sim-api: %s", c.Params["fields"])
		}
	}
	*calls = nil
	if send("entity_context_get", `{"actorId":"c22","fields":["actor"]}`); len(*calls) != 1 {
		t.Fatalf("payments read without being asked: %v", *calls)
	}
}

// dateBy rides to sim-api as is: transfer/filter bounds and orders by the legs' business date
// itself (eval transfers-07); a long period is still read in 366-day windows, each with dateBy.
func TestTransfersByBusinessDate(t *testing.T) {
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		if strings.HasSuffix(method, "/count") {
			return `{"count":2}`
		}
		return `{"list":[{"id":"t1","originalDate":1776000000000}],"hasMore":false}`
	})
	r := send("transfers_filter", `{"from":1775001600000,"to":1777593600000,"dateBy":"originalDate","actorIds":["dev"]}`)
	if first := (*calls)[0]; !strings.Contains(r, `"originalDate":1776000000000`) || len(*calls) != 1 || first.Method != "transfer/filter" ||
		string(first.Params["dateBy"]) != `"originalDate"` || string(first.Params["actorIds"]) != `["dev"]` {
		t.Fatalf("%v %s", *calls, r)
	}
	*calls = nil
	const day = int64(24 * 3600 * 1000)
	if r = send("transfers_filter_count", fmt.Sprintf(`{"from":%d,"to":%d,"dateBy":"originalDate"}`, 1777593600000-500*day, 1777593600000)); !strings.Contains(r, `"count":4`) || len(*calls) != 2 {
		t.Fatalf("%d calls: %s", len(*calls), r)
	}
	for _, c := range *calls {
		if string(c.Params["dateBy"]) != `"originalDate"` {
			t.Fatalf("a window without dateBy: %v", c.Params)
		}
	}
}

// graph_traverse counts the reached actors per template (start and denied ones left out).
func TestTraverseCountsByForm(t *testing.T) {
	send, _ := kbServer(t, func(method string, _ map[string]json.RawMessage) string {
		if method == "graph/traverse" {
			return `{"nodes":[{"id":"d","formId":1,"formTitle":"Dept","depth":0},{"id":"p1","formId":2,"formTitle":"Project","depth":1},{"id":"c1","formId":3,"formTitle":"Contract","depth":2},` +
				`{"id":"c2","formId":3,"formTitle":"Contract","depth":2},{"id":"h","formId":3,"depth":2,"denied":true}],"edges":[]}`
		}
		return `{}`
	})
	if r := send("graph_traverse", `{"actorIds":["d"],"depth":2}`); !strings.Contains(r, `"countsByForm":[{"formId":3,"formTitle":"Contract","count":2},{"formId":2,"formTitle":"Project","count":1}]`) {
		t.Fatal(r)
	}
}

// The comment must hold the title's number as a whole word: sim-api's full-text query also finds
// parts of it.
func TestMentionsToken(t *testing.T) {
	for _, c := range []struct {
		comment, token string
		want           bool
	}{
		{"Оплата за договором №22", "№22", true}, {"оплата за ДОГОВОРОМ №22, етап 2", "№22", true},
		{"Оплата за договором №21, етап 2", "№2", false}, {"Договір №22", "№2", false},
		{"рахунок 22/2026 сплачено", "22/2026", true}, {"рахунок 122/2026", "22/2026", false}, {"№22", "22", true},
	} {
		if got := mentionsToken(c.comment, c.token); got != c.want {
			t.Errorf("%q in %q: %v", c.token, c.comment, got)
		}
	}
}
