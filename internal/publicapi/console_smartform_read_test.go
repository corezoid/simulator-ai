package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const outlinePage = `{"schemaVersion":"2.0","id":"index","tree":{"id":"root","type":"Stack","children":[
 {"id":"card","type":"Card","children":{"header":[{"id":"title","type":"Label","props":{"text":"A very long caption that does not fit the outline line","size":"lg"}}],"body":[{"type":"Label","props":{"text":"no id"}}]}},
 {"id":"rows","$ref":"#/definitions/row","for":{"items":"vm.rows","as":"r"},"if":"vm.show"},
 {"id":"open","type":"Button","props":{"label":"Open","disabled":false,"value":3},"actions":{"onClick":[{"type":"overlay.open","content":{"id":"dialog","type":"Modal","children":[{"id":"inside","type":"Label"}]}}]}}]},
 "bindings":[{"on":"x","do":[{"type":"overlay.open","content":[{"id":"drawer","type":"Drawer"}]}]}]}`

func parsed(t *testing.T, source string) any {
	t.Helper()
	doc, err := decodeDoc([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestOutline(t *testing.T) {
	page := parsed(t, outlinePage)
	want := `9 nodes
Stack#root
 Card#card
  [header] Label#title text="A very long caption that does no…"
  [body] Label@/tree/children/0/children/body/0 text="no id"
 $definitions/row#rows for if
 Button#open label="Open" value=3 on:onClick
  [overlay] Modal#dialog +1
[binding overlay] Drawer#drawer`
	if got := outlineDoc(page, nil, 3, 4000); got != want {
		t.Fatalf("outline:\n%s", got)
	}
	if got := outlineDoc(page, nil, 1, 4000); got != "9 nodes\nStack#root +7\n[binding overlay] Drawer#drawer" {
		t.Fatalf("one level:\n%s", got)
	}
	if got := outlineDoc(page, nil, 9, 60); !strings.HasSuffix(got, "… cut: ask fewer levels, or outline one node (nodeId)") || len(got) > 130 {
		t.Fatalf("cut:\n%s", got)
	}
	keys := parsed(t, `{"orders":[1,2],"user":{"name":"Ann","age":3,"tags":null},"ok":true}`)
	if got := outlineDoc(keys, nil, 3, 4000); got != "/ {3}\n orders [2]\n user {3}\n  name \"Ann\"\n  age 3\n  tags null\n ok true" {
		t.Fatalf("keys:\n%s", got)
	}
	if got := outlineDoc(parsed(t, `[{"id":"a","type":"Label"},{"id":"b","type":"Label"}]`), []string{"tree", "children"}, 2, 4000); got != "Label#a\nLabel#b" {
		t.Fatalf("list:\n%s", got)
	}
	if got := outlineDoc(parsed(t, `{"a":1}`), []string{"vm", "user"}, 1, 4000); got != "user {1}" {
		t.Fatalf("part:\n%s", got)
	}
}

func TestCutSource(t *testing.T) {
	for _, tc := range []struct {
		source string
		limit  int
		want   string
	}{
		{"abcdef", 4, "abcd"},
		{"abc", 10, "abc"},
		{`a"b"c`, 4, `a"b`}, // a quote is two bytes in the answer
		{"привіт", 5, "пр"}, // never inside a character
		{"<<<<", 12, "<<"},  // < is six bytes in the answer
		{"abc", 0, ""},
	} {
		if got := cutSource(tc.source, tc.limit); got != tc.want || encodedLen(got) > tc.limit {
			t.Errorf("cutSource(%q, %d) = %q", tc.source, tc.limit, got)
		}
	}
}

func TestStaleWrite(t *testing.T) {
	stored := `{"schemaVersion":"2.0","id":"p","tree":{"id":"root","type":"Stack","children":[{"id":"a","type":"Label"},{"id":"added","type":"Label"}]}}`
	sent := `{"schemaVersion":"2.0","id":"p","tree":{"id":"root","type":"Stack","children":[{"id":"a","type":"Label"}]}}`
	api := &APIError{Code: 1, Reason: "appFileSourceMismatch", stale: []staleFile{{ID: 7, Title: "config", Source: stored, SourceHash: "h2"}}}
	got := staleWrite(api, []SmartFormFileUpdate{{ID: 7, Source: &sent}}).Error()
	if !strings.HasPrefix(got, "not written: config (id 7) changed since you read it: its sourceHash is now h2 (it now has nodes yours lacks: added). Read what changed") || strings.Contains(got, "schemaVersion") {
		t.Fatal(got)
	}
	if got = lostBy(sent, stored); got != " (it has no node yours lacks; props may differ)" {
		t.Fatal(got)
	}
	if got = lostBy("a { color: red }", "b {}"); got != " (16 bytes now, 4 sent)" {
		t.Fatal(got)
	}
	var many []string
	for i := range maxStaleIDs + 3 {
		many = append(many, `{"id":"n`+string(rune('a'+i))+`","type":"Label"}`)
	}
	if got = lostBy(`{"tree":{"id":"root","type":"Stack","children":[`+strings.Join(many, ",")+`]}}`, `{"tree":{"id":"root","type":"Stack"}}`); !strings.HasSuffix(got, "nl and 3 more)") {
		t.Fatal(got)
	}
	plain := errors.New("x")
	if !errors.Is(staleWrite(plain, nil), plain) {
		t.Fatal("another error must pass as it is")
	}
	// The server's refusal carries the whole current source: it stays out of the message.
	c, err := New(Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	wire := &rpcError{Code: 1, Message: "the file changed"}
	wire.Data.Reason = "appFileSourceMismatch"
	wire.Data.ReasonParams = json.RawMessage(`{"files":[{"id":7,"title":"config","source":"SECRET SOURCE","sourceHash":"h2"}]}`)
	if e := c.apiError(wire); strings.Contains(e.Error(), "SECRET") || len(e.stale) != 1 || e.stale[0].SourceHash != "h2" {
		t.Fatal(e)
	}
	wire.Data.ReasonParams = json.RawMessage(`{"files":"?"}`)
	if e := c.apiError(wire); len(e.stale) != 0 || !strings.Contains(e.Error(), "the file changed") {
		t.Fatal(e)
	}
}

// The reads of a develop tree against the stateful backend: a listing, sources within the answer's
// budget, parts of one file, and what each refuses.
func TestSmartFormReads(t *testing.T) {
	c, f := fakeClient(t)
	ctx := context.Background()
	app, err := c.CreateSmartForm(ctx, SmartFormCreateInput{Ref: "reads", Title: "Reads"})
	if err != nil {
		t.Fatal(err)
	}
	get := func(in SmartFormFilesInput) SmartFormFiles {
		t.Helper()
		in.AppID = app.AppID
		out, err := c.GetSmartFormFiles(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	entry := func(out SmartFormFiles, path string) SmartFormEntry {
		t.Helper()
		for _, e := range out.Files {
			if e.Path == path {
				return e
			}
		}
		t.Fatal("no entry ", path)
		return SmartFormEntry{}
	}
	listing := get(SmartFormFilesInput{})
	styles, vm := entry(listing, "styles/index"), entry(listing, "pages/index/viewModel")
	if listing.Note != "" || styles.Source != nil || len(styles.SourceHash) != 64 {
		t.Fatalf("listing: %+v", listing)
	}
	less := strings.Repeat("a { color: réd }\n", 6000) // 108,000 bytes, not JSON
	view := `{"orders":[{"id":1},{"id":2}],"user":{"name":"Ann"}}`
	if _, err = c.UpdateSmartFormFiles(ctx, SmartFormFilesUpdateInput{AppID: app.AppID, Items: []SmartFormFileUpdate{
		{ID: vm.ID, Title: "viewModel", Type: "application/json", Source: &view, IfMatchHash: vm.SourceHash}}}); err != nil {
		t.Fatal(err)
	}
	f.EditFile(app.AppID, "styles/index", func(string) string { return less }) // longer than a tool may write

	// A long file is read in parts that join into the file, each within the budget.
	var joined strings.Builder
	offset, parts := 0, 0
	for {
		e := entry(get(SmartFormFilesInput{Paths: []string{"styles/index"}, Offset: offset}), "styles/index")
		joined.WriteString(*e.Source)
		if parts++; encodedLen(*e.Source) > maxReadBytes || parts > 9 {
			t.Fatal("a part over the budget, or no end")
		}
		if offset = e.Next; offset == 0 {
			break
		}
	}
	if joined.String() != less || parts != 3 {
		t.Fatalf("%d parts, %d bytes", parts, joined.Len())
	}
	if _, err = c.GetSmartFormFiles(ctx, SmartFormFilesInput{AppID: app.AppID, Paths: []string{"styles/index"}, Offset: len(less)}); err == nil || !strings.Contains(err.Error(), "past the end") {
		t.Fatal(err)
	}
	// After a file that fills the answer, the others come without source and the note says so.
	all := get(SmartFormFilesInput{WithSource: true})
	if entry(all, "pages/index/config").Source == nil || entry(all, "styles/index").Source != nil || !strings.Contains(all.Note, "1 files left without source") {
		t.Fatalf("note %q", all.Note)
	}
	folder := get(SmartFormFilesInput{Paths: []string{"pages/index", "nope"}})
	if len(folder.Files) != 4 || entry(folder, "pages/index/viewModel").Source == nil || len(folder.Missing) != 1 || folder.Note != "" {
		t.Fatalf("%+v", folder)
	}
	// An outline of a folder covers its pages only; a file named itself is outlined by keys, and a
	// file that is not JSON answers its source.
	outlined := get(SmartFormFilesInput{Paths: []string{"pages"}, Outline: 2})
	if entry(outlined, "pages/index/config").Outline != "1 nodes\nStack#root" || entry(outlined, "pages/index/viewModel").Outline != "" {
		t.Fatalf("%+v", outlined.Files)
	}
	if e := entry(get(SmartFormFilesInput{Paths: []string{"pages/index/viewModel"}, Outline: 2}), "pages/index/viewModel"); e.Outline != "/ {2}\n orders [2]\n user {1}" {
		t.Fatal(e.Outline)
	}
	if e := entry(get(SmartFormFilesInput{Paths: []string{"pages/index/viewModel"}, NodeID: "/orders/1"}), "pages/index/viewModel"); string(e.Node) != `{"id":2}` {
		t.Fatal(string(e.Node))
	}
	if e := entry(get(SmartFormFilesInput{Paths: []string{"styles/index"}, Outline: 2}), "styles/index"); e.Source == nil || e.Next == 0 {
		t.Fatal("a file that is not JSON has no outline: its source")
	}
	for _, tc := range []struct {
		in   SmartFormFilesInput
		want string
	}{
		{SmartFormFilesInput{Outline: 21}, "outline 1..20"},
		{SmartFormFilesInput{Offset: -1}, "outline 1..20"},
		{SmartFormFilesInput{NodeID: "root"}, "one file"},
		{SmartFormFilesInput{Paths: []string{"nope"}, Offset: 5}, "one file"},
		{SmartFormFilesInput{Paths: []string{"styles/index"}, NodeID: "root"}, "not JSON"},
		{SmartFormFilesInput{Paths: []string{"pages/index/config"}, NodeID: "ghost"}, `no node with id "ghost"`},
		{SmartFormFilesInput{Paths: []string{"pages/index/config"}, NodeID: "/tree/nope"}, `no key "nope"`},
	} {
		tc.in.AppID = app.AppID
		if _, err = c.GetSmartFormFiles(ctx, tc.in); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: %v", tc.in, err)
		}
	}
	// More files than one answer reads: the rest are named by the note, not refused.
	var items []SmartFormFileCreate
	for range maxPathSources {
		items = append(items, SmartFormFileCreate{FolderID: entry(listing, "styles").ID, Title: "extra", Type: "text/less", Source: "x"})
	}
	if _, err = c.CreateSmartFormFiles(ctx, SmartFormFilesCreateInput{AppID: app.AppID, Items: items}); err != nil {
		t.Fatal(err)
	}
	f.EditFile(app.AppID, "styles/index", func(string) string { return "" })
	if many := get(SmartFormFilesInput{Paths: []string{"styles"}}); !strings.Contains(many.Note, "1 files left without source") {
		t.Fatal(many.Note)
	}
}
