package cduschema

import (
	"strings"
	"testing"
)

// A kept $ref is written by name; Full keeps the whole description on one line.
func TestKeepAndFull(t *testing.T) {
	defs := map[string]any{
		"ClientAction": map[string]any{"oneOf": []any{map[string]any{"type": "object"}}},
		"Size":         map[string]any{"enum": []any{"a", "b"}},
	}
	schema := map[string]any{"required": []any{"then"}, "properties": map[string]any{
		"then": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/ClientAction"}, "description": "Runs next.\n\nOnly on success. More."},
		"size": map[string]any{"$ref": "#/$defs/Size"},
	}}
	r := New(defs, "ClientAction")
	if got := strings.Join(r.KeyLines(schema, true), "\n"); got != "size?: 'a'|'b'\nthen: ClientAction[] — Runs next." {
		t.Fatal(got)
	}
	r.Full = true
	if got := r.KeyLines(schema, true)[1]; got != "then: ClientAction[] — Runs next. Only on success. More." {
		t.Fatal(got)
	}
}
