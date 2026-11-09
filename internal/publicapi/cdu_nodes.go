package publicapi

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// findNode returns the JSON Pointer of the one CDU node with this id in a page config or in a
// definition (definitions/<name>, whose root is a node). Like sim-api's save-time validator it
// walks the tree's children lists and named slots, and the content of every overlay.open action,
// in node actions and in the page's bindings. It does not open definitions: a $ref instance is
// found by its own id, and the nodes of its definition are in that definition's file.
func findNode(doc any, id string) (string, error) {
	f, ok := walkNodes(doc, id)
	if !ok {
		return "", errors.New("nodeId needs a page config (a file with a tree) or a definition (a file that is one node)")
	}
	switch {
	case len(f.found) == 1:
		return f.found[0], nil
	case len(f.found) > 1:
		return "", fmt.Errorf("nodeId %q names %d nodes (%s): patch by absolute path", id, len(f.found), strings.Join(f.found, ", "))
	}
	ids := f.ids
	if len(ids) > 40 {
		ids = append(ids[:40], "…")
	}
	msg := fmt.Sprintf("no node with id %q in the file; node ids: %s", id, strings.Join(ids, ", "))
	if len(f.refs) > 0 {
		msg += "; the nodes of a definition are in its own file: patch path definitions/<name> (this file uses " + strings.Join(f.refs, ", ") + ")"
	}
	return "", errors.New(msg)
}

// walkNodes walks a page config or a definition, collecting its node ids and the pointers of the
// nodes with this id; ok is false for any other document.
func walkNodes(doc any, id string) (*nodeFinder, bool) {
	f := &nodeFinder{id: id}
	page, _ := doc.(*object)
	tree, isPage := any(nil), false
	if page != nil {
		tree, isPage = page.get("tree")
	}
	switch {
	case isPage:
		f.node(tree, []string{"tree"})
		if bindings, ok := page.get("bindings"); ok {
			for _, kid := range overlays(nil, bindings, []string{"bindings"}) {
				f.node(kid.v, kid.path)
			}
		}
	case page != nil && (page.vals["type"] != nil || page.vals["$ref"] != nil):
		f.node(page, nil)
	default:
		return f, false
	}
	return f, true
}

type nodeFinder struct {
	id         string
	found, ids []string
	refs       []string // the definitions the walked nodes are instances of
}

// refContent is what a $ref instance cannot carry: sim-api refuses an instance with type, props,
// children or actions, which all come from the definition; it takes id, if, visibility, for, slot
// and variant.
var refContent = []string{"type", "props", "children", "actions"}

// instanceOp refuses an op that would give the $ref instance at base a key only its definition
// may hold, naming the definition file to patch instead.
func instanceOp(doc any, base string, op PatchOp, i int) error {
	tokens, err := parsePointer(base)
	if err != nil {
		return nil
	}
	n, _ := getAt(doc, tokens)
	node, _ := n.(*object)
	if node == nil {
		return nil
	}
	ref, _ := node.vals["$ref"].(string)
	if ref == "" || op.Path == "" {
		return nil
	}
	key := strings.SplitN(strings.TrimPrefix(op.Path, "/"), "/", 2)[0]
	for _, k := range refContent {
		if key == k {
			return fmt.Errorf("op %d: node %q is an instance of %s, which holds its %s: patch path %s instead (that changes every instance), or replace the instance (path \"\") with an inline node", i, base, ref, k, strings.TrimPrefix(ref, "#/"))
		}
	}
	return nil
}

// at is path extended by keys, as a new slice.
func at(path []string, keys ...string) []string {
	return append(append(make([]string, 0, len(path)+len(keys)), path...), keys...)
}

// node walks a node or a list of nodes.
func (f *nodeFinder) node(n any, path []string) {
	if list, ok := n.([]any); ok {
		for i, item := range list {
			f.node(item, at(path, fmt.Sprint(i)))
		}
		return
	}
	node, ok := n.(*object)
	if !ok {
		return
	}
	if ref, ok := node.vals["$ref"].(string); ok && !slices.Contains(f.refs, strings.TrimPrefix(ref, "#/")) {
		f.refs = append(f.refs, strings.TrimPrefix(ref, "#/"))
	}
	if v, ok := node.get("id"); ok {
		if s, ok := v.(string); ok {
			f.ids = append(f.ids, s)
			if s == f.id {
				f.found = append(f.found, pointerOf(path))
			}
		}
	}
	for _, kid := range nodeKids(node, path) {
		f.node(kid.v, kid.path)
	}
}

// nodeKid is what sits directly under a node: its children list, the list of a named slot, or the
// content an overlay.open of its actions shows.
type nodeKid struct {
	v     any
	path  []string
	label string // "" for children, the slot name, or "overlay"
}

func nodeKids(node *object, path []string) []nodeKid {
	var out []nodeKid
	switch c := node.vals["children"].(type) {
	case []any:
		out = append(out, nodeKid{c, at(path, "children"), ""})
	case *object:
		for _, slot := range c.keys {
			out = append(out, nodeKid{c.vals[slot], at(path, "children", slot), slot})
		}
	}
	if actions, ok := node.vals["actions"].(*object); ok {
		for _, event := range actions.keys {
			out = overlays(out, actions.vals[event], at(path, "actions", event))
		}
	}
	return out
}

// overlays adds the content of every overlay.open an action chain (or a binding's do) holds.
func overlays(out []nodeKid, v any, path []string) []nodeKid {
	switch t := v.(type) {
	case []any:
		for i, item := range t {
			out = overlays(out, item, at(path, fmt.Sprint(i)))
		}
	case *object:
		if t.vals["type"] == "overlay.open" {
			out = append(out, nodeKid{t.vals["content"], at(path, "content"), "overlay"})
		}
		for _, key := range []string{"do", "then", "else", "onSuccess", "onError"} {
			if next, ok := t.vals[key]; ok {
				out = overlays(out, next, at(path, key))
			}
		}
	}
	return out
}
