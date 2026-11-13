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
	"github.com/santhosh-tekuri/jsonschema/v6"
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
	// The handler's hand-written result type answers what the pinned output schema declares.
	var envelope struct {
		Result struct {
			Structured any `json:"structuredContent"`
		} `json:"result"`
	}
	var schema any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tool.Tool.RawOutputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://test.example/chart", schema); err != nil {
		t.Fatal(err)
	}
	validator, err := compiler.Compile("https://test.example/chart")
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(envelope.Result.Structured); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0]["actorId"] != "chart" || sent[0]["accId"] != "workspace" || sent[0]["balanceOnDate"] != true || sent[0]["counterType"] != "amount" {
		t.Fatalf("sent %v", sent)
	}
	for _, args := range []string{`{}`, `{"actorId":"chart"}`, `{"chartId":"chart","accId":"other"}`, `{"chartId":"chart","with":["groups"]}`,
		`{"chartId":"chart","source":{"chartType":"bar"}}`, `{"chartId":"chart","interval":"week"}`, `{"chartId":"chart","timezoneOffset":721}`, `null`} {
		if out := call(args); !strings.Contains(out, `"isError":true`) && !strings.Contains(out, `"error":`) {
			t.Fatal("invalid arguments accepted", args, out)
		}
	}
	if len(sent) != 1 {
		t.Fatal("refused arguments reached the backend")
	}
	// 100 actorIds of 255 bytes are schema-valid and fit the argument bound.
	long := strings.Repeat("a", 255)
	ids := `"` + long + `"` + strings.Repeat(`,"`+long+`"`, 99)
	if out := call(`{"chartId":"chart","actorIds":[` + ids + `]}`); strings.Contains(out, `"isError":true`) {
		t.Fatal("bounded actorIds refused", out[:200])
	}
	// An answer past the bound is refused with how to narrow it, not handed to the model.
	points := strings.Repeat(`{"bucketStart":1788210000000,"value":1},`, 7000)
	reply = `"result":{"data":[{"actorId":"a1","nameId":"n1","currencyId":7,"incomeType":"total","accountType":"fact","value":0,"data":[` + points[:len(points)-1] + `]}]}`
	if out := call(`{"chartId":"chart","from":1,"to":2}`); !strings.Contains(out, `"isError":true`) || !strings.Contains(out, "coarser interval") {
		t.Fatal("oversized answer returned", len(out))
	}
	for _, reason := range []string{"invalidParams", "accessDenied", "notFound"} {
		reply = fmt.Sprintf(`"error":{"code":-32602,"message":"refused","data":{"reason":%q}}`, reason)
		if out := call(`{"chartId":"chart","with":["series"]}`); !strings.Contains(out, `"isError":true`) || !strings.Contains(out, "reason="+reason) {
			t.Fatal(reason, out)
		}
	}
}
