package publicapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PatchOp is one RFC 6902 operation: add, replace, remove, move, copy or test. path and from are
// RFC 6901 JSON Pointers ("/tree/children/0/props/size"; "~1" is "/", "~0" is "~").
type PatchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	From  string          `json:"from,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// applyPatch applies ops to doc in order and returns the new document; the first failing op stops
// it with an error naming that op's index and path. doc is changed in place: on an error the
// caller discards it, so a patch applies all or nothing.
func applyPatch(doc any, ops []PatchOp) (any, error) {
	root := &doc
	for i, op := range ops {
		if err := applyOp(root, op); err != nil {
			return nil, fmt.Errorf("op %d (%s %s): %w", i, op.Op, op.Path, err)
		}
	}
	return *root, nil
}

func applyOp(root *any, op PatchOp) error {
	path, err := parsePointer(op.Path)
	if err != nil {
		return err
	}
	switch op.Op {
	case "add", "replace", "test":
		if len(op.Value) == 0 {
			return errors.New("value required")
		}
		v, err := decodeDoc(op.Value)
		if err != nil {
			return fmt.Errorf("value: %w", err)
		}
		switch op.Op {
		case "add":
			return addAt(root, path, v)
		case "replace":
			if _, err = getAt(*root, path); err != nil {
				return err
			}
			return setAt(root, path, v)
		}
		cur, err := getAt(*root, path)
		if err != nil {
			return err
		}
		if !equalDoc(cur, v) {
			return errors.New("test failed: the value there differs")
		}
		return nil
	case "remove":
		return removeAt(root, path)
	case "move", "copy":
		from, err := parsePointer(op.From)
		if err != nil {
			return fmt.Errorf("from: %w", err)
		}
		v, err := getAt(*root, from)
		if err != nil {
			return fmt.Errorf("from %s: %w", op.From, err)
		}
		if op.Op == "copy" {
			return addAt(root, path, cloneDoc(v))
		}
		if op.From == op.Path {
			return nil
		}
		if strings.HasPrefix(op.Path, op.From+"/") {
			return errors.New("cannot move a value into itself")
		}
		if err = removeAt(root, from); err != nil {
			return err
		}
		return addAt(root, path, v)
	}
	return fmt.Errorf("unknown op %q: use add, replace, remove, move, copy or test", op.Op)
}

// parsePointer splits a JSON Pointer into its unescaped reference tokens; "" is the whole document.
func parsePointer(p string) ([]string, error) {
	if p == "" {
		return nil, nil
	}
	if p[0] != '/' {
		return nil, fmt.Errorf("pointer %q must start with /", p)
	}
	tokens := strings.Split(p[1:], "/")
	for i, t := range tokens {
		for j := 0; j < len(t); j++ {
			if t[j] == '~' && (j+1 == len(t) || t[j+1] != '0' && t[j+1] != '1') {
				return nil, fmt.Errorf("pointer %q: ~ must be ~0 or ~1", p)
			}
		}
		tokens[i] = strings.ReplaceAll(strings.ReplaceAll(t, "~1", "/"), "~0", "~")
	}
	return tokens, nil
}

// pointerOf joins reference tokens into a JSON Pointer.
func pointerOf(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(t, "~", "~0"), "/", "~1"))
	}
	return b.String()
}

// getAt returns the value a pointer names.
func getAt(doc any, path []string) (any, error) {
	cur := doc
	for i, t := range path {
		switch c := cur.(type) {
		case *object:
			v, ok := c.get(t)
			if !ok {
				return nil, fmt.Errorf("%s has no key %q", where(path[:i]), t)
			}
			cur = v
		case []any:
			n, err := arrayIndex(t, len(c), false)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", where(path[:i]), err)
			}
			cur = c[n]
		default:
			return nil, fmt.Errorf("%s is not an object or array", where(path[:i]))
		}
	}
	return cur, nil
}

// setAt stores v at an existing location (or a new key of an existing object).
func setAt(root *any, path []string, v any) error {
	if len(path) == 0 {
		*root = v
		return nil
	}
	parent, err := getAt(*root, path[:len(path)-1])
	if err != nil {
		return err
	}
	last := path[len(path)-1]
	switch c := parent.(type) {
	case *object:
		c.set(last, v)
	case []any:
		n, err := arrayIndex(last, len(c), false)
		if err != nil {
			return fmt.Errorf("%s: %w", where(path[:len(path)-1]), err)
		}
		c[n] = v
	default:
		return fmt.Errorf("%s is not an object or array", where(path[:len(path)-1]))
	}
	return nil
}

// addAt adds a key to an object, or inserts into an array at an index or at its end ("-").
func addAt(root *any, path []string, v any) error {
	if len(path) == 0 {
		*root = v
		return nil
	}
	parentPath, last := path[:len(path)-1], path[len(path)-1]
	parent, err := getAt(*root, parentPath)
	if err != nil {
		return err
	}
	arr, ok := parent.([]any)
	if !ok {
		return setAt(root, path, v)
	}
	n := len(arr)
	if last != "-" {
		if n, err = arrayIndex(last, len(arr), true); err != nil {
			return fmt.Errorf("%s: %w", where(parentPath), err)
		}
	}
	grown := make([]any, 0, len(arr)+1)
	grown = append(append(append(grown, arr[:n]...), v), arr[n:]...)
	return setAt(root, parentPath, grown)
}

// removeAt deletes an object key or an array item.
func removeAt(root *any, path []string) error {
	if len(path) == 0 {
		return errors.New("cannot remove the whole document")
	}
	if _, err := getAt(*root, path); err != nil {
		return err
	}
	parentPath, last := path[:len(path)-1], path[len(path)-1]
	parent, _ := getAt(*root, parentPath)
	if o, ok := parent.(*object); ok {
		o.del(last)
		return nil
	}
	arr := parent.([]any)
	n, _ := arrayIndex(last, len(arr), false)
	shrunk := append(append(make([]any, 0, len(arr)-1), arr[:n]...), arr[n+1:]...)
	return setAt(root, parentPath, shrunk)
}

// arrayIndex parses an array reference token (digits without a leading zero) into an index of an
// array of size items; end also allows size itself, the insert position after the last item.
func arrayIndex(t string, size int, end bool) (int, error) {
	n, err := strconv.Atoi(t)
	if err != nil || n < 0 || strconv.Itoa(n) != t {
		return 0, fmt.Errorf("%q is not an array index", t)
	}
	if n > size || n == size && !end {
		return 0, fmt.Errorf("index %d is out of range (the array has %d items)", n, size)
	}
	return n, nil
}

func where(path []string) string {
	if len(path) == 0 {
		return "the document"
	}
	return pointerOf(path)
}
