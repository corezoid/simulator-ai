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

func TestChartDataGet(t *testing.T) {
	var sent []map[string]any
	reply := `"result":{"data":[{"actorId":"a1","actorTitle":"Olena","nameId":"n1","accountName":"Revenue","currencyId":7,"currencyName":"USD","precision":2,` +
		`"incomeType":"credit","accountType":"fact","showTrendLine":false,"errorFormula":false,"value":520.5}],"total":{"value":520.5}}`
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.Method != "dashboard/get" {
			t.Errorf("unexpected request %s %v", q.Method, err)
		}
		sent = append(sent, q.Params)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,%s}`, q.ID, reply)
	}))
	defer backend.Close()
	client, err := publicapi.New(publicapi.Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "graph-read"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(client)
	tool, ok := s.ListTools()["chart_data_get"]
	if !ok {
		t.Fatal("chart_data_get missing from graph-read")
	}
	if a := tool.Tool.Annotations; a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint || a.IdempotentHint == nil || !*a.IdempotentHint {
		t.Fatal("chart_data_get is not annotated as a read")
	}
	call := func(args string) string {
		t.Helper()
		out, err := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"chart_data_get","arguments":%s}}`, args))))
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	out := call(`{"chartId":"chart","from":1788210000000,"to":1790888399000,"counterType":"amount","balanceOnDate":true,"actorIds":["a1"],"with":["total"]}`)
	if strings.Contains(out, `"isError":true`) || !strings.Contains(out, `"structuredContent":{"data":[{"actorId":"a1"`) || !strings.Contains(out, `"total":{"value":520.5}`) {
		t.Fatal(out)
	}
	if len(sent) != 1 || sent[0]["actorId"] != "chart" || sent[0]["accId"] != "workspace" || sent[0]["balanceOnDate"] != true || sent[0]["counterType"] != "amount" {
		t.Fatalf("sent %v", sent)
	}
	for _, args := range []string{`{}`, `{"actorId":"chart"}`, `{"chartId":"chart","accId":"other"}`, `{"chartId":"chart","with":["groups"]}`,
		`{"chartId":"chart","source":{"chartType":"bar"}}`, `{"chartId":"chart","interval":"week"}`, `null`} {
		if out := call(args); !strings.Contains(out, `"isError":true`) && !strings.Contains(out, `"error":`) {
			t.Fatal("invalid arguments accepted", args, out)
		}
	}
	if len(sent) != 1 {
		t.Fatal("refused arguments reached the backend")
	}
	for _, reason := range []string{"invalidParams", "accessDenied", "notFound"} {
		reply = fmt.Sprintf(`"error":{"code":-32602,"message":"refused","data":{"reason":%q}}`, reason)
		if out := call(`{"chartId":"chart","with":["series"]}`); !strings.Contains(out, `"isError":true`) || !strings.Contains(out, "reason="+reason) {
			t.Fatal(reason, out)
		}
	}
}
