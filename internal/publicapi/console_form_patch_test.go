package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// formBody is a template with two sections; field "note" carries a key this tool does not know,
// which must survive the round trip and not block an update of that field.
const formBody = `{"id":7,"canModify":true,"editToken":"v1:t1","form":{"title":"T","settings":{"x":1},"sections":[` +
	`{"title":"main","content":[{"id":"name","class":"edit","title":"Name"},{"id":"note","class":"edit","title":"Note","custom":{"k":1.50}}]},` +
	`{"title":"tech","content":[{"id":"kind","class":"select","title":"Kind","options":[{"title":"A","value":"a"}]}]}]}}`

func patchBackend(t *testing.T, get string, update string) (*Client, *[]rpcCall) {
	t.Helper()
	return rpcBackend(t, func(method string) string {
		if method == "form/get" {
			return get
		}
		return update
	})
}

func sectionsSent(t *testing.T, calls []rpcCall) string {
	t.Helper()
	b, _ := json.Marshal(calls[len(calls)-1].Params)
	return string(b)
}

func TestPatchFormFieldsAppliesOpsByID(t *testing.T) {
	c, calls := patchBackend(t, formBody, `{"id":7,"editToken":"v1:t2","form":{}}`)
	ops := []FormFieldOp{
		{Op: "add", Field: map[string]json.RawMessage{"id": json.RawMessage(`"age"`), "class": json.RawMessage(`"edit"`), "type": json.RawMessage(`"int"`), "title": json.RawMessage(`"Age"`)}, After: "name"},
		{Op: "update", ID: "name", Set: map[string]json.RawMessage{"required": json.RawMessage(`true`)}, Unset: []string{"title"}},
		{Op: "move", ID: "kind", Section: json.RawMessage(`"main"`)},
		{Op: "update", ID: "note", Set: map[string]json.RawMessage{"title": json.RawMessage(`"N"`)}},
		{Op: "remove", ID: "note"},
		{Op: "section_remove", Section: json.RawMessage(`1`)},
	}
	out, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, ExpectedEditToken: "v1:t1", Ops: ops, Form: map[string]json.RawMessage{"title": json.RawMessage(`"New"`)}})
	if err != nil {
		t.Fatal(err)
	}
	sent := sectionsSent(t, *calls)
	for _, want := range []string{
		`"expectedEditToken":"v1:t1"`, `"title":"New"`, `"formId":7`, `"accId":"ws"`,
		`"sections":[{"content":[{"class":"edit","id":"name","required":true},{"class":"edit","id":"age","title":"Age","type":"int"},{"class":"select","id":"kind","options":[{"title":"A","value":"a"}],"title":"Kind"}],"title":"main"}]`,
	} {
		if !strings.Contains(sent, want) {
			t.Errorf("update lacks %s:\n%s", want, sent)
		}
	}
	if out.EditToken != "v1:t2" || len(out.Changed) != 6 || out.Changed[0].ID != "age" || *out.Changed[0].Index != 1 ||
		len(out.Sections) != 1 || out.Sections[0] != (FormSectionCount{Title: "main", Fields: 3}) {
		t.Fatalf("%+v", out)
	}
}

func TestPatchFormFieldsKeepsUnknownKeysAndGeneratesIDs(t *testing.T) {
	c, calls := patchBackend(t, formBody, `{"editToken":"v1:t2"}`)
	out, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, Ops: []FormFieldOp{
		{Op: "add", Field: map[string]json.RawMessage{"class": json.RawMessage(`"check"`), "title": json.RawMessage(`"Done"`)}, Section: json.RawMessage(`1`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	sent := sectionsSent(t, *calls)
	if (*calls)[len(*calls)-1].Method != "form/update" || !strings.Contains(sent, `"custom":{"k":1.5}`) || !strings.Contains(sent, `"expectedEditToken":"v1:t1"`) {
		t.Fatalf("lost a key or the read token: %s", sent)
	}
	if id := out.Changed[0].ID; !strings.HasPrefix(id, "item_") || out.Changed[0].Section != 1 || *out.Changed[0].Index != 1 || !strings.Contains(sent, `"id":"`+id+`"`) {
		t.Fatalf("%+v %s", out.Changed, sent)
	}
}

func TestPatchFormFieldsRefusesWithPreciseErrors(t *testing.T) {
	c, calls := patchBackend(t, formBody, `{}`)
	field := func(kv string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		_ = json.Unmarshal([]byte(kv), &m)
		return m
	}
	for _, tc := range []struct {
		in   FormPatchInput
		want string
	}{
		{FormPatchInput{FormID: 7, ExpectedEditToken: "v1:old", Ops: []FormFieldOp{{Op: "remove", ID: "name"}}}, "changed since you read it"},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "remove", ID: "nope"}}}, `ops[0] (remove): no field has id "nope"; fields: name, note, kind`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "add", Field: field(`{"id":"x","class":"textarea"}`)}}}, `field.class "textarea" is not a field class; allowed: edit, select, multiSelect`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "add", Field: field(`{"id":"x","class":"edit","type":"number"}`)}}}, `field.type "number" is not allowed; allowed: text, password, email, phone, int, float`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "add", Field: field(`{"id":"name","class":"edit"}`)}}}, `a field with id "name" already exists`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "add", Field: field(`{"id":"a.b","class":"edit"}`)}}}, `without '.', '[' or ']'`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "update", ID: "name", Set: field(`{"visibility":"shown"}`)}}}, `field "name": set.visibility "shown" is not allowed; allowed: visible, disabled, hidden`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "update", ID: "name", Set: field(`{"id":"n2"}`)}}}, "set.id is refused"},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "update", ID: "name", Set: field(`{"colour":"red"}`)}}}, "set.colour is not a field property; allowed: id, class"},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "update", ID: "kind", Set: field(`{"options":[{"title":"A","value":"a"},{"title":"B","value":"a"}]}`)}}}, `set.options[1].value "a" repeats options[0]`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "update", ID: "name", Set: field(`{"options":[]}`)}}}, "set.options belongs to a select/multiSelect/radio/check field, not edit"},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "move", ID: "name", Section: json.RawMessage(`"misc"`)}}}, `no section is titled "misc"; sections: "main", "tech"`},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "section_remove", Section: json.RawMessage(`0`)}}}, "section 0 still has 2 fields"},
		{FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "rename", ID: "name"}}}, `unknown op "rename"; allowed: add, update`},
		{FormPatchInput{FormID: 7, Form: map[string]json.RawMessage{"sections": json.RawMessage(`[]`)}}, "form.sections is not a template property"},
		{FormPatchInput{FormID: 7}, "formId required with 1..100 ops"},
	} {
		_, err := c.PatchFormFields(context.Background(), tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("want %q, got %v", tc.want, err)
		}
	}
	for _, call := range *calls {
		if call.Method == "form/update" {
			t.Fatal("a refused patch wrote the template")
		}
	}
}

func TestPatchFormFieldsConflictOnWrite(t *testing.T) {
	c, _ := patchBackend(t, formBody, `"error":{"code":-32000,"message":"form changed since it was loaded","data":{"reason":"formEditConflict","reasonParams":{"currentEditToken":"v1:t9","currentSnapshot":{"big":"body"}}}}`)
	_, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, Ops: []FormFieldOp{{Op: "remove", ID: "name"}}})
	if err == nil || !strings.Contains(err.Error(), "changed since you read it") || strings.Contains(err.Error(), "big") {
		t.Fatalf("%v", err)
	}
}

func TestPatchFormFieldsNeedsModify(t *testing.T) {
	c, _ := patchBackend(t, `{"canModify":false,"editToken":"v1:t1","form":{"sections":[]}}`, `{}`)
	if _, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, Form: map[string]json.RawMessage{"title": json.RawMessage(`"x"`)}}); err == nil || !strings.Contains(err.Error(), "may not modify") {
		t.Fatal(err)
	}
	c.config.Profile = "read"
	if _, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, Form: map[string]json.RawMessage{"title": json.RawMessage(`"x"`)}}); err == nil {
		t.Fatal("patched outside the console profile")
	}
}

// Review fixes: settings merge into the current ones, null section is "not given", a section the
// patch empties is refused, and values the op does not touch are not re-validated.
func TestPatchFormFieldsReviewRules(t *testing.T) {
	legacy := `{"canModify":true,"editToken":"v1:t1","form":{"settings":{"hideAfterSend":false,"keep":1},"sections":[` +
		`{"title":"main","content":[{"id":"old","class":"edit","type":"number","visibility":"shown","title":"Old"}]},{"title":"tech","content":[{"id":"t","class":"check"}]}]}}`
	c, calls := patchBackend(t, legacy, `{"editToken":"v1:t2"}`)
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	_, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7,
		Ops:  []FormFieldOp{{Op: "update", ID: "old", Set: map[string]json.RawMessage{"title": raw(`"New"`)}}, {Op: "add", Field: map[string]json.RawMessage{"class": raw(`"check"`)}, Section: raw(`null`)}},
		Form: map[string]json.RawMessage{"settings": raw(`{"hideAfterSend":true,"keep":null,"extra":"x"}`)}})
	if err != nil {
		t.Fatal("legacy values the op did not touch were re-validated:", err)
	}
	sent := sectionsSent(t, *calls)
	if !strings.Contains(sent, `"settings":{"extra":"x","hideAfterSend":true}`) || !strings.Contains(sent, `{"class":"check","id":"t"},{"class":"check","id":"item_`) {
		t.Fatalf("settings not merged or null section not the default: %s", sent)
	}
	for _, tc := range []struct {
		ops  []FormFieldOp
		want string
	}{
		{[]FormFieldOp{{Op: "remove", ID: "t"}}, `section 1 "tech" would have no fields`},
		{[]FormFieldOp{{Op: "section_add", Title: strPtr("empty")}}, `section 2 "empty" would have no fields`},
		{[]FormFieldOp{{Op: "update", ID: "old", Set: map[string]json.RawMessage{"class": raw(`"edit"`)}}}, `set.type "number" is not allowed`},
		{[]FormFieldOp{{Op: "move", ID: "old", Section: raw(`null`)}}, "move needs section, before or after"},
	} {
		if _, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, Ops: tc.ops}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("want %q, got %v", tc.want, err)
		}
	}
	if _, err := c.PatchFormFields(context.Background(), FormPatchInput{FormID: 7, Form: map[string]json.RawMessage{"settings": raw(`[1]`)}}); err == nil {
		t.Error("settings that are not an object accepted")
	}
}

func strPtr(s string) *string { return &s }
