// Package cduschema renders the JSON Schemas of the CDU v2 contracts (the component catalog and the
// page config) as compact TypeScript-like text: one line per key, $refs resolved, enums inlined.
// smart_form_components answers with it, the CDU reference documents are written with it, and
// cmd/gen-cdu-reference renders the page protocol with it.
package cduschema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Expression is the pattern of the string branch that lets any prop be a {{ }} expression.
const Expression = `\{\{.*\}\}`

const (
	maxDescription  = 140 // runes of a description: its first sentence, cut here
	maxNestedKeys   = 40  // keys of an object nested in a prop; documented ones first
	maxEnumInline   = 30  // values listed before the rest is counted
	namedEnumValues = 6   // a longer enum below the top level is named and listed once in Named
)

// Text renders the schema nodes of one document; defs are its $defs.
type Text struct {
	// Full describes a key with its whole description on one line instead of its first sentence.
	Full     bool
	defs     map[string]any
	named    map[string]string
	keep     map[string]bool
	visiting map[string]bool
}

// New renders against defs; a $ref to one of keep is written as its name, never expanded (a chain of
// actions reads ClientAction[], not seventeen object types joined by |).
func New(defs map[string]any, keep ...string) *Text {
	r := &Text{defs: defs, named: map[string]string{}, keep: map[string]bool{}, visiting: map[string]bool{}}
	for _, k := range keep {
		r.keep[k] = true
	}
	return r
}

// Named is the long enums met below the top level, by def name: each is written once here and by
// name where it is used.
func (r *Text) Named() map[string]string { return r.named }

// KeyLines renders an object schema one key per line; optional marks the keys it does not require.
func (r *Text) KeyLines(schema map[string]any, optional bool) []string {
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		if len(schema) == 0 || len(schema) == 1 && schema["$defs"] != nil {
			return nil
		}
		return []string{r.Type(schema) + r.describe(schema)}
	}
	required := requiredSet(schema)
	lines := []string{}
	hidden := 0
	for _, k := range SortedKeys(props) {
		if isAttribute(k) {
			hidden++
			continue
		}
		mark := ""
		if optional && !required[k] {
			mark = "?"
		}
		lines = append(lines, k+mark+": "+r.typeExpr(props[k], 0)+r.describe(props[k]))
	}
	if hidden > 0 {
		lines = append(lines, fmt.Sprintf("(+%d HTML/aria-*/on* attributes)", hidden))
	}
	return lines
}

// Type renders a schema as a type.
func (r *Text) Type(node any) string { return r.typeExpr(node, 0) }

// Object renders an object schema on one line: {key?: type, …}; deeper objects are "object".
func (r *Text) Object(m map[string]any) string { return r.objectExpr(m, 0) }

func (r *Text) describe(node any) string {
	if r.Full {
		return DescribeFull(node)
	}
	return Describe(node)
}

// typeExpr renders a schema as a type; depth counts the objects it is nested in.
func (r *Text) typeExpr(node any, depth int) string {
	m, ok := node.(map[string]any)
	if !ok {
		return "any"
	}
	if ref, ok := m["$ref"].(string); ok {
		return r.refExpr(strings.TrimPrefix(ref, "#/$defs/"), depth)
	}
	if values, ok := m["enum"].([]any); ok {
		return enumExpr(values)
	}
	if c, ok := m["const"]; ok {
		return literal(c)
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if branches, ok := m[key].([]any); ok {
			return r.unionExpr(branches, depth)
		}
	}
	if all, ok := m["allOf"].([]any); ok && len(all) > 0 {
		parts := make([]string, len(all))
		for i, b := range all {
			parts[i] = r.typeExpr(b, depth)
		}
		return strings.Join(parts, " & ")
	}
	switch t := m["type"].(type) {
	case string:
		return r.typedExpr(t, m, depth)
	case []any:
		parts := make([]string, 0, len(t))
		for _, one := range t {
			s, _ := one.(string)
			parts = append(parts, r.typedExpr(s, m, depth))
		}
		return strings.Join(dedupe(parts), " | ")
	}
	if m["properties"] != nil {
		return r.typedExpr("object", m, depth)
	}
	return "any"
}

func (r *Text) typedExpr(t string, m map[string]any, depth int) string {
	switch t {
	case "string":
		if m["pattern"] == Expression {
			return "{{expr}}"
		}
		return "string"
	case "array":
		item := r.typeExpr(m["items"], depth)
		if m["items"] == nil {
			item = "any"
		}
		if strings.Contains(item, " | ") || strings.Contains(item, " & ") {
			item = "(" + item + ")"
		}
		return item + "[]"
	case "object":
		if depth > 0 {
			return "object"
		}
		return r.objectExpr(m, depth)
	case "":
		return "any"
	}
	return t
}

// refExpr resolves a $ref; a long enum below the top level is named and listed once in named.
func (r *Text) refExpr(name string, depth int) string {
	def, ok := r.defs[name].(map[string]any)
	if !ok || r.visiting[name] || r.keep[name] {
		return name
	}
	if values, ok := def["enum"].([]any); ok && depth > 0 && len(values) > namedEnumValues {
		r.named[name] = enumExpr(values)
		return name
	}
	r.visiting[name] = true
	defer delete(r.visiting, name)
	return r.typeExpr(def, depth)
}

func (r *Text) unionExpr(branches []any, depth int) string {
	parts := make([]string, 0, len(branches))
	for _, b := range branches {
		parts = append(parts, r.typeExpr(b, depth))
	}
	return strings.Join(dedupe(parts), " | ")
}

// objectExpr renders an object one level deep: {key?: type, …}; deeper objects are "object". Of a
// large object (a layout primitive's inherited HTML attributes) only the documented keys are shown.
func (r *Text) objectExpr(m map[string]any, depth int) string {
	props, _ := m["properties"].(map[string]any)
	if len(props) == 0 {
		if extra, ok := m["additionalProperties"].(map[string]any); ok {
			return "{[key: string]: " + r.typeExpr(extra, depth+1) + "}"
		}
		return "{}"
	}
	required := requiredSet(m)
	var keys, undocumented []string
	for _, k := range SortedKeys(props) {
		switch {
		case isAttribute(k):
		case Description(props[k]) != "":
			keys = append(keys, k)
		default:
			undocumented = append(undocumented, k)
		}
	}
	if len(keys) == 0 || len(keys)+len(undocumented) <= maxNestedKeys {
		keys = append(keys, undocumented...) // a large object with documented keys shows only those
	}
	more := len(props) - len(keys)
	if len(keys) > maxNestedKeys {
		more += len(keys) - maxNestedKeys
		keys = keys[:maxNestedKeys]
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	for _, k := range keys {
		mark := "?"
		if required[k] {
			mark = ""
		}
		parts = append(parts, k+mark+": "+r.typeExpr(props[k], depth+1))
	}
	if more > 0 {
		parts = append(parts, fmt.Sprintf("… +%d more", more))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// Describe is " — " and the first sentence of a schema's description, or of its first described
// union branch (a prop's description sits in its non-expression branch).
func Describe(node any) string {
	d := Description(node)
	if d == "" {
		return ""
	}
	if i := strings.IndexAny(d, "\n"); i >= 0 {
		d = d[:i]
	}
	if i := strings.Index(d, ". "); i >= 0 {
		d = d[:i+1]
	}
	if utf8.RuneCountInString(d) > maxDescription {
		d = string([]rune(d)[:maxDescription-1]) + "…"
	}
	return " — " + strings.TrimSpace(d)
}

// DescribeFull is " — " and the whole description on one line.
func DescribeFull(node any) string {
	if d := OneLine(Description(node)); d != "" {
		return " — " + d
	}
	return ""
}

// OneLine joins a description's lines and paragraphs with single spaces.
func OneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Description is a schema's description, or its first described union branch's.
func Description(node any) string {
	m, _ := node.(map[string]any)
	if d, _ := m["description"].(string); d != "" {
		return d
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		branches, _ := m[key].([]any)
		for _, b := range branches {
			if bm, _ := b.(map[string]any); bm != nil {
				if d, _ := bm["description"].(string); d != "" {
					return d
				}
			}
		}
	}
	return ""
}

// isAttribute is a DOM attribute a layout primitive inherits from React's types, not a CDU prop
// authors set: aria-*, data-*, on* handlers and React internals.
func isAttribute(k string) bool {
	if strings.HasPrefix(k, "aria-") || strings.HasPrefix(k, "data-") || strings.HasPrefix(k, "suppress") || k == "dangerouslySetInnerHTML" {
		return true
	}
	return len(k) > 2 && strings.HasPrefix(k, "on") && unicode.IsUpper(rune(k[2]))
}

func enumExpr(values []any) string {
	parts := make([]string, 0, len(values))
	for i, v := range values {
		if i == maxEnumInline-5 && len(values) > maxEnumInline {
			parts = append(parts, fmt.Sprintf("…(+%d)", len(values)-i))
			break
		}
		parts = append(parts, literal(v))
	}
	return strings.Join(parts, "|")
}

func literal(v any) string {
	switch t := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(t, "'", `\'`) + "'"
	case nil:
		return "null"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func requiredSet(m map[string]any) map[string]bool {
	out := map[string]bool{}
	list, _ := m["required"].([]any)
	for _, k := range list {
		if s, ok := k.(string); ok {
			out[s] = true
		}
	}
	return out
}

// SortedKeys is a map's keys in order.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func dedupe(parts []string) []string {
	seen := map[string]bool{}
	out := parts[:0]
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
