package publicapi

import (
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
	AppID          string      `json:"appId,omitempty"` // the Smart Form whose audience may submit
}

// DataForm is the created data template and a ready page config that submits into it.
type DataForm struct {
	FormID int64  `json:"formId"`
	Page   string `json:"page"`
}

var dataFieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

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
	if !validTitle(in.Title, false) || len(in.Fields) < 1 || len(in.Fields) > 50 || utf8.RuneCountInString(in.SuccessMessage) > 200 || len(in.AppID) > 64 {
		return DataForm{}, errors.New("a title, 1..50 fields, a success message up to 200 characters and an appId up to 64 characters required")
	}
	seen := map[string]bool{}
	fields := make([]templateField, len(in.Fields))
	for i, f := range in.Fields {
		if !dataFieldName.MatchString(f.Name) || seen[f.Name] || f.Label == "" {
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

// pageNode is one object of a generated page config.
type pageNode = map[string]any

// dataPage is a CDU 2.0 page: a Form node with one bound input per field and a button that
// validates the form and submits it into the data template.
func dataPage(in DataFormInput, formID int64, workspace string) pageNode {
	var rules []pageNode
	var children []any
	for _, f := range in.Fields {
		label := literal(f.Label)
		rules = append(rules, pageNode{"name": f.Name, "label": label, "required": f.Required})
		value, path := "{{ vm."+dataFormNode+"."+f.Name+" }}", dataFormNode+"."+f.Name
		errText := "{{ vm.errors." + dataFormNode + "." + f.Name + " }}"
		set := func(from string) pageNode {
			return pageNode{"onChange": []any{pageNode{"type": "state", "mode": "set", "path": path, "value": from}}}
		}
		switch f.Type {
		case "date":
			// A calendar field holds {startDate, endDate} in Unix seconds (legacy makeCalendarItemValue),
			// the shape of the DatePicker value and event too. DatePicker has no label, required or
			// helperText props: Labels carry its caption and its validation error.
			children = append(children, pageNode{"id": f.Name + "Label", "type": "Label", "props": pageNode{"text": label}},
				pageNode{"id": f.Name, "type": "DatePicker", "props": pageNode{"mode": "single", "value": value},
					"actions": set(`{{ {"startDate": $event.startDate, "endDate": $event.endDate} }}`)},
				pageNode{"id": f.Name + "Error", "type": "Label", "props": pageNode{"text": errText, "error": true}})
		case "select":
			var options []any
			for _, o := range f.Options {
				options = append(options, pageNode{"value": literal(o), "label": literal(o)})
			}
			children = append(children, pageNode{"id": f.Name, "type": "Select", "props": pageNode{"label": label, "options": options, "value": value, "required": f.Required, "helperText": errText}, "actions": set("{{ $event.value }}")})
		default:
			props := pageNode{"label": label, "value": value, "required": f.Required, "helperText": errText}
			if f.Type == "number" {
				props["type"] = "float" // the template's edit/float; TextInput's "number" is deprecated
			}
			children = append(children, pageNode{"id": f.Name, "type": "TextInput", "props": props, "actions": set("{{ $event.value }}")})
		}
	}
	label := in.SubmitLabel
	if label == "" {
		label = "Submit"
	}
	// formData overrides the values the dispatcher sends (extra params are spread after them): a
	// field left empty, or emptied by form.reset, holds "", which without the filter would be stored
	// in a typed field of the actor, so empty values are not sent at all.
	params := pageNode{"accId": workspace, "formId": formID,
		"formData": "{{ $merge([{}, $sift(vm." + dataFormNode + `, function($v){ $v != "" and $v != null })]) }}`}
	if in.AppID != "" {
		params["appId"] = in.AppID // form/submit lets whoever may open this Smart Form submit
	}
	if in.SuccessMessage != "" {
		params["successMessage"] = literal(in.SuccessMessage)
	}
	// form/submit names the actor after a text edit field; without one it would be the template
	// name and a date, so the page names it after the template itself, within form/submit's 255
	// characters.
	if title := []rune(in.Title); !hasText(in.Fields) {
		params["title"] = literal(string(title[:min(len(title), 255)]))
	}
	submit := pageNode{"type": "form.submit", "formId": dataFormNode, "via": "simapi", "simapiMethod": "form/submit", "simapiExtraParams": params,
		"onError": []any{pageNode{"type": "notify", "level": "error", "title": "Not sent", "helperText": "{{ $error.message }}"}}}
	children = append(children, pageNode{"id": "submit", "type": "Button", "props": pageNode{"title": literal(label), "type": "filled", "variant": "primary"},
		"actions": pageNode{"onClick": []any{pageNode{"type": "form.validate", "form": dataFormNode}, pageNode{"type": "if", "cond": "vm.valid." + dataFormNode + " = true", "then": []any{submit}}}}})
	return pageNode{"schemaVersion": "2.0", "id": "index", "tree": pageNode{"id": "root", "type": "Stack", "props": pageNode{"direction": "vertical"},
		"children": []any{pageNode{"id": dataFormNode, "type": "Form", "props": pageNode{"valueKey": dataFormNode, "fields": rules}, "children": children}}}}
}

// literal keeps the agent's text literal: the renderer resolves every string holding {{, so
// such text becomes a JSONata string literal with its braces escaped (one {{ … }} in all).
func literal(s string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	quoted, _ := json.Marshal(s)
	escaped := strings.NewReplacer("{", `\u007b`, "}", `\u007d`).Replace(string(quoted))
	return "{{ " + escaped + " }}"
}

func hasText(fields []DataField) bool {
	for _, f := range fields {
		if f.Type == "" || f.Type == "text" {
			return true
		}
	}
	return false
}
