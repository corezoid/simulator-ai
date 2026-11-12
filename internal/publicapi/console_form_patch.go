package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FormPatchInput changes an account template field by field: each op names a field by its id (the
// key of its value in actor data), so the model never resends the whole sections array.
type FormPatchInput struct {
	FormID int64 `json:"formId"`
	// ExpectedEditToken, when sent, refuses a template that changed since it was read. Without it the
	// ops apply to the current template; only the fields they name change.
	ExpectedEditToken string        `json:"expectedEditToken"`
	Ops               []FormFieldOp `json:"ops"`
	// Form patches the template's own properties (title, description, color, ref, settings, tags,
	// parentId) as form/update does: an omitted key keeps, null clears where the server allows it.
	// settings merges key by key into the current ones (null drops a key); tags replace the list.
	Form map[string]json.RawMessage `json:"form"`
}

// FormFieldOp is one field or section operation. op picks which keys apply: add {field, section?,
// before?|after?}, update {id, set?, unset?}, remove {id}, move {id, section?|before?|after?},
// section_add {title, at?}, section_update {section, title}, section_remove {section}.
type FormFieldOp struct {
	Op      string                     `json:"op"`
	ID      string                     `json:"id"`
	Field   map[string]json.RawMessage `json:"field"`
	Set     map[string]json.RawMessage `json:"set"`
	Unset   []string                   `json:"unset"`
	Section json.RawMessage            `json:"section"`
	Before  string                     `json:"before"`
	After   string                     `json:"after"`
	Title   *string                    `json:"title"`
	At      *int                       `json:"at"`
}

// FormPatchResult is what changed, not the template: editToken chains the next patch.
type FormPatchResult struct {
	FormID    int64              `json:"formId"`
	EditToken string             `json:"editToken"`
	Changed   []FormFieldChanged `json:"changed"`
	Sections  []FormSectionCount `json:"sections"`
}

// FormFieldChanged is one op's outcome: where the field (or section) now is.
type FormFieldChanged struct {
	Op      string `json:"op"`
	ID      string `json:"id,omitempty"`
	Section int    `json:"section"`
	Index   *int   `json:"index,omitempty"`
}

// FormSectionCount outlines the template after the patch: each section's title and field count.
type FormSectionCount struct {
	Title  string `json:"title"`
	Fields int    `json:"fields"`
}

// formUpdateKeys are the template properties form/update patches besides sections.
var formUpdateKeys = []string{"title", "description", "color", "ref", "settings", "tags", "parentId"}

const maxFormPatchOps = 100

type formPatchWire struct {
	EditToken string          `json:"editToken"`
	CanModify *bool           `json:"canModify"`
	Form      json.RawMessage `json:"form"`
}

// PatchFormFields reads the template with its editor token, applies the ops to its sections in
// order (all or none) and writes the result with form/update under that token: the server's
// compare-and-set refuses a template changed in between.
func (c *Client) PatchFormFields(ctx context.Context, in FormPatchInput) (FormPatchResult, error) {
	if e := consoleOnly(c); e != nil {
		return FormPatchResult{}, e
	}
	if in.FormID < 1 || len(in.Ops)+len(in.Form) == 0 || len(in.Ops) > maxFormPatchOps {
		return FormPatchResult{}, fmt.Errorf("formId required with 1..%d ops or a form patch", maxFormPatchOps)
	}
	for k := range in.Form {
		if !contains(formUpdateKeys, k) {
			return FormPatchResult{}, fmt.Errorf("form.%s is not a template property; allowed: %s", k, strings.Join(formUpdateKeys, ", "))
		}
	}
	cur, err := call[map[string]any, formPatchWire](ctx, c, "form/get", map[string]any{"accId": c.workspace(), "formId": in.FormID, "with": []string{"editor"}})
	if err != nil {
		return FormPatchResult{}, err
	}
	if cur.CanModify != nil && !*cur.CanModify {
		return FormPatchResult{}, errors.New("you may not modify this template (needs modify access)")
	}
	if in.ExpectedEditToken != "" && in.ExpectedEditToken != cur.EditToken {
		return FormPatchResult{}, staleTemplate(cur.EditToken)
	}
	params := map[string]json.RawMessage{}
	for k, v := range in.Form {
		params[k] = v
	}
	if raw, ok := in.Form["settings"]; ok {
		merged, e := mergeSettings(cur.Form, raw)
		if e != nil {
			return FormPatchResult{}, e
		}
		params["settings"] = merged
	}
	var changed []FormFieldChanged
	var sections []map[string]json.RawMessage
	if len(in.Ops) > 0 {
		doc, e := parseFormBody(cur.Form)
		if e != nil {
			return FormPatchResult{}, e
		}
		if changed, e = doc.apply(in.Ops); e != nil {
			return FormPatchResult{}, e
		}
		sections = doc.sections()
		params["sections"], _ = json.Marshal(sections)
	}
	params["formId"], _ = json.Marshal(in.FormID)
	params["expectedEditToken"], _ = json.Marshal(cur.EditToken)
	params["accId"], _ = json.Marshal(c.workspace())
	saved, err := call[map[string]json.RawMessage, formPatchWire](ctx, c, "form/update", params)
	var api *APIError
	if errors.As(err, &api) && api.Reason == "formEditConflict" {
		return FormPatchResult{}, staleTemplate("")
	}
	if err != nil {
		return FormPatchResult{}, err
	}
	out := FormPatchResult{FormID: in.FormID, EditToken: saved.EditToken, Changed: changed}
	if sections == nil {
		doc, e := parseFormBody(saved.Form)
		if e != nil {
			return FormPatchResult{}, e
		}
		sections = doc.sections()
	}
	for _, s := range sections {
		var title string
		_ = json.Unmarshal(s["title"], &title)
		var content []json.RawMessage
		_ = json.Unmarshal(s["content"], &content)
		out.Sections = append(out.Sections, FormSectionCount{Title: title, Fields: len(content)})
	}
	return out, nil
}

// mergeSettings lays the patch's settings keys over the template's current ones (null drops a
// key): form/update replaces settings whole, and a patch of one key must not erase the rest.
func mergeSettings(body, patch json.RawMessage) (json.RawMessage, error) {
	var add map[string]json.RawMessage
	if json.Unmarshal(patch, &add) != nil || add == nil {
		return nil, errors.New("form.settings must be an object of the keys to change (null drops a key)")
	}
	var cur struct {
		Settings map[string]json.RawMessage `json:"settings"`
	}
	_ = json.Unmarshal(body, &cur)
	if cur.Settings == nil {
		cur.Settings = map[string]json.RawMessage{}
	}
	for k, v := range add {
		if string(v) == "null" {
			delete(cur.Settings, k)
		} else {
			cur.Settings[k] = v
		}
	}
	return json.Marshal(cur.Settings)
}

func staleTemplate(token string) error {
	m := "the template changed since you read it (someone else edited it): read it again with form_get and redo the ops on what is there now"
	if token != "" {
		m += "; its editToken is now " + token
	}
	return errors.New(m)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// formDoc is the template body's sections, each field kept as raw JSON so keys this tool does not
// know (and the server's number formats) survive the round trip.
type formDoc struct {
	secs []formSection
}

type formSection struct {
	props  map[string]json.RawMessage
	fields []map[string]json.RawMessage
	// initial is the field count as read (-1: added by this patch); a section the patch leaves
	// empty is refused, one stored empty is left as it is.
	initial int
}

func parseFormBody(raw json.RawMessage) (*formDoc, error) {
	var body struct {
		Sections []map[string]json.RawMessage `json:"sections"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, errors.New("the template body is not an object with sections")
		}
	}
	d := &formDoc{}
	for i, s := range body.Sections {
		sec := formSection{props: s}
		if c, ok := s["content"]; ok && string(c) != "null" {
			if err := json.Unmarshal(c, &sec.fields); err != nil {
				return nil, fmt.Errorf("section %d content is not a list of fields", i)
			}
		}
		delete(sec.props, "content")
		sec.initial = len(sec.fields)
		d.secs = append(d.secs, sec)
	}
	return d, nil
}

func (d *formDoc) sections() []map[string]json.RawMessage {
	out := make([]map[string]json.RawMessage, len(d.secs))
	for i, s := range d.secs {
		m := map[string]json.RawMessage{}
		for k, v := range s.props {
			m[k] = v
		}
		fields := s.fields
		if fields == nil {
			fields = []map[string]json.RawMessage{}
		}
		m["content"], _ = json.Marshal(fields)
		out[i] = m
	}
	return out
}

// find returns the section and index of the field with this id, or -1, -1.
func (d *formDoc) find(id string) (int, int) {
	for si, s := range d.secs {
		for fi, f := range s.fields {
			if fieldID(f) == id {
				return si, fi
			}
		}
	}
	return -1, -1
}

func fieldID(f map[string]json.RawMessage) string {
	var id string
	_ = json.Unmarshal(f["id"], &id)
	return id
}

// section resolves a section reference: its 0-based index, or its title (unique).
func (d *formDoc) section(ref json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(ref, &n); err == nil {
		if n < 0 || n >= len(d.secs) {
			return 0, fmt.Errorf("section %d does not exist: the template has %d sections (0..%d)", n, len(d.secs), len(d.secs)-1)
		}
		return n, nil
	}
	var title string
	if err := json.Unmarshal(ref, &title); err != nil {
		return 0, errors.New("section is a 0-based index or a section title")
	}
	found := -1
	var titles []string
	for i, s := range d.secs {
		var t string
		_ = json.Unmarshal(s.props["title"], &t)
		titles = append(titles, strconv.Quote(t))
		if t == title {
			if found >= 0 {
				return 0, fmt.Errorf("two sections are titled %q: name the section by its index", title)
			}
			found = i
		}
	}
	if found < 0 {
		return 0, fmt.Errorf("no section is titled %q; sections: %s", title, strings.Join(titles, ", "))
	}
	return found, nil
}
