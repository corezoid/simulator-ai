package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// A Console Smart Form saves what the user submits as an actor of a data template: the page's
// form.submit calls public form/submit (via simapi) with that template's formId, and each field
// name of the page's Form node is the id of the template field it fills.

// DataField is one field of a Smart Form and of its data template.
type DataField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type,omitempty"` // text (default), number, date or select
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"` // select choices
}

type DataFormInput struct {
	Title          string      `json:"title"`
	Fields         []DataField `json:"fields"`
	SubmitLabel    string      `json:"submitLabel,omitempty"`
	SuccessMessage string      `json:"successMessage,omitempty"`
}

// DataForm is the created data template and a ready page config that submits into it.
type DataForm struct {
	FormID int64  `json:"formId"`
	Page   string `json:"page"`
}

var fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// dataFormNode is the Form node id of generated pages; vm.<it>.<field> holds the values.
const dataFormNode = "request"

type templateField struct {
	ID       string           `json:"id"`
	Class    string           `json:"class"`
	Type     string           `json:"type,omitempty"`
	Title    string           `json:"title"`
	Required bool             `json:"required,omitempty"`
	Options  []map[string]any `json:"options,omitempty"`
}
type templateSection struct {
	Title   string          `json:"title"`
	Content []templateField `json:"content"`
}
type dataFormRequest struct {
	AccID      string            `json:"accId"`
	Title      string            `json:"title"`
	IsTemplate bool              `json:"isTemplate"`
	Sections   []templateSection `json:"sections"`
}

// CreateDataForm creates the data template of a Smart Form and returns the page that submits into it.
func (c *Client) CreateDataForm(ctx context.Context, in DataFormInput) (DataForm, error) {
	if e := consoleOnly(c); e != nil {
		return DataForm{}, e
	}
	if !validTitle(in.Title, false) || len(in.Fields) < 1 || len(in.Fields) > 50 || utf8.RuneCountInString(in.SuccessMessage) > 200 {
		return DataForm{}, errors.New("a title, 1..50 fields and a success message up to 200 characters required")
	}
	seen := map[string]bool{}
	fields := make([]templateField, len(in.Fields))
	for i, f := range in.Fields {
		if !fieldName.MatchString(f.Name) || seen[f.Name] || f.Label == "" {
			return DataForm{}, fmt.Errorf("field %d: a unique name (letters, digits, _) and a label required", i)
		}
		seen[f.Name] = true
		t := templateField{ID: f.Name, Class: "edit", Type: "text", Title: f.Label, Required: f.Required}
		switch f.Type {
		case "", "text":
		case "number":
			t.Type = "float"
		case "date":
			t.Class, t.Type = "calendar", ""
		case "select":
			if len(f.Options) == 0 {
				return DataForm{}, fmt.Errorf("field %s: a select needs options", f.Name)
			}
			t.Class, t.Type = "select", ""
			for _, o := range f.Options {
				t.Options = append(t.Options, map[string]any{"title": o, "value": o})
			}
		default:
			return DataForm{}, fmt.Errorf("field %s: type is text, number, date or select", f.Name)
		}
		fields[i] = t
	}
	form, err := call[dataFormRequest, struct {
		ID int64 `json:"id"`
	}](ctx, c, "form/create", dataFormRequest{AccID: c.config.Workspace, Title: in.Title, IsTemplate: true, Sections: []templateSection{{Title: in.Title, Content: fields}}})
	if err != nil {
		return DataForm{}, err
	}
	if form.ID < 1 {
		return DataForm{}, errors.New("form/create returned no template id")
	}
	page, err := json.Marshal(dataPage(in, form.ID, c.config.Workspace))
	return DataForm{FormID: form.ID, Page: string(page)}, err
}

type node = map[string]any

// dataPage is a CDU 2.0 page: a Form node with one bound input per field and a button that
// validates the form and submits it into the data template.
func dataPage(in DataFormInput, formID int64, workspace string) node {
	var rules []node
	var children []any
	for _, f := range in.Fields {
		rules = append(rules, node{"name": f.Name, "label": f.Label, "required": f.Required})
		value, path := "{{ vm."+dataFormNode+"."+f.Name+" }}", dataFormNode+"."+f.Name
		errText := "{{ vm.errors." + dataFormNode + "." + f.Name + " }}"
		set := func(from string) node {
			return node{"onChange": []any{node{"type": "state", "mode": "set", "path": path, "value": from}}}
		}
		switch f.Type {
		case "date":
			// DatePicker has no label prop; its event is {startDate, endDate, ...} in Unix seconds.
			children = append(children, node{"id": f.Name + "Label", "type": "Label", "props": node{"text": f.Label}},
				node{"id": f.Name, "type": "DatePicker", "props": node{"mode": "single"}, "actions": set("{{ $event.startDate }}")})
		case "select":
			var options []any
			for _, o := range f.Options {
				options = append(options, node{"value": o, "label": o})
			}
			children = append(children, node{"id": f.Name, "type": "Select", "props": node{"label": f.Label, "options": options, "value": value, "required": f.Required, "helperText": errText}, "actions": set("{{ $event.value }}")})
		default:
			props := node{"label": f.Label, "value": value, "required": f.Required, "helperText": errText}
			if f.Type == "number" {
				props["type"] = "number"
			}
			children = append(children, node{"id": f.Name, "type": "TextInput", "props": props, "actions": set("{{ $event.value }}")})
		}
	}
	label, message := in.SubmitLabel, in.SuccessMessage
	if label == "" {
		label = "Submit"
	}
	params := node{"accId": workspace, "formId": formID}
	if message != "" {
		params["successMessage"] = message
	}
	submit := node{"type": "form.submit", "formId": dataFormNode, "via": "simapi", "simapiMethod": "form/submit", "simapiExtraParams": params,
		"onError": []any{node{"type": "notify", "level": "error", "title": "Not sent", "helperText": "{{ $error.message }}"}}}
	children = append(children, node{"id": "submit", "type": "Button", "props": node{"title": label, "type": "filled", "variant": "primary"},
		"actions": node{"onClick": []any{node{"type": "form.validate", "form": dataFormNode}, node{"type": "if", "cond": "vm.valid." + dataFormNode + " = true", "then": []any{submit}}}}})
	return node{"schemaVersion": "2.0", "id": "index", "tree": node{"id": "root", "type": "Stack", "props": node{"direction": "vertical"},
		"children": []any{node{"id": dataFormNode, "type": "Form", "props": node{"valueKey": dataFormNode, "fields": rules}, "children": children}}}}
}

// wireSubmits checks a page config before it is stored and returns the source to store. It refuses
// what made a Console form send nothing (acceptance D7): a Form node no form.submit names, and a
// button without any action beside inputs on a page without a Form. A form.submit to form/submit must
// name its data template, and its accId is set to the host workspace. Other submits (via corezoid,
// other methods) are the page author's and pass as they are. A source that is not a page config, or
// needs no accId, is stored byte for byte.
func wireSubmits(source, workspace string) (string, error) {
	var doc any
	if json.Unmarshal([]byte(source), &doc) != nil {
		return source, nil
	}
	if page, ok := doc.(map[string]any); !ok || page["tree"] == nil || page["schemaVersion"] == nil {
		return source, nil
	}
	forms, submitted := map[string]bool{}, map[string]bool{}
	inputs, idleButtons, refs, changed := 0, 0, false, false
	var problems []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if _, ok := t["$ref"]; ok {
				refs = true // a fragment may hold the submit
			}
			switch t["type"] {
			case "TextInput", "DatePicker", "Select", "Checkbox", "Radio", "Toggle", "Slider", "Upload":
				inputs++
			case "Button":
				if actions, _ := t["actions"].(map[string]any); len(actions) == 0 {
					idleButtons++
				}
			case "Form":
				if id, ok := t["id"].(string); ok {
					forms[id] = true
				}
			case "form.submit":
				id, _ := t["formId"].(string)
				submitted[id] = true
				if t["via"] == "simapi" && t["simapiMethod"] == "form/submit" {
					params, _ := t["simapiExtraParams"].(map[string]any)
					if params == nil || !positive(params["formId"]) && !expression(params["formId"]) {
						problems = append(problems, fmt.Sprintf("form.submit of %q needs simapiExtraParams.formId, the data template id", id))
					} else if params["accId"] != workspace {
						params["accId"], changed = workspace, true
					}
				}
			}
			for _, x := range t {
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(doc)
	if inputs > 0 && idleButtons > 0 && len(forms) == 0 {
		problems = append(problems, "the page has inputs and a button with no action but no Form node, so nothing would be sent")
	}
	for id := range forms {
		if !submitted[id] && !refs {
			problems = append(problems, fmt.Sprintf("Form %q has no button with form.submit", id))
		}
	}
	if len(problems) > 0 {
		return "", errors.New("page submit wiring: " + strings.Join(problems, "; ") + ". Create the data template and a ready page with smart_form_data_form_create")
	}
	if !changed {
		return source, nil
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(doc)
	return strings.TrimSuffix(out.String(), "\n"), err
}

func expression(v any) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, "{{")
}

func positive(v any) bool {
	n, ok := v.(float64)
	return ok && n >= 1 && n == float64(int64(n))
}
