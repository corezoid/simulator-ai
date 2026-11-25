package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The strict check of an actor's data against its template. sim-api's own check on a write
// (service_form.CheckStoredValue) is lenient: a key the template does not know passes unchecked. A
// model that misspells a field id would then store a value nobody sees, so the patch refuses it and
// lists the template's fields instead.

// schemaField is one value-holding field of a template body (e_form.FastFormSectionContent).
type schemaField struct {
	ID       string `json:"id"`
	Class    string `json:"class"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Options  []struct {
		Title string `json:"title"`
		Value string `json:"value"`
	} `json:"options"`
	Extra json.RawMessage `json:"extra"`
}

// valueClasses are the field classes that hold a value (service_form.HoldsValue).
var valueClasses = map[string]bool{"edit": true, "select": true, "radio": true, "multiSelect": true, "check": true, "calendar": true, "upload": true}

// template is a template with its value-holding fields by id.
type template struct {
	ID     int64
	Title  string
	Fields map[string]schemaField
	Order  []string
}

type formWire struct {
	ID    int64           `json:"id"`
	Title string          `json:"title"`
	Form  json.RawMessage `json:"form"`
}

// readTemplate reads a template and its fields.
func (c *Client) readTemplate(ctx context.Context, id int64) (template, error) {
	f, err := call[map[string]any, formWire](ctx, c, "form/get", map[string]any{"accId": c.workspace(), "formId": id})
	if err != nil {
		return template{}, err
	}
	t := template{ID: id, Title: f.Title, Fields: map[string]schemaField{}}
	var body struct {
		Sections []struct {
			Content []schemaField `json:"content"`
		} `json:"sections"`
	}
	if len(f.Form) > 0 && string(f.Form) != "null" {
		if json.Unmarshal(f.Form, &body) != nil {
			return template{}, fmt.Errorf("template %d: its body is not an object with sections", id)
		}
	}
	for _, s := range body.Sections {
		for _, field := range s.Content {
			if field.ID != "" && valueClasses[field.Class] {
				if _, dup := t.Fields[field.ID]; !dup {
					t.Order = append(t.Order, field.ID)
				}
				t.Fields[field.ID] = field
			}
		}
	}
	return t, nil
}

// findTemplate resolves a template by its title: exactly one template of that title (any case).
func (c *Client) findTemplate(ctx context.Context, title string) (int64, error) {
	page, err := call[map[string]any, struct {
		List []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"list"`
	}](ctx, c, "form/list", map[string]any{"accId": c.workspace(), "query": title, "type": "all", "limit": 100})
	if err != nil {
		return 0, err
	}
	var ids []int64
	for _, f := range page.List {
		if strings.EqualFold(strings.TrimSpace(f.Title), strings.TrimSpace(title)) {
			ids = append(ids, f.ID)
		}
	}
	switch len(ids) {
	case 0:
		return 0, fmt.Errorf("no template titled %q (forms_list finds one)", title)
	case 1:
		return ids[0], nil
	}
	return 0, fmt.Errorf("%d templates are titled %q (ids %v): pass formId", len(ids), title, ids)
}

// describe lists the template's fields for an error: id (title, class: options).
func (t template) describe() string {
	parts := make([]string, 0, len(t.Order))
	for _, id := range t.Order {
		f := t.Fields[id]
		s := fmt.Sprintf("%s (%s, %s", id, f.Title, f.Class)
		if f.Type != "" && f.Class == "edit" {
			s += " " + f.Type
		}
		if opts := f.optionValues(); len(opts) > 0 && !f.dynamic() {
			s += ": " + strings.Join(opts, " | ")
		}
		parts = append(parts, s+")")
	}
	if len(parts) == 0 {
		return "it has no value fields"
	}
	return "its fields: " + strings.Join(parts, "; ")
}

func (f schemaField) optionValues() []string {
	out := make([]string, 0, len(f.Options))
	for _, o := range f.Options {
		label := o.Value
		if o.Title != "" && o.Title != o.Value {
			label = o.Value + "=" + o.Title
		}
		out = append(out, label)
	}
	return out
}

// dynamic says the options come from elsewhere (extra.optionsSource): any value is the source's.
func (f schemaField) dynamic() bool {
	var extra struct {
		OptionsSource json.RawMessage `json:"optionsSource"`
	}
	return json.Unmarshal(f.Extra, &extra) == nil && len(extra.OptionsSource) > 0 && string(extra.OptionsSource) != "null"
}

// checkData checks every key of data against the template and returns data with choice values in
// their stored shape ([{value, title}]); a missing required field is a warning, not a refusal.
func (t template) checkData(data json.RawMessage) (json.RawMessage, []string, error) {
	values := map[string]json.RawMessage{}
	if len(data) > 0 && string(data) != "null" {
		if json.Unmarshal(data, &values) != nil {
			return nil, nil, errors.New("data must be an object of template field values")
		}
	}
	var problems, warnings []string
	unknown := false
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f, ok := t.Fields[k]
		if !ok {
			problems, unknown = append(problems, fmt.Sprintf("field %q is not in template %q (%d)", k, t.Title, t.ID)), true
			continue
		}
		v, err := f.normalize(values[k])
		if err != nil {
			problems = append(problems, fmt.Sprintf("field %q (%s): %v", k, f.Title, err))
			continue
		}
		values[k] = v
	}
	for _, id := range t.Order {
		if _, given := values[id]; !given && t.Fields[id].Required {
			warnings = append(warnings, fmt.Sprintf("required field %q (%s) is not given", id, t.Fields[id].Title))
		}
	}
	if unknown {
		problems = append(problems, t.describe())
	}
	if len(problems) > 0 {
		return nil, nil, errors.New(strings.Join(problems, "; "))
	}
	out, err := json.Marshal(values)
	return out, warnings, err
}

// normalize checks one value against its field and returns it as the field stores it.
func (f schemaField) normalize(raw json.RawMessage) (json.RawMessage, error) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, errors.New("not a JSON value")
	}
	if v == nil {
		return raw, nil
	}
	switch f.Class {
	case "edit":
		if f.Type == "int" || f.Type == "float" {
			if _, ok := v.(float64); !ok {
				return nil, fmt.Errorf("takes a number (%s)", f.Type)
			}
		}
		return raw, nil
	case "select", "radio", "multiSelect", "check":
		return f.normalizeChoice(v, raw)
	}
	return raw, nil
}

// normalizeChoice accepts an option's value or title, a list of them, or stored {value, title}
// items, and returns the stored list; a value that is no option is refused with the options.
func (f schemaField) normalizeChoice(v any, raw json.RawMessage) (json.RawMessage, error) {
	if b, ok := v.(bool); ok && f.Class == "check" {
		return json.Marshal(b)
	}
	list, isList := v.([]any)
	if !isList {
		list = []any{v}
	}
	if (f.Class == "select" || f.Class == "radio") && len(list) > 1 {
		return nil, errors.New("takes one value")
	}
	if f.dynamic() {
		return raw, nil
	}
	type item struct {
		Value string `json:"value"`
		Title string `json:"title"`
	}
	out := make([]item, 0, len(list))
	for _, entry := range list {
		text, ok := entry.(string)
		if obj, isObj := entry.(map[string]any); isObj {
			text, ok = obj["value"].(string)
		}
		if !ok {
			return nil, fmt.Errorf("takes its options: %s", strings.Join(f.optionValues(), " | "))
		}
		found := false
		for _, o := range f.Options {
			if o.Value == text || strings.EqualFold(o.Title, text) {
				out, found = append(out, item{Value: o.Value, Title: o.Title}), true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%q is not one of its options: %s", text, strings.Join(f.optionValues(), " | "))
		}
	}
	return json.Marshal(out)
}
