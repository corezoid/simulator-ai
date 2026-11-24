package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

type kbCall struct {
	Method string
	Params map[string]json.RawMessage
}

// kbServer is a console-full server over a stand whose answers come from reply (a batch is
// answered item by item); calls records every method, batch items included.
func kbServer(t *testing.T, reply func(method string, params map[string]json.RawMessage) string) (func(tool, args string) string, *[]kbCall) {
	t.Helper()
	calls := &[]kbCall{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string                     `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		result := ""
		if q.Method == "batch" {
			var list []struct {
				Method string                     `json:"method"`
				Params map[string]json.RawMessage `json:"params"`
			}
			_ = json.Unmarshal(q.Params["list"], &list)
			items := make([]string, len(list))
			for i, item := range list {
				*calls = append(*calls, kbCall{item.Method, item.Params})
				items[i] = fmt.Sprintf(`{"id":"%d","result":%s}`, i, reply(item.Method, item.Params))
			}
			result = `{"list":[` + strings.Join(items, ",") + `]}`
		} else {
			*calls = append(*calls, kbCall{q.Method, q.Params})
			result = reply(q.Method, q.Params)
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, result)
	}))
	t.Cleanup(api.Close)
	c, err := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator user-jwt"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	return func(tool, args string) string {
		msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tool, args)
		b, _ := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(msg)))
		return string(b)
	}, calls
}

func TestPeriodWindows(t *testing.T) {
	// Closed windows [from, to] (the lists) meet at to = previous from - 1; half-open ones
	// [from, to) (the filters) at to = previous from: no millisecond lost or read twice.
	for _, open := range []bool{false, true} {
		gap := int64(1)
		if open {
			gap = 0
		}
		ws, cut := windows(0, 3*maxWindowMs, open)
		if len(ws) != 3 || cut || ws[0].to != 3*maxWindowMs || ws[2].from != 0 {
			t.Fatalf("open %v: %+v %v", open, ws, cut)
		}
		for i, w := range ws {
			if w.to-w.from > maxWindowMs || w.to <= w.from || i > 0 && w.to != ws[i-1].from-gap {
				t.Fatalf("open %v, window %d: %+v", open, i, ws)
			}
		}
		if ws, cut = windows(0, 10*maxWindowMs, open); len(ws) != maxWindows || !cut {
			t.Fatalf("ceiling: %d windows, cut %v", len(ws), cut)
		}
	}
}

// A period over 366 days is read window by window, newest first: counts are summed, pages joined up
// to the limit with the call that reads on; a short one is one plain call.
func TestLedgerLongPeriod(t *testing.T) {
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		var from, to, limit int64
		_ = json.Unmarshal(p["from"], &from)
		_ = json.Unmarshal(p["to"], &to)
		_ = json.Unmarshal(p["limit"], &limit)
		if to-from > maxWindowMs {
			return `{"error":"window too long"}`
		}
		if strings.HasSuffix(method, "/count") {
			return `{"count":4}`
		}
		rows := []string{}
		for i := int64(0); i < min(2, limit); i++ {
			rows = append(rows, fmt.Sprintf(`{"id":"%d-%d"}`, to, i))
		}
		return `{"list":[` + strings.Join(rows, ",") + `],"hasMore":false,"appliedAccounts":[]}`
	})
	const to = int64(1_800_000_000_000)
	from := to - 800*24*3600*1000
	if r := send("transfers_filter_count", fmt.Sprintf(`{"from":%d,"to":%d}`, from, to)); !strings.Contains(r, `"count":12`) || !strings.Contains(r, `"windows":3`) {
		t.Fatal(r)
	}
	*calls = nil
	r := send("transfers_filter", fmt.Sprintf(`{"from":%d,"to":%d,"limit":3}`, from, to))
	if !strings.Contains(r, `"hasMore":true`) || strings.Count(r, `"id":"`) != 3 || !strings.Contains(r, `"appliedAccounts":[]`) ||
		!strings.Contains(r, fmt.Sprintf(`"next":{"from":%d,"to":%d}`, from, to-2*maxWindowMs)) || len(*calls) != 2 {
		t.Fatalf("%d calls: %s", len(*calls), r)
	}
	// next keeps dateBy: the windows were on the business date.
	if r = send("transfers_filter", fmt.Sprintf(`{"from":%d,"to":%d,"limit":3,"dateBy":"originalDate"}`, from, to)); !strings.Contains(r, `"dateBy":"originalDate"`) || !strings.Contains(r, `"next":{`) {
		t.Fatal(r)
	}
	if r = send("transactions_filter", fmt.Sprintf(`{"from":%d,"to":%d,"offset":50}`, from, to)); !strings.Contains(r, `"isError":true`) || !strings.Contains(r, "cursor") {
		t.Fatal(r)
	}
	*calls = nil
	if r = send("account_transactions_list", fmt.Sprintf(`{"accountId":"a1","from":%d,"to":%d}`, to-1000, to)); strings.Contains(r, "isError") || len(*calls) != 1 || string((*calls)[0].Params["from"]) != fmt.Sprint(to-1000) {
		t.Fatalf("%v %s", *calls, r)
	}
}

const dialogMeta = `"metaInfo":{"aiDialogParentId":"layer-1"}`

// sim-api leaves the AI Console dialogs out of the search and the backlinks itself: includeAiDialogs
// reaches it; a dialog's own thread, which it does not filter, needs includeAiDialogs here.
func TestAIDialogsLeftOut(t *testing.T) {
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch method {
		case "actor/search/list":
			return `{"list":[{"id":"d1","title":"Dialog",` + dialogMeta + `},{"id":"a1","title":"Acme","metaInfo":{}}]}`
		case "actor/get":
			if string(p["actorId"]) == `"dlg"` {
				return `{"id":"dlg",` + dialogMeta + `}`
			}
			return `{"id":` + string(p["actorId"]) + `}`
		case "actor/backlinks/get":
			return `{"mentions":[{"actorId":"c1","rootActorId":"dlg","snippet":"x"},{"actorId":"doc","snippet":"Договір"}],"mentionsHasMore":false}`
		case "reaction/list":
			return `{"list":[{"id":"r1"}]}`
		case "graph/layer/actors/search/list":
			return `{"list":[{"id":"d1","actor":{"id":"d1","title":"Dialog",` + dialogMeta + `}},{"id":"a1","actor":{"id":"a1","title":"Acme","metaInfo":{}}}]}`
		}
		return `{}`
	})
	// The layer search sim-api does not filter: the plugin adds metaInfo, drops the dialogs and
	// the metaInfo it added, and counts them.
	if r := send("layer_actors_search", `{"actorId":"layer-1","query":"a"}`); !strings.Contains(r, `"aiDialogsHidden":1`) || strings.Contains(r, `"Dialog"`) ||
		strings.Contains(r, "metaInfo") || string((*calls)[0].Params["fields"]) != `["id","title","formId","metaInfo"]` {
		t.Fatalf("layer search: %s %s", (*calls)[0].Params["fields"], r)
	}
	// sim-api filters the search and the backlinks itself: the switch reaches it as sent, the
	// projection stays the caller's and nothing is filtered again here.
	for _, tool := range []string{"actors_search", "actor_backlinks_get"} {
		for _, args := range []string{`{"query":"acme","actorId":"acme"}`, `{"query":"acme","actorId":"acme","includeAiDialogs":true}`} {
			if tool == "actors_search" {
				args = strings.Replace(args, `,"actorId":"acme"`, "", 1)
			} else {
				args = strings.Replace(args, `"query":"acme",`, "", 1)
			}
			*calls = nil
			r := send(tool, args)
			sent := (*calls)[0].Params
			if strings.Contains(r, "isError") || strings.Contains(r, "aiDialogsHidden") || len(*calls) != 1 ||
				strings.Contains(args, "includeAiDialogs") != (string(sent["includeAiDialogs"]) == "true") ||
				tool == "actors_search" && string(sent["fields"]) != `["id","title","formId"]` {
				t.Fatalf("%s %s: sent %v: %s", tool, args, sent, r)
			}
		}
	}
	r := send("search_all", `{"query":"acme","filters":["actors"],"includeAiDialogs":true}`)
	if last := (*calls)[len(*calls)-1]; strings.Contains(r, "isError") || last.Method != "actor/search/list" || string(last.Params["includeAiDialogs"]) != "true" {
		t.Fatalf("search_all: %v %s", last, r)
	}
	if r = send("reactions_list", `{"rootActorId":"dlg"}`); !strings.Contains(r, `"aiDialog":true`) || strings.Contains(r, `"r1"`) {
		t.Fatal(r)
	}
	for _, args := range []string{`{"rootActorId":"dlg","includeAiDialogs":true}`, `{"rootActorId":"acme"}`} {
		if r = send("reactions_list", args); !strings.Contains(r, `"r1"`) {
			t.Fatal(args, r)
		}
	}
}

// The card's money edges become one line per counterparty with the summed amount and the
// transfers behind it; full keeps them as edges. graph_get names its money edges as transfers.
func TestMoneyEdges(t *testing.T) {
	long := strings.Repeat("я", 400)
	card := `{"actor":{"id":"acme","title":"ТОВ Акме Софт"},"data":[{"id":"note","title":"Note","value":"` + long + `"}],` +
		`"neighbors":[{"edgeTypeId":1,"edgeType":"hierarchy","role":"hierarchy","direction":"in","neighbors":[{"actorId":"p","title":"Parent"}],"hasMore":false},` +
		`{"edgeTypeId":9,"edgeType":"transfersAmount_n_1_5","role":"transactions","direction":"in","hasMore":true,"neighbors":[` +
		`{"actorId":"dev","title":"Відділ розробки","weight":"200000.00","linkedActorId":"t1"},{"actorId":"dev","title":"Відділ розробки","weight":"50000","linkedActorId":"t2"},` +
		`{"actorId":"sales","title":"Відділ продажів","weight":"60000","linkedActorId":"t3"}]}],` +
		`"accounts":[{"nameId":"n_1","name":"Гроші","currencyId":5,"currency":"UAH","type":"fact","debit":300000,"credit":900000.5}]}`
	graph := `{"nodes":[{"id":"dev","title":"Dev"},{"id":"acme","title":"Acme"}],"edges":[{"id":"e1","source":"dev","target":"acme","edgeTypeId":9,"weight":250000,"linkedActorId":"t1"},{"id":"e2","source":"acme","target":"dev","edgeTypeId":1}]}`
	send, _ := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch method {
		case "graph/get":
			return graph
		case "actor/get": // the Transfers actors: ref = the transfer id; t3 is not readable here
			if id := strings.Trim(string(p["actorId"]), `"`); id != "t3" {
				return `{"id":"` + id + `","ref":"tr-` + id + `"}`
			}
			return `{"id":"t3"}`
		}
		return card
	})
	r := send("entity_context_get", `{"actorId":"acme"}`)
	for _, want := range []string{`"direction":"in","actorId":"dev","title":"Відділ розробки","amount":"250000","account":"Гроші","currency":"UAH","transferIds":["tr-t1","tr-t2"],"partial":true,"edgeTypeId":9`,
		`"actorId":"sales","title":"Відділ продажів","amount":"60000","account":"Гроші","currency":"UAH","transferActorIds":["t3"],"partial":true`, `"transfersHasMore":true`, `"edgeType":"hierarchy"`, "я…", `"balance":600000.5`} {
		if !strings.Contains(r, want) {
			t.Fatalf("missing %s in %s", want, r)
		}
	}
	if strings.Contains(r, "transfersAmount_") || strings.Contains(r, long) {
		t.Fatal(r)
	}
	if r = send("entity_context_get", `{"actorId":"acme","full":true}`); !strings.Contains(r, "transfersAmount_n_1_5") || strings.Contains(r, `"transfers":`) {
		t.Fatal(r)
	}
	if r = send("graph_get", `{"actorId":"acme"}`); !strings.Contains(r, `"transfers":[{"from":"dev","fromTitle":"Dev","to":"acme","toTitle":"Acme","amount":"250000","edgeTypeId":9,"transferId":"tr-t1"}]`) {
		t.Fatal(r)
	}
}
