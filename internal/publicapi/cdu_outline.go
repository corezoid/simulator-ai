package publicapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// An outline is a file read as one line per node instead of its source: a page of 60,000 characters
// is a few hundred short lines, and the ids it names are what smart_form_page_patch and a nodeId
// read address.

// outlineProps are the props a line shows, in this order, at most outlinePropCount of them.
var outlineProps = []string{"label", "text", "title", "name", "valueKey", "placeholder", "value", "src", "href", "variant"}

const (
	outlinePropCount = 2
	outlineValueLen  = 32
)

type outliner struct {
	b      strings.Builder
	levels int // lines go this deep
	limit  int // bytes
	cut    bool
}

// isNode says v is a CDU tree node: an object with a type or a $ref.
func isNode(v any) (*object, bool) {
	o, ok := v.(*object)
	if !ok {
		return nil, false
	}
	_, typed := o.vals["type"].(string)
	_, ref := o.vals["$ref"].(string)
	return o, typed || ref
}

// countNodes is how many nodes v (a node or a list of nodes) holds, overlay contents included.
func countNodes(v any) int {
	if list, ok := v.([]any); ok {
		n := 0
		for _, item := range list {
			n += countNodes(item)
		}
		return n
	}
	node, ok := v.(*object)
	if !ok {
		return 0
	}
	n := 1
	for _, kid := range nodeKids(node, nil) {
		n += countNodes(kid.v)
	}
	return n
}

func (o *outliner) line(level int, text string) {
	if o.cut {
		return
	}
	if o.b.Len()+level+len(text) > o.limit {
		o.cut = true
		return
	}
	o.b.WriteString(strings.Repeat(" ", level))
	o.b.WriteString(text)
	o.b.WriteByte('\n')
}

// nodes writes a node, or each node of a list, with what is under it.
func (o *outliner) nodes(v any, path []string, level int, label string) {
	if list, ok := v.([]any); ok {
		for i, item := range list {
			o.nodes(item, at(path, strconv.Itoa(i)), level, label)
		}
		return
	}
	node, ok := v.(*object)
	if !ok || o.cut {
		return
	}
	kids := nodeKids(node, path)
	text := nodeLine(node, path, label)
	if level+1 >= o.levels {
		hidden := 0
		for _, kid := range kids {
			hidden += countNodes(kid.v)
		}
		if hidden > 0 {
			text += " +" + strconv.Itoa(hidden)
		}
		o.line(level, text)
		return
	}
	o.line(level, text)
	for _, kid := range kids {
		o.nodes(kid.v, kid.path, level+1, kid.label)
	}
}

// nodeLine is "[slot] Type#id prop=value on:event": a node without an id is named by its pointer.
func nodeLine(node *object, path []string, label string) string {
	var b strings.Builder
	if label != "" {
		b.WriteString("[" + label + "] ")
	}
	if ref, ok := node.vals["$ref"].(string); ok {
		b.WriteString("$" + strings.TrimPrefix(ref, "#/"))
	} else if kind, ok := node.vals["type"].(string); ok {
		b.WriteString(kind)
	} else {
		b.WriteString("?")
	}
	if id, ok := node.vals["id"].(string); ok {
		b.WriteString("#" + id)
	} else {
		b.WriteString("@" + pointerOf(path))
	}
	if props, ok := node.vals["props"].(*object); ok {
		shown := 0
		for _, k := range outlineProps {
			if v, ok := props.vals[k]; ok && shown < outlinePropCount {
				if s := scalar(v); s != "" {
					b.WriteString(" " + k + "=" + s)
					shown++
				}
			}
		}
	}
	for _, k := range []string{"for", "if"} {
		if _, ok := node.vals[k]; ok {
			b.WriteString(" " + k)
		}
	}
	if actions, ok := node.vals["actions"].(*object); ok && len(actions.keys) > 0 {
		b.WriteString(" on:" + strings.Join(actions.keys, ","))
	}
	return b.String()
}

// scalar is a short text of a string, number or boolean; "" for anything else.
func scalar(v any) string {
	switch t := v.(type) {
	case string:
		if utf8.RuneCountInString(t) > outlineValueLen {
			t = string([]rune(t)[:outlineValueLen]) + "…"
		}
		return strconv.Quote(t)
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

// keys writes a JSON value that is not a node tree (a viewModel, a locale): one line per key with
// what it holds, so a part is then read by its pointer.
func (o *outliner) keys(v any, name string, level int) {
	switch t := v.(type) {
	case *object:
		o.line(level, fmt.Sprintf("%s {%d}", name, len(t.keys)))
		if level+1 < o.levels {
			for _, k := range t.keys {
				o.keys(t.vals[k], k, level+1)
			}
		}
	case []any:
		o.line(level, fmt.Sprintf("%s [%d]", name, len(t)))
	default:
		s := scalar(v)
		if s == "" {
			s = "null"
		}
		o.line(level, name+" "+s)
	}
}

// outlineDoc outlines a parsed file, or the part of it at path, levels deep in at most limit bytes.
// A page config is its tree and the overlays its bindings open; a definition or a node is that
// node; anything else is outlined by keys.
func outlineDoc(doc any, path []string, levels, limit int) string {
	o := &outliner{levels: levels, limit: limit}
	page, _ := doc.(*object)
	tree, isPage := any(nil), false
	if page != nil && len(path) == 0 {
		tree, isPage = page.get("tree")
	}
	_, node := isNode(doc)
	// A list is a node list (a children array) when one of its items is a node; rows of a viewModel
	// are plain objects and are outlined by keys.
	list := false
	if items, ok := doc.([]any); ok {
		for _, item := range items {
			if _, ok := isNode(item); ok {
				list = true
				break
			}
		}
	}
	switch {
	case isPage:
		total := countNodes(tree)
		var bound []nodeKid
		if bindings, ok := page.get("bindings"); ok {
			bound = overlays(nil, bindings, []string{"bindings"})
		}
		for _, kid := range bound {
			total += countNodes(kid.v)
		}
		o.line(0, strconv.Itoa(total)+" nodes")
		o.nodes(tree, []string{"tree"}, 0, "")
		for _, kid := range bound {
			o.nodes(kid.v, kid.path, 0, "binding overlay")
		}
	case node || list:
		o.nodes(doc, path, 0, "")
	default:
		name := "/"
		if len(path) > 0 {
			name = path[len(path)-1]
		}
		o.keys(doc, name, 0)
	}
	if o.cut {
		o.b.WriteString("… cut: ask fewer levels, or outline one node (nodeId)\n")
	}
	return strings.TrimSuffix(o.b.String(), "\n")
}
