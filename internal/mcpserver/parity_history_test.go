package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

// history_list narrows by the stored key: a template field named by its title or bare id is sent
// as data.<id>, a column as is; a window in Unix seconds is read as seconds (eval asof-07).
func TestHistoryListField(t *testing.T) {
	send, calls := kbServer(t, func(method string, p map[string]json.RawMessage) string {
		switch method {
		case "actor/get":
			if string(p["actorId"]) == `"gone"` {
				return `{"error":"not found"}`
			}
			return `{"id":"c30","formId":7}`
		case "form/get":
			return `{"id":7,"title":"Договір","form":{"sections":[{"content":[{"id":"amount","class":"edit","title":"Сума"},{"id":"note","class":"label","title":"Note"},{"id":"state","class":"select","title":"Status"}]}]}}`
		case "history/list":
			var field string
			_ = json.Unmarshal(p["field"], &field)
			if field != "" && field != "data.amount" && field != "title" {
				return `{"list":[]}`
			}
			return `{"list":[{"field":"data.amount","fieldTitle":"Сума","prevValue":210000,"nextValue":230000}]}`
		}
		return `{}`
	})
	last := func() kbCall { return (*calls)[len(*calls)-1] }
	for _, field := range []string{"Сума", " сума ", "amount", "data.amount"} {
		r := send("history_list", `{"objType":"actor","objId":"c30","from":0,"to":1795540863000,"field":"`+field+`"}`)
		if !strings.Contains(r, `"nextValue":230000`) || string(last().Params["field"]) != `"data.amount"` {
			t.Fatalf("%q: sent %s: %s", field, last().Params["field"], r)
		}
	}
	*calls = nil
	if r := send("history_list", `{"objType":"actor","objId":"c30","from":0,"to":1795540863000,"field":"title"}`); len(*calls) != 1 || string(last().Params["field"]) != `"title"` || strings.Contains(r, "fieldKey") {
		t.Fatalf("a column: %v %s", *calls, r)
	}
	// A template field titled like a column does not take it over; a failed lookup sends the field as is.
	*calls = nil
	if r := send("history_list", `{"objType":"actor","objId":"c30","from":0,"to":1795540863000,"field":"Status"}`); len(*calls) != 1 || string(last().Params["field"]) != `"status"` || !strings.Contains(r, `"fieldKey":"status"`) {
		t.Fatalf("column Status: %v %s", *calls, r)
	}
	*calls = nil
	if r := send("history_list", `{"objType":"actor","objId":"gone","from":0,"to":1795540863000,"field":"Сума"}`); string(last().Params["field"]) != `"Сума"` || strings.Contains(r, "isError") {
		t.Fatalf("deleted actor: %v %s", *calls, r)
	}
	*calls = nil
	r := send("history_list", `{"objType":"actor","objId":"c30","from":1795000000,"to":1795540863}`)
	if string(last().Params["from"]) != "1795000000000" || string(last().Params["to"]) != "1795540863000" || !strings.Contains(r, "read as seconds") || len(*calls) != 1 {
		t.Fatalf("%v %s", *calls, r)
	}
	*calls = nil
	if send("history_list", `{"objType":"account","objId":"a1","from":0,"to":1795540863000,"field":"Сума"}`); len(*calls) != 1 || string(last().Params["field"]) != `"Сума"` {
		t.Fatalf("another object type: %v", *calls)
	}
}
