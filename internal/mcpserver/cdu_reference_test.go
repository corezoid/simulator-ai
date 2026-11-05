package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func catalogFixture(t *testing.T) json.RawMessage {
	t.Helper()
	// sim-api's cdu/catalog/get answer for 15 types (Stack, Form, inputs, Table, Tabs, Row/Col, Card,
	// Button, Label, the ActorList block), as the 2.0 contracts produce it.
	b, err := os.ReadFile("testdata/cdu-catalog-core.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureDocs(t *testing.T) []referenceDoc {
	t.Helper()
	components, err := componentsDoc(catalogFixture(t), typeNotes)
	if err != nil {
		t.Fatal(err)
	}
	return []referenceDoc{components, protocolDoc, examplesDoc}
}

func allWindows(t *testing.T, docs []referenceDoc) []docWindow {
	t.Helper()
	var out []docWindow
	for _, d := range docs {
		w, err := window(docs, d.Name)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, w)
	}
	return out
}

// The components document is the catalog one section per type: purpose, children, a line per prop.
func TestReferenceComponentsDoc(t *testing.T) {
	d := fixtureDocs(t)[0]
	text := strings.Join(d.Lines, "\n")
	for _, want := range []string{
		"# CDU components (schemaVersion 2.0): 15 types",
		"## Stack (primitive, Layout)\nLays its children out in a row or a column, with a gap between them.\nchildren: default — any children\nprops:\n",
		"  size?: 'none'|'min'|'3xs'|'2xs'|'xs'|'sm'|'md'|'lg'|'xl'|'2xl'|'3xl'|'4xl'|'5xl'|'6xl'|'7xl'|'8xl'|'9xl'|'10xl'|'11xl' | {{expr}} — Gap size between items",
		"## Row (primitive, Layout)", "children: default — allows Col", "## DatePicker (primitive, Input) — Date Picker\n",
		"## ActorList (block, ", "examples (props):\n  Graphs with a custom row action — ", `: {"formTitle":"Graphs","defaultPageSize":25,`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("components lacks %q", want)
		}
	}
	if strings.Index(text, "## ActorList") < strings.Index(text, "## TextInput") {
		t.Error("primitives come before blocks")
	}
	if _, err := componentsDoc(json.RawMessage(`{"schemaVersion":"2.0","list":[]}`), nil); err == nil {
		t.Fatal("an empty catalog must fail")
	}
}

func TestReferenceGrep(t *testing.T) {
	docs := fixtureDocs(t)
	windows := allWindows(t, docs)
	out, err := grepReference(windows, nil, "gap|spacing", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("gap|spacing: %d chars", len(out))
	stack := sectionLine(docs[0], "Stack")
	heading := fmt.Sprintf("%d- ## Stack (primitive, Layout)", stack)
	if !strings.Contains(out, heading) || !strings.Contains(out, ":   size?: 'none'") || !strings.Contains(out, "--- components (") {
		t.Fatalf("grep gap must find Stack size under its section:\n%s", out)
	}
	if out, _ = grepReference(windows, nil, `^formId: Identifier — The .Form. node`, 0, 0); !strings.Contains(out, "protocol=") || !strings.Contains(out, "### form.submit — Submit a form") {
		t.Fatalf("form.submit:\n%s", out)
	}
	// context lines carry '-', hits ':'
	w, _ := window(docs, "protocol/form.submit")
	if out, _ = grepReference([]docWindow{w}, nil, "^formId", 1, 0); !strings.Contains(out, "protocol-") || !strings.Contains(out, "protocol:") || strings.Contains(out, "--- components") {
		t.Fatalf("doc filter and context:\n%s", out)
	}
	// the cap: every line matches
	out, _ = grepReference(windows, []string{"components: not available (x)"}, ".", 2, 200)
	if len(out) > grepMaxChars+300 || !strings.Contains(out, "hits not shown: narrow the pattern") || !strings.HasPrefix(out, "components: not available (x)\n") {
		t.Fatalf("uncapped (%d chars):\n%s", len(out), out[len(out)-300:])
	}
	if out, _ = grepReference(windows, nil, "zzqqxx", 2, 0); !strings.HasPrefix(out, "no hits for /zzqqxx/") {
		t.Fatal(out)
	}
	if _, err = grepReference(windows, nil, "(?<=a)b", 2, 0); err == nil || !strings.Contains(err.Error(), "RE2 syntax") {
		t.Fatal("invalid regex:", err)
	}
	// the limit counts hits
	if out, _ = grepReference(windows, nil, "^## ", 0, 3); strings.Count(out, ": ## ") != 3 || !strings.Contains(out, "hits not shown") {
		t.Fatalf("limit 3:\n%s", out)
	}
}

func sectionLine(d referenceDoc, id string) int {
	for i, l := range d.Lines {
		if headingLevel(l) > 0 && sectionID(l) == id {
			return i + 1
		}
	}
	return -1
}

func TestReferenceRead(t *testing.T) {
	docs := fixtureDocs(t)
	w, _ := window(docs, "protocol")
	out, err := readReference(w, 0, 10)
	if err != nil || !strings.HasPrefix(out, fmt.Sprintf("protocol: lines 1–10 of %d\n1: # CDU v2 page protocol", len(protocolDoc.Lines))) || !strings.HasSuffix(out, `more: {doc: "protocol", offset: 11}`) {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _ = readReference(w, 11, 400); len(out) > readMaxChars || !strings.Contains(out, "more:") {
		t.Fatalf("read cap: %d chars", len(out))
	}
	w, err = window(docs, "components/datepicker")
	if err != nil {
		t.Fatal(err)
	}
	out, _ = readReference(w, 0, 0)
	if !strings.Contains(out, "(section components/DatePicker: lines") || !strings.Contains(out, ": ## DatePicker") || strings.Contains(out, "## Select") || strings.Contains(out, "more:") {
		t.Fatalf("section read:\n%s", out)
	}
	if _, err = readReference(w, 1, 10); err == nil {
		t.Fatal("an offset outside the section must fail")
	}
	if _, err = window(docs, "components/Nope"); err == nil || !strings.Contains(err.Error(), `grep "^## "`) {
		t.Fatal(err)
	}
	if _, err = window(docs, "manual"); err == nil {
		t.Fatal("unknown document")
	}
	w, _ = window(docs, "examples/E12-conditional-validation")
	if out, _ = readReference(w, 0, 400); !strings.Contains(out, `"type": "Form"`) || !strings.Contains(out, "pages/E12-conditional-validation/config:") {
		t.Fatalf("example section:\n%s", out[:300])
	}
	list := listReference(docs, []string{"components: not available (x)"})
	if !strings.Contains(list, "- components — ") || !strings.Contains(list, "15 types") || !strings.Contains(list, "sections: page, node, expressions, bindings, actions,") ||
		!strings.Contains(list, "sections: E12-conditional-validation, E11-wizard") {
		t.Fatal(list)
	}
}

// The catalog is fetched once per catalogTTL; a failure is not kept.
func TestReferenceCache(t *testing.T) {
	now := time.Unix(1000, 0)
	fetches := 0
	fail := true
	c := &componentsCache{now: func() time.Time { return now }, fetch: func(context.Context) (json.RawMessage, error) {
		fetches++
		if fail {
			return nil, errors.New("down")
		}
		return catalogFixture(t), nil
	}}
	if _, err := c.get(context.Background()); err == nil {
		t.Fatal("fetch error")
	}
	fail = false
	for range 3 {
		if _, err := c.get(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(catalogTTL + time.Second)
	_, _ = c.get(context.Background())
	if fetches != 3 {
		t.Fatalf("%d fetches, want 3 (failed, first, after the TTL)", fetches)
	}
}

func toolText(t *testing.T, send func(string) string, tool, args string) (string, bool) {
	t.Helper()
	var r struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(send(fmt.Sprintf(`"method":"tools/call","params":{"name":%q,"arguments":%s}`, tool, args))), &r); err != nil || len(r.Result.Content) != 1 {
		t.Fatal(err)
	}
	return r.Result.Content[0].Text, r.Result.IsError
}

func TestReferenceTools(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{"cdu/catalog/get": `"result":` + string(catalogFixture(t))})
	text, isErr := toolText(t, send, "smart_form_reference_grep", `{"pattern":"gap|spacing"}`)
	if isErr || !strings.Contains(text, "size?: 'none'") || !strings.Contains(text, "## Stack") {
		t.Fatal(text)
	}
	if text, isErr = toolText(t, send, "smart_form_reference_read", `{"doc":"components/Stack","limit":5}`); isErr || !strings.Contains(text, ": ## Stack (primitive, Layout)") {
		t.Fatal(text)
	}
	if text, _ = toolText(t, send, "smart_form_reference_read", `{}`); !strings.Contains(text, "- protocol — ") {
		t.Fatal(text)
	}
	if text, _ = toolText(t, send, "smart_form_reference_grep", `{"pattern":"DatePicker","doc":"protocol"}`); strings.Contains(text, "components") {
		t.Fatal("doc protocol greps protocol only:", text)
	}
	if text, isErr = toolText(t, send, "smart_form_reference_grep", `{"pattern":"("}`); !isErr || !strings.Contains(text, "invalid pattern") {
		t.Fatal(text)
	}
	if methodsOf(*calls) != "cdu/catalog/get" || (*calls)[0].Params["brief"] != nil || (*calls)[0].Params["types"] != nil {
		t.Fatalf("one full catalog read per server: %s %v", methodsOf(*calls), (*calls)[0].Params)
	}
	// without the catalog the embedded documents still answer
	send, _ = consoleServer(t, map[string]string{"cdu/catalog/get": `"error":{"code":-32000,"message":"denied"}`})
	if text, isErr = toolText(t, send, "smart_form_reference_grep", `{"pattern":"form\\.submit"}`); isErr || !strings.HasPrefix(text, "components: not available") || !strings.Contains(text, "### form.submit") {
		t.Fatal(text)
	}
	if _, isErr = toolText(t, send, "smart_form_reference_read", `{"doc":"components"}`); !isErr {
		t.Fatal("components alone must fail without the catalog")
	}
}

// The embedded documents name the commits they were generated from (make gen-check regenerates them).
func TestReferenceEmbedded(t *testing.T) {
	for _, d := range []referenceDoc{protocolDoc, examplesDoc} {
		if len(d.Lines) < 100 || !strings.HasPrefix(d.Lines[1], "Generated by cmd/gen-cdu-reference from sim-api ") {
			t.Errorf("%s: %d lines, header %q", d.Name, len(d.Lines), d.Lines[1])
		}
	}
	w, err := window([]referenceDoc{protocolDoc}, "protocol/definitions")
	if err != nil {
		t.Fatal(err)
	}
	section := strings.Join(protocolDoc.Lines[w.lo:w.hi], "\n")
	for _, want := range []string{"Contract: $ref is mutually exclusive with type/props/children/actions.", "Give every instance its own id", `"$ref":"#/definitions/field-row","for":`} {
		if !strings.Contains(section, want) {
			t.Errorf("protocol/definitions lacks %q", want)
		}
	}
	if len(typeNotes) < 50 || typeNotes["Stack"].Purpose == "" {
		t.Fatal("type notes", len(typeNotes))
	}
}
