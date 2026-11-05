package mcpserver

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// The CDU v2 reference is a few documents the Console agent searches and reads by line, as Claude
// Code greps the format's sources: it has no file system, and a tool answer over 50 000 chars is
// spilled where it cannot read it. components is the whole component catalog in the compact lines
// of smart_form_components, built from the stand's own cdu/catalog/get (what the validator
// enforces, so it never drifts); protocol and examples are generated from sim-api's page-config
// contract and the ui-kit's help bundle and example pages by cmd/gen-cdu-reference.

var (
	//go:embed cdureference/protocol.md
	protocolText string
	//go:embed cdureference/examples.md
	examplesText string
	//go:embed cdureference/types.json
	typeNotesJSON []byte
)

// referenceDoc is one document: its lines, numbered from 1.
type referenceDoc struct {
	Name    string
	Summary string
	Lines   []string
}

func textDoc(name, summary, text string) referenceDoc {
	return referenceDoc{name, summary, strings.Split(strings.TrimRight(text, "\n"), "\n")}
}

var (
	protocolDoc = textDoc("protocol", "The page config, tree nodes, {{ }} expressions and their context ($locale, $event, vm, …), bindings, every client and response action with its fields, routes, definitions.", protocolText)
	examplesDoc = textDoc("examples", "Complete example pages split into their Smart Form files: a form with validation and submit, a multi-step form, a server-driven table, a Row/Col/Card/Tabs layout, a loop list, routes.", examplesText)
)

const componentsSummary = "Every component type of the catalog the stand validates pages against: purpose, children, then one line per prop, event, state key and api method, enums and prop examples."

// typeNote is what the generated types.json adds to a catalog type: cdu/catalog/get has neither its
// purpose (the editor's hint), nor where its children go, nor the editor's prop examples.
type typeNote struct {
	Purpose  string `json:"purpose"`
	Children string `json:"children"`
	Examples []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Props       json.RawMessage `json:"props"`
	} `json:"examples"`
}

var typeNotes = func() map[string]typeNote {
	var doc struct {
		Types map[string]typeNote `json:"types"`
	}
	if err := json.Unmarshal(typeNotesJSON, &doc); err != nil {
		panic(err) // generated, embedded file
	}
	return doc.Types
}()

// componentsDoc renders a full cdu/catalog/get answer one section per type, grouped by kind and
// palette category, so `^## ` lists the types and a prop line is found by its words.
func componentsDoc(answer json.RawMessage, notes map[string]typeNote) (referenceDoc, error) {
	var catalog struct {
		SchemaVersion string        `json:"schemaVersion"`
		List          []catalogType `json:"list"`
	}
	if err := json.Unmarshal(answer, &catalog); err != nil || len(catalog.List) == 0 {
		return referenceDoc{}, fmt.Errorf("cdu/catalog/get returned no component types")
	}
	kindRank := map[string]int{"primitive": 0, "block": 1}
	sort.SliceStable(catalog.List, func(i, j int) bool {
		a, b := catalog.List[i], catalog.List[j]
		if kindRank[a.Kind] != kindRank[b.Kind] {
			return kindRank[a.Kind] < kindRank[b.Kind]
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Type < b.Type
	})
	lines := []string{
		fmt.Sprintf("# CDU components (schemaVersion %s): %d types", catalog.SchemaVersion, len(catalog.List)),
		"From the stand's cdu/catalog/get: the catalog sim-api validates every saved page against. Use only these types and props.",
		"Each type is a section \"## <Type> (<kind>, <category>) — <label>\": what it is for, where its children go, then one line per prop, event (its $event payload), state key ({{ $block.<id>.state.<key> }}) and api method (widget.call), its named enums and prop examples. grep \"^## \" lists the types; read one with {doc: \"components/<Type>\"}.",
		"Notation: TypeScript-like: key?: optional, 'a'|'b' enum, {{expr}} a {{ }} expression string, A[] array, Name a value of the type's enums; HTML, aria-* and on* attributes are left out. smart_form_components {types, schema: true} returns the JSON Schemas.",
	}
	for _, t := range catalog.List {
		lines = append(lines, "")
		lines = append(lines, typeSection(compactOne(t), notes[t.Type])...)
	}
	return referenceDoc{"components", componentsSummary, lines}, nil
}

func typeSection(c compactType, note typeNote) []string {
	heading := fmt.Sprintf("## %s (%s, %s)", c.Type, c.Kind, c.Category)
	if c.Label != "" && c.Label != c.Type {
		heading += " — " + c.Label
	}
	lines := []string{heading}
	if note.Purpose != "" {
		lines = append(lines, note.Purpose)
	}
	if note.Children != "" {
		lines = append(lines, "children: "+note.Children)
	}
	for _, part := range []struct {
		name  string
		items []string
	}{{"props", c.Props}, {"events", c.Events}, {"state", c.State}, {"api", c.API}} {
		if len(part.items) == 0 {
			continue
		}
		lines = append(lines, part.name+":")
		for _, item := range part.items {
			lines = append(lines, "  "+item)
		}
	}
	if len(c.Enums) > 0 {
		lines = append(lines, "enums:")
		for _, name := range sortedNames(c.Enums) {
			lines = append(lines, "  "+name+": "+c.Enums[name])
		}
	}
	if len(note.Examples) > 0 {
		lines = append(lines, "examples (props):")
		for _, e := range note.Examples {
			line := "  " + e.Name
			if e.Description != "" {
				line += " — " + e.Description
			}
			props := &bytes.Buffer{}
			_ = json.Compact(props, e.Props) // one line: types.json is indented
			lines = append(lines, line+": "+props.String())
		}
	}
	return lines
}

func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// catalogTTL is how long the components document is reused: the catalog changes only with a
// sim-api deployment, and one Console run greps it many times.
const catalogTTL = 10 * time.Minute

// componentsCache builds the components document once per catalogTTL. Errors are not cached; a
// concurrent call waits for the fetch in progress instead of starting its own.
type componentsCache struct {
	fetch func(context.Context) (json.RawMessage, error)
	now   func() time.Time
	mu    sync.Mutex
	at    time.Time
	doc   *referenceDoc
}

func (c *componentsCache) get(ctx context.Context) (referenceDoc, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.doc != nil && c.now().Sub(c.at) < catalogTTL {
		return *c.doc, nil
	}
	answer, err := c.fetch(ctx)
	if err != nil {
		return referenceDoc{}, err
	}
	doc, err := componentsDoc(answer, typeNotes)
	if err != nil {
		return referenceDoc{}, err
	}
	c.doc, c.at = &doc, c.now()
	return doc, nil
}
