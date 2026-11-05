package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// smart_form_components answers a narrowed catalog (types or kind) compactly: each component's
// props, events, state and api one line each, in a TypeScript-like notation with $refs resolved
// and enums inlined, instead of the JSON Schemas with their whole $defs closures. Stack and Form
// alone were 60 KB of schema, which the Console agent never saw past a spilled preview; the
// line for Stack's gap is `size?: 'none'|…|'11xl' | {{expr}} — Gap size between items`. schema
// true returns the backend's JSON Schemas unchanged.

// catalogNotation explains the compact lines once per answer.
const catalogNotation = "TypeScript-like: key?: optional, 'a'|'b' enum, {{expr}} a {{ }} expression string, A[] array, Name a value of enums[Name]; HTML, aria-* and on* attributes are left out. schema:true returns the JSON Schemas."

// expression is the pattern of the string branch that lets any prop be a {{ }} expression.
const expression = `\{\{.*\}\}`

const (
	maxDescription  = 140 // runes of a description: its first sentence, cut here
	maxNestedKeys   = 40  // keys of an object nested in a prop; documented ones first
	maxEnumInline   = 30  // values listed before the rest is counted
	namedEnumValues = 6   // a longer enum below the top level is named and listed once in enums
)

type compactType struct {
	Type     string            `json:"type"`
	Kind     string            `json:"kind,omitempty"`
	Category string            `json:"category,omitempty"`
	Label    string            `json:"label,omitempty"`
	Props    []string          `json:"props,omitempty"`
	Events   []string          `json:"events,omitempty"`
	State    []string          `json:"state,omitempty"`
	API      []string          `json:"api,omitempty"`
	Enums    map[string]string `json:"enums,omitempty"`
}

type catalogType struct {
	Type     string         `json:"type"`
	Kind     string         `json:"kind,omitempty"`
	Category string         `json:"category,omitempty"`
	Label    string         `json:"label,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Events   map[string]any `json:"events,omitempty"`
	State    map[string]any `json:"state,omitempty"`
	API      map[string]any `json:"api,omitempty"`
}

// compactCatalogUnlessSchema is smart_form_components' Shape: compact unless the call sent schema true.
func compactCatalogUnlessSchema(local map[string]json.RawMessage, v json.RawMessage) (json.RawMessage, error) {
	if string(local["schema"]) == "true" {
		return v, nil
	}
	return compactCatalog(v)
}

// compactCatalog rewrites a cdu/catalog/get answer; a brief one (no schemas) is returned as it came.
func compactCatalog(v json.RawMessage) (json.RawMessage, error) {
	var doc struct {
		SchemaVersion string        `json:"schemaVersion"`
		List          []catalogType `json:"list"`
		Unknown       []string      `json:"unknown,omitempty"`
	}
	if json.Unmarshal(v, &doc) != nil {
		return v, nil
	}
	schemas := false
	out := make([]compactType, len(doc.List))
	for i, t := range doc.List {
		schemas = schemas || t.Props != nil || t.Events != nil || t.State != nil || t.API != nil
		out[i] = compactOne(t)
	}
	if !schemas {
		return v, nil
	}
	return json.Marshal(struct {
		SchemaVersion string        `json:"schemaVersion"`
		Notation      string        `json:"notation"`
		List          []compactType `json:"list"`
		Unknown       []string      `json:"unknown,omitempty"`
	}{doc.SchemaVersion, catalogNotation, out, doc.Unknown})
}

func compactOne(t catalogType) compactType {
	defs, _ := t.Props["$defs"].(map[string]any)
	r := &schemaText{defs: defs, named: map[string]string{}, visiting: map[string]bool{}}
	out := compactType{Type: t.Type, Kind: t.Kind, Category: t.Category, Label: t.Label}
	if t.Props != nil {
		out.Props = r.keyLines(t.Props, true)
	}
	for _, name := range sortedKeys(t.Events) {
		ev, _ := t.Events[name].(map[string]any)
		line := name
		if props, _ := ev["properties"].(map[string]any); len(props) > 0 {
			line += " $event " + r.objectExpr(ev, 0)
		}
		out.Events = append(out.Events, line+describe(ev))
	}
	if t.State != nil {
		out.State = r.keyLines(t.State, false)
	}
	for _, name := range sortedKeys(t.API) {
		m, _ := t.API[name].(map[string]any)
		args := ""
		if a, _ := m["args"].(map[string]any); a != nil {
			if props, _ := a["properties"].(map[string]any); len(props) > 0 {
				args = r.objectExpr(a, 0)
			}
		}
		out.API = append(out.API, name+"("+args+")"+describe(m))
	}
	if len(r.named) > 0 {
		out.Enums = r.named
	}
	return out
}

// schemaText renders JSON Schema nodes of one component; defs are its props' $defs.
type schemaText struct {
	defs     map[string]any
	named    map[string]string
	visiting map[string]bool
}

// keyLines renders an object schema one key per line; optional marks the keys it does not require.
func (r *schemaText) keyLines(schema map[string]any, optional bool) []string {
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		if len(schema) == 0 || len(schema) == 1 && schema["$defs"] != nil {
			return nil
		}
		return []string{r.typeExpr(schema, 0) + describe(schema)}
	}
	required := requiredSet(schema)
	lines := []string{}
	hidden := 0
	for _, k := range sortedKeys(props) {
		if isAttribute(k) {
			hidden++
			continue
		}
		mark := ""
		if optional && !required[k] {
			mark = "?"
		}
		lines = append(lines, k+mark+": "+r.typeExpr(props[k], 0)+describe(props[k]))
	}
	if hidden > 0 {
		lines = append(lines, fmt.Sprintf("(+%d HTML/aria-*/on* attributes)", hidden))
	}
	return lines
}

// typeExpr renders a schema as a type; depth counts the objects it is nested in.
func (r *schemaText) typeExpr(node any, depth int) string {
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

func (r *schemaText) typedExpr(t string, m map[string]any, depth int) string {
	switch t {
	case "string":
		if m["pattern"] == expression {
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

// refExpr resolves a $ref; a long enum below the top level is named and listed once in enums.
func (r *schemaText) refExpr(name string, depth int) string {
	def, ok := r.defs[name].(map[string]any)
	if !ok || r.visiting[name] {
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

func (r *schemaText) unionExpr(branches []any, depth int) string {
	parts := make([]string, 0, len(branches))
	for _, b := range branches {
		parts = append(parts, r.typeExpr(b, depth))
	}
	return strings.Join(dedupe(parts), " | ")
}

// objectExpr renders an object one level deep: {key?: type, …}; deeper objects are "object". Of a
// large object (a layout primitive's inherited HTML attributes) only the documented keys are shown.
func (r *schemaText) objectExpr(m map[string]any, depth int) string {
	props, _ := m["properties"].(map[string]any)
	if len(props) == 0 {
		if extra, ok := m["additionalProperties"].(map[string]any); ok {
			return "{[key: string]: " + r.typeExpr(extra, depth+1) + "}"
		}
		return "{}"
	}
	required := requiredSet(m)
	var keys, undocumented []string
	for _, k := range sortedKeys(props) {
		switch {
		case isAttribute(k):
		case hasDescription(props[k]):
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

// describe is " — " and the first sentence of a schema's description, or of its first described
// union branch (a prop's description sits in its non-expression branch).
func describe(node any) string {
	d := description(node)
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

func description(node any) string {
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

func hasDescription(node any) bool { return description(node) != "" }

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

func sortedKeys[V any](m map[string]V) []string {
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
