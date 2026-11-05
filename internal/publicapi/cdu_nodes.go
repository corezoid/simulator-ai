package publicapi

import (
	"errors"
	"fmt"
	"strings"
)

// findNode returns the JSON Pointer of the one CDU node with this id in a page config. Like
// sim-api's save-time validator it walks the tree's children lists and named slots, and the
// content of every overlay.open action, in node actions and in the page's bindings.
func findNode(doc any, id string) (string, error) {
	page, ok := doc.(*object)
	tree, has := any(nil), false
	if ok {
		tree, has = page.get("tree")
	}
	if !has {
		return "", errors.New("nodeId needs a page config: the file has no tree")
	}
	f := &nodeFinder{id: id}
	f.node(tree, []string{"tree"})
	if bindings, ok := page.get("bindings"); ok {
		f.actions(bindings, []string{"bindings"})
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
	return "", fmt.Errorf("no node with id %q in the page; node ids: %s", id, strings.Join(ids, ", "))
}

type nodeFinder struct {
	id         string
	found, ids []string
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
	if v, ok := node.get("id"); ok {
		if s, ok := v.(string); ok {
			f.ids = append(f.ids, s)
			if s == f.id {
				f.found = append(f.found, pointerOf(path))
			}
		}
	}
	switch c := node.vals["children"].(type) {
	case []any:
		f.node(c, at(path, "children"))
	case *object:
		for _, slot := range c.keys {
			f.node(c.vals[slot], at(path, "children", slot))
		}
	}
	if actions, ok := node.vals["actions"].(*object); ok {
		for _, event := range actions.keys {
			f.actions(actions.vals[event], at(path, "actions", event))
		}
	}
}

// actions follows an action chain (and a binding's do) into overlay.open content.
func (f *nodeFinder) actions(v any, path []string) {
	switch t := v.(type) {
	case []any:
		for i, item := range t {
			f.actions(item, at(path, fmt.Sprint(i)))
		}
	case *object:
		if t.vals["type"] == "overlay.open" {
			f.node(t.vals["content"], at(path, "content"))
		}
		for _, key := range []string{"do", "then", "else", "onSuccess", "onError"} {
			if next, ok := t.vals[key]; ok {
				f.actions(next, at(path, key))
			}
		}
	}
}
