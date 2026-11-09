package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
)

// renderLocal is smart_form_page_render's plugin-side param.
var renderLocal = map[string]json.RawMessage{"full": json.RawMessage(`{"type":"boolean","description":"The whole render (tree, bindings, viewModel, locale: bytes long) instead of the summary"}`)}

// maxRenderKeys bounds the view model keys a render summary names.
const maxRenderKeys = 40

// renderSummary answers a render as what it proves: the page resolved, and how much it holds. The
// whole render of a large page is tens of thousands of characters the agent wrote itself.
type renderedPage struct {
	OK       bool     `json:"ok"`
	PageID   string   `json:"pageId"`
	Title    string   `json:"title,omitempty"`
	Nodes    int      `json:"nodes"`
	Bindings int      `json:"bindings"`
	VMKeys   []string `json:"vmKeys"`
	Locales  []string `json:"locales"`
	Bytes    int      `json:"bytes"`
	Warnings []string `json:"warnings,omitempty"`
}

func renderSummary(local map[string]json.RawMessage, v json.RawMessage) (json.RawMessage, error) {
	if string(local["full"]) == "true" {
		return v, nil
	}
	var r struct {
		PageID    string                     `json:"pageId"`
		Title     string                     `json:"title"`
		Tree      any                        `json:"tree"`
		Bindings  []json.RawMessage          `json:"bindings"`
		ViewModel map[string]json.RawMessage `json:"viewModel"`
		Locale    map[string]json.RawMessage `json:"locale"`
	}
	if err := json.Unmarshal(v, &r); err != nil {
		return v, nil // not the shape of a render: answer it as it came
	}
	out := renderedPage{OK: true, PageID: r.PageID, Title: r.Title, Bindings: len(r.Bindings), VMKeys: keysOf(r.ViewModel), Locales: keysOf(r.Locale), Bytes: len(v)}
	refs := 0
	out.Nodes = renderedNodes(r.Tree, &refs)
	if out.Nodes == 0 {
		out.Warnings = append(out.Warnings, "the tree has no nodes")
	}
	if refs > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d $ref nodes were not replaced by their definitions", refs))
	}
	if n := len(out.VMKeys); n > maxRenderKeys {
		out.VMKeys = append(out.VMKeys[:maxRenderKeys], fmt.Sprintf("… %d more", n-maxRenderKeys))
	}
	return json.Marshal(out)
}

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// renderedNodes counts the nodes of a rendered tree (children lists and named slots), and the
// $ref nodes the server left in it.
func renderedNodes(v any, refs *int) int {
	switch t := v.(type) {
	case []any:
		n := 0
		for _, item := range t {
			n += renderedNodes(item, refs)
		}
		return n
	case map[string]any:
		if _, ref := t["$ref"]; ref {
			*refs++
		}
		n := 1
		switch c := t["children"].(type) {
		case []any:
			n += renderedNodes(c, refs)
		case map[string]any:
			for _, slot := range c {
				n += renderedNodes(slot, refs)
			}
		}
		return n
	}
	return 0
}
