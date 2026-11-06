package publicapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

// A JSON document read for a patch: objects keep their keys in source order (so a patched file
// differs from what was read only where the patch changed it), arrays are []any, numbers stay
// json.Number (an id above 2^53 is written back as it came), the rest is string, bool or nil.

// object is a JSON object in source key order.
type object struct {
	keys []string
	vals map[string]any
}

func newObject() *object { return &object{vals: map[string]any{}} }

func (o *object) get(k string) (any, bool) {
	v, ok := o.vals[k]
	return v, ok
}

// set replaces a key in place or appends a new one.
func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) del(k string) {
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

// decodeDoc parses exactly one JSON value.
func decodeDoc(src []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err = dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("data after the JSON value")
	}
	return v, nil
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		o := newObject()
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			o.set(k.(string), v) // a repeated key keeps its first place and the last value, like encoding/json
		}
		_, err = dec.Token()
		return o, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err = dec.Token()
		return arr, err
	}
	return tok, nil
}

// encodeDoc writes v compactly, or indented by indent when it is not empty; strings are not
// HTML-escaped, so an expression such as {{ a < b }} is written as it was read.
func encodeDoc(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeValue(&buf, v); err != nil {
		return nil, err
	}
	if indent == "" {
		return buf.Bytes(), nil
	}
	var out bytes.Buffer
	err := json.Indent(&out, buf.Bytes(), "", indent)
	return out.Bytes(), err
}

func writeValue(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case *object:
		buf.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k)
			buf.WriteByte(':')
			if err := writeValue(buf, t.vals[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeString(buf, t)
	case json.Number:
		buf.WriteString(t.String())
	case bool:
		buf.WriteString(strconv.FormatBool(t))
	case nil:
		buf.WriteString("null")
	default:
		return errors.New("unsupported JSON value")
	}
	return nil
}

func writeString(buf *bytes.Buffer, s string) {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	buf.Truncate(buf.Len() - 1)
}

// docIndent is the indent unit of a JSON source: the leading blanks of its first indented line,
// "" for a single-line source.
func docIndent(src string) string {
	for _, line := range strings.Split(src, "\n")[1:] {
		if blank := len(line) - len(strings.TrimLeft(line, " \t")); blank > 0 {
			return line[:blank]
		}
	}
	return ""
}

// cloneDoc deep-copies a document value.
func cloneDoc(v any) any {
	switch t := v.(type) {
	case *object:
		o := newObject()
		for _, k := range t.keys {
			o.set(k, cloneDoc(t.vals[k]))
		}
		return o
	case []any:
		arr := make([]any, len(t))
		for i, item := range t {
			arr[i] = cloneDoc(item)
		}
		return arr
	}
	return v
}

// equalDoc compares two document values as JSON: key order does not matter, numbers by value.
func equalDoc(a, b any) bool {
	switch x := a.(type) {
	case *object:
		y, ok := b.(*object)
		if !ok || len(x.keys) != len(y.keys) {
			return false
		}
		for _, k := range x.keys {
			yv, ok := y.vals[k]
			if !ok || !equalDoc(x.vals[k], yv) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equalDoc(x[i], y[i]) {
				return false
			}
		}
		return true
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		xf, e1 := x.Float64()
		yf, e2 := y.Float64()
		if e1 != nil || e2 != nil {
			return x == y
		}
		return xf == yf
	}
	return a == b
}
