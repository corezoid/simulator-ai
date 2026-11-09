package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

// A render answers a summary unless full is sent; what the server left unresolved is a warning.
func TestRenderSummary(t *testing.T) {
	render := `{"schemaVersion":"2.0","pageId":"index","title":"Leave","tree":{"id":"root","type":"Stack","children":{"header":[{"id":"h","type":"Label"}],"body":[{"id":"r","$ref":"#/definitions/row"}]}},"bindings":[{"on":"x","do":[]}],"viewModel":{"b":1,"a":2},"locale":{"en":{},"uk":{}}}`
	send, calls := consoleServer(t, map[string]string{"page/render": `"result":` + render})
	reply := callTool(send, "smart_form_page_render", `{"ref":"leave_form","envTitle":"develop"}`)
	want := `{"ok":true,"pageId":"index","title":"Leave","nodes":3,"bindings":1,"vmKeys":["a","b"],"locales":["en","uk"],"bytes":274,"warnings":["1 $ref nodes were not replaced by their definitions"]}`
	if !strings.Contains(reply, want) || (*calls)[0].Params["full"] != nil {
		t.Fatal(reply, (*calls)[0].Params)
	}
	if reply = callTool(send, "smart_form_page_render", `{"ref":"leave_form","envTitle":"develop","full":true}`); !strings.Contains(reply, `"tree":{"id":"root"`) {
		t.Fatal(reply)
	}
	send, _ = consoleServer(t, map[string]string{"page/render": `"result":{"pageId":"index","tree":null,"viewModel":{},"locale":{}}`})
	if reply = callTool(send, "smart_form_page_render", `{"ref":"leave_form"}`); !strings.Contains(reply, `"warnings":["the tree has no nodes"]`) {
		t.Fatal(reply)
	}
	send, _ = consoleServer(t, map[string]string{"page/render": `"error":{"code":1,"message":"invalid CDU source in page 'index': missing definition","data":{"reason":"invalidParams"}}`})
	if reply = callTool(send, "smart_form_page_render", `{"ref":"leave_form"}`); !strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, "missing definition") {
		t.Fatal(reply)
	}
	keys := map[string]json.RawMessage{}
	for i := range maxRenderKeys + 5 {
		keys[strings.Repeat("k", i+1)] = json.RawMessage("1")
	}
	vm, _ := json.Marshal(keys)
	out, err := renderSummary(nil, json.RawMessage(`{"pageId":"p","tree":{"type":"Stack"},"viewModel":`+string(vm)+`}`))
	if err != nil || !strings.Contains(string(out), `"… 5 more"]`) {
		t.Fatal(string(out), err)
	}
	if out, _ = renderSummary(nil, json.RawMessage(`[1]`)); string(out) != `[1]` {
		t.Fatal(string(out))
	}
}

// Every schema byte of a Smart Form run rides in the model context on every step: the tools this
// work changed, the smart_forms group and the plain console catalog keep to these sizes.
func TestSmartFormSchemaBudgets(t *testing.T) {
	size := func(profile string, names ...string) int {
		tools := consoleTools(t, profile).ListTools()
		if len(names) == 0 {
			for name := range tools {
				names = append(names, name)
			}
		}
		n := 0
		for _, name := range names {
			b, _ := json.Marshal(tools[name].Tool)
			n += len(b)
		}
		return n
	}
	for _, tc := range []struct {
		what   string
		got    int
		budget int
	}{
		{"smart_form_files_get", size("console-full", "smart_form_files_get"), 2100},
		{"smart_form_files_update", size("console-full", "smart_form_files_update"), 2600},
		{"smart_form_page_render", size("console-full", "smart_form_page_render"), 1200},
		{"group smart_forms", size("console-full", consoleGroups["smart_forms"]...), 31 << 10},
		{"console catalog", size("console"), 108 << 10},
	} {
		t.Logf("%s: %d bytes", tc.what, tc.got)
		if tc.got > tc.budget {
			t.Errorf("%s is %d bytes of tools/list, budget %d", tc.what, tc.got, tc.budget)
		}
	}
}
