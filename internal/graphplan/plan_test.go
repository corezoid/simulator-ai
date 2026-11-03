package graphplan

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

func definition(kind string, w, h float64) publicapi.GraphBlock {
	view, _ := json.Marshal(map[string]any{"shape": map[string]any{"type": "primitive", "primitive": []any{map[string]any{"kind": "rect", "x": 0, "y": 0, "w": w, "h": h}}}, "size": map[string]any{"w": w, "h": h}, "textFrame": map[string]any{"x": 5, "y": 5, "w": w - 10, "h": h - 10}})
	form, _ := json.Marshal(map[string]any{"sections": []any{map[string]any{"content": []any{map[string]any{"id": "blockId", "value": kind}, map[string]any{"id": "view", "value": string(view)}}}}})
	return publicapi.GraphBlock{ID: 22, ParentID: 21, BlockID: kind, UpdatedAt: 123, Form: form}
}
func approval() Input {
	return Input{Nodes: []Node{{"start", "startStop", "Початок"}, {"check", "process", "Перевірка заявки"}, {"decision", "decision", "Усі дані є?"}, {"approve", "process", "Погодити"}, {"end", "startStop", "Кінець"}, {"clarify", "process", "Запросити уточнення"}}, Edges: []Edge{{"begin", "start", "check", ""}, {"validate", "check", "decision", ""}, {"yes", "decision", "approve", "Так"}, {"finish", "approve", "end", ""}, {"no", "decision", "clarify", "Ні"}, {"return", "clarify", "check", ""}}}
}
func TestNativeApprovalPlan(t *testing.T) {
	input := approval()
	catalog := []publicapi.GraphBlock{definition("startStop", 150, 80), definition("process", 200, 100), definition("decision", 250, 150)}
	p, err := Build(input, catalog)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Build(input, catalog)
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatal("nondeterministic plan", err)
	}
	if !reflect.DeepEqual(p.Edges, input.Edges) || !reflect.DeepEqual(p.ReturnEdges, []string{"return"}) {
		t.Fatal("semantic cycle/labels changed")
	}
	if p.Nodes[1].Position.Y >= p.Nodes[2].Position.Y || p.Nodes[3].Position.Y != p.Nodes[5].Position.Y || p.Nodes[4].Position.Y <= p.Nodes[3].Position.Y {
		t.Fatal("invalid approval ranks")
	}
	for i, a := range p.Nodes {
		var data map[string]json.RawMessage
		if json.Unmarshal(a.Data, &data) != nil || len(data) != 2 || len(data["__form__22:view"]) == 0 || data["__form__22:view"][0] != '{' || a.FormID != 21 || a.ChildFormID != 22 {
			t.Fatal("native namespace/view drift")
		}
		for _, b := range p.Nodes[i+1:] {
			dx := a.Position.X - b.Position.X
			if dx < 0 {
				dx = -dx
			}
			dy := a.Position.Y - b.Position.Y
			if dy < 0 {
				dy = -dy
			}
			if dx < (a.LayerSettings.Width+b.LayerSettings.Width)/2 && dy < (a.LayerSettings.Height+b.LayerSettings.Height)/2 {
				t.Fatal("overlapping nodes", a.Key, b.Key)
			}
		}
	}
	p.Nodes[0].Data[0] = 'x'
	if !json.Valid(catalog[0].Form) {
		t.Fatal("plan aliases catalog")
	}
}
func TestPlanRejectsAmbiguousOrUnsupportedInput(t *testing.T) {
	cases := []Input{{}, {Nodes: []Node{{"a", "process", "A"}}}, {Nodes: []Node{{"a", "process", "A"}, {"a", "process", "B"}}, Edges: []Edge{}}, {Nodes: []Node{{"a", "process", "A"}}, Edges: []Edge{{"loop", "a", "a", ""}}}, {Nodes: []Node{{"a", "process", "A"}}, Edges: []Edge{{"missing", "a", "b", ""}}}}
	p := approval()
	p.Edges = append(p.Edges, Edge{"duplicate", "start", "check", "different label"})
	cases = append(cases, p)
	for i, p := range cases {
		if Validate(p) == nil {
			t.Fatal("accepted invalid input", i)
		}
	}
	if _, err := Build(approval(), nil); err == nil {
		t.Fatal("missing catalog accepted")
	}
}
func TestMalformedNativeDefinitionsFailClosed(t *testing.T) {
	good := definition("process", 200, 100)
	cases := []publicapi.GraphBlock{definition("process", 0, 100), definition("process", 200, 100001)}
	for _, change := range []struct{ old, new string }{{`\"kind\":\"rect\"`, `\"kind\":\"alien\"`}, {`\"w\":200`, `\"w\":null`}, {`"value":"process"`, `"value":"other"`}, {`\"type\":\"primitive\"`, `\"type\":\"invalid\"`}} {
		d := good
		d.Form = json.RawMessage(strings.ReplaceAll(string(d.Form), change.old, change.new))
		if reflect.DeepEqual(d, good) {
			t.Fatal("fixture did not mutate", change)
		}
		cases = append(cases, d)
	}
	for i, d := range cases {
		if _, _, err := decodeDefinition(d); err == nil {
			t.Fatal("accepted invalid catalog", i)
		}
	}
}
func TestDisconnectedCycleAndEmptyEdges(t *testing.T) {
	catalog := []publicapi.GraphBlock{definition("process", 200, 100)}
	p := Input{Nodes: []Node{{"a", "process", "A"}, {"b", "process", "B"}, {"c", "process", "C"}}, Edges: []Edge{{"ab", "a", "b", ""}, {"ba", "b", "a", ""}}}
	out, err := Build(p, catalog)
	if err != nil || !reflect.DeepEqual(out.ReturnEdges, []string{"ba"}) {
		t.Fatal(out, err)
	}
	p.Edges = []Edge{}
	out, err = Build(p, catalog)
	if err != nil || out.Edges == nil || out.ReturnEdges == nil {
		t.Fatal("empty edges not supported", err)
	}
}
