package publicapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// The field vocabulary the Account Template editor writes (ui-kit account-template protocol and
// inspector, pong-server's form item schema). Values outside it are refused with this list.
var (
	fieldClasses    = []string{"edit", "select", "multiSelect", "radio", "check", "calendar", "upload", "label", "button", "image", "link"}
	editTypes       = []string{"text", "password", "email", "phone", "int", "float"}
	fieldVisibility = []string{"visible", "disabled", "hidden"}
	// fieldKeys are the keys of a form item (sim-api FormSectionContent).
	fieldKeys = []string{"id", "class", "title", "type", "value", "options", "required", "visibility", "description",
		"regexp", "errorMsg", "extra", "align", "w", "row", "key", "idNotChanged", "autocomplete", "schema"}
	optionClasses = []string{"select", "multiSelect", "radio", "check"}
	fieldOps      = []string{"add", "update", "remove", "move", "section_add", "section_update", "section_remove"}
)

// badFieldID is the path syntax sim-api refuses in a field id (actor data lookups split on it).
var badFieldID = regexp.MustCompile(`[.\[\]]`)

// apply runs the ops in order on the document; the first refused op stops the patch, naming its
// index, and nothing is written.
func (d *formDoc) apply(ops []FormFieldOp) ([]FormFieldChanged, error) {
	out := make([]FormFieldChanged, 0, len(ops))
	for i, op := range ops {
		ch, err := d.applyOne(op)
		if err != nil {
			return nil, fmt.Errorf("ops[%d] (%s): %w; nothing was changed", i, op.Op, err)
		}
		out = append(out, ch)
	}
	for i, s := range d.secs {
		if len(s.fields) == 0 && s.initial != 0 {
			var title string
			_ = json.Unmarshal(s.props["title"], &title)
			return nil, fmt.Errorf("section %d %q would have no fields: a template section holds at least one (the editor's schema), so add a field to it or section_remove it; nothing was changed", i, title)
		}
	}
	return out, nil
}

// given says whether an optional JSON argument was sent with a value: null is "not given".
func given(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func (d *formDoc) applyOne(op FormFieldOp) (FormFieldChanged, error) {
	switch op.Op {
	case "add":
		return d.add(op)
	case "update":
		return d.update(op)
	case "remove":
		si, fi, err := d.mustFind(op.ID)
		if err != nil {
			return FormFieldChanged{}, err
		}
		d.secs[si].fields = append(d.secs[si].fields[:fi], d.secs[si].fields[fi+1:]...)
		return FormFieldChanged{Op: op.Op, ID: op.ID, Section: si, Index: &fi}, nil
	case "move":
		si, fi, err := d.mustFind(op.ID)
		if err != nil {
			return FormFieldChanged{}, err
		}
		if !given(op.Section) && op.Before == "" && op.After == "" {
			return FormFieldChanged{}, errors.New("move needs section, before or after")
		}
		f := d.secs[si].fields[fi]
		d.secs[si].fields = append(d.secs[si].fields[:fi], d.secs[si].fields[fi+1:]...)
		ts, ti, err := d.target(op)
		if err != nil {
			return FormFieldChanged{}, err
		}
		d.insert(ts, ti, f)
		return FormFieldChanged{Op: op.Op, ID: op.ID, Section: ts, Index: &ti}, nil
	case "section_add":
		if op.Title == nil {
			return FormFieldChanged{}, errors.New("section_add needs title")
		}
		at := len(d.secs)
		if op.At != nil {
			if *op.At < 0 || *op.At > len(d.secs) {
				return FormFieldChanged{}, fmt.Errorf("at must be 0..%d", len(d.secs))
			}
			at = *op.At
		}
		title, _ := json.Marshal(*op.Title)
		sec := formSection{props: map[string]json.RawMessage{"title": title}, initial: -1}
		d.secs = append(d.secs[:at], append([]formSection{sec}, d.secs[at:]...)...)
		return FormFieldChanged{Op: op.Op, Section: at}, nil
	case "section_update":
		si, err := d.sectionOf(op)
		if err != nil {
			return FormFieldChanged{}, err
		}
		if op.Title == nil {
			return FormFieldChanged{}, errors.New("section_update needs title")
		}
		d.secs[si].props["title"], _ = json.Marshal(*op.Title)
		return FormFieldChanged{Op: op.Op, Section: si}, nil
	case "section_remove":
		si, err := d.sectionOf(op)
		if err != nil {
			return FormFieldChanged{}, err
		}
		if n := len(d.secs[si].fields); n > 0 {
			return FormFieldChanged{}, fmt.Errorf("section %d still has %d fields: move or remove them first", si, n)
		}
		d.secs = append(d.secs[:si], d.secs[si+1:]...)
		return FormFieldChanged{Op: op.Op, Section: si}, nil
	}
	return FormFieldChanged{}, fmt.Errorf("unknown op %q; allowed: %s", op.Op, strings.Join(fieldOps, ", "))
}

func (d *formDoc) add(op FormFieldOp) (FormFieldChanged, error) {
	if len(op.Field) == 0 {
		return FormFieldChanged{}, errors.New("add needs field {class, title, ...}")
	}
	f := map[string]json.RawMessage{}
	for k, v := range op.Field {
		f[k] = v
	}
	id := fieldID(f)
	if _, given := f["id"]; !given {
		id = newFieldID()
		f["id"], _ = json.Marshal(id)
	}
	if _, ok := f["class"]; !ok {
		return FormFieldChanged{}, fmt.Errorf("field.class is required; allowed: %s", strings.Join(fieldClasses, ", "))
	}
	if err := checkField(f, op.Field, "field"); err != nil {
		return FormFieldChanged{}, err
	}
	if si, _ := d.find(id); si >= 0 {
		return FormFieldChanged{}, fmt.Errorf("a field with id %q already exists (section %d): ids are unique keys of actor data", id, si)
	}
	if len(d.secs) == 0 {
		title, _ := json.Marshal("main")
		d.secs = append(d.secs, formSection{props: map[string]json.RawMessage{"title": title}, initial: -1})
	}
	si, fi, err := d.target(op)
	if err != nil {
		return FormFieldChanged{}, err
	}
	d.insert(si, fi, f)
	return FormFieldChanged{Op: op.Op, ID: id, Section: si, Index: &fi}, nil
}

func (d *formDoc) update(op FormFieldOp) (FormFieldChanged, error) {
	si, fi, err := d.mustFind(op.ID)
	if err != nil {
		return FormFieldChanged{}, err
	}
	if len(op.Set)+len(op.Unset) == 0 {
		return FormFieldChanged{}, errors.New("update needs set or unset")
	}
	if _, renames := op.Set["id"]; renames {
		return FormFieldChanged{}, errors.New("set.id is refused: the id is the key of this field's value in every actor's data, a new id orphans those values; add a new field (and remove this one) only if the user wants that")
	}
	f := map[string]json.RawMessage{}
	for k, v := range d.secs[si].fields[fi] {
		f[k] = v
	}
	for k, v := range op.Set {
		f[k] = v
	}
	for _, k := range op.Unset {
		if k == "id" || k == "class" {
			return FormFieldChanged{}, fmt.Errorf("unset.%s is refused: every field has an id and a class", k)
		}
		if !contains(fieldKeys, k) {
			return FormFieldChanged{}, fmt.Errorf("unset.%s is not a field property; allowed: %s", k, strings.Join(fieldKeys, ", "))
		}
		delete(f, k)
	}
	if err := checkField(f, op.Set, "set"); err != nil {
		return FormFieldChanged{}, fmt.Errorf("field %q: %w", op.ID, err)
	}
	d.secs[si].fields[fi] = f
	return FormFieldChanged{Op: op.Op, ID: op.ID, Section: si, Index: &fi}, nil
}

func (d *formDoc) mustFind(id string) (int, int, error) {
	if id == "" {
		return 0, 0, errors.New("id (the field id) is required")
	}
	si, fi := d.find(id)
	if si < 0 {
		return 0, 0, fmt.Errorf("no field has id %q; fields: %s", id, d.fieldIDs())
	}
	return si, fi, nil
}

func (d *formDoc) fieldIDs() string {
	var ids []string
	for _, s := range d.secs {
		for _, f := range s.fields {
			ids = append(ids, fieldID(f))
		}
	}
	if len(ids) > 60 {
		return strings.Join(ids[:60], ", ") + fmt.Sprintf(" … (%d in all)", len(ids))
	}
	if len(ids) == 0 {
		return "(none)"
	}
	return strings.Join(ids, ", ")
}

func (d *formDoc) sectionOf(op FormFieldOp) (int, error) {
	if !given(op.Section) {
		return 0, errors.New("section (index or title) is required")
	}
	return d.section(op.Section)
}

// target is where an added or moved field goes: before or after a field (in that field's
// section), else the end of the named section, else the end of the last section.
func (d *formDoc) target(op FormFieldOp) (int, int, error) {
	if op.Before != "" && op.After != "" {
		return 0, 0, errors.New("give before or after, not both")
	}
	if anchor := op.Before + op.After; anchor != "" {
		si, fi := d.find(anchor)
		if si < 0 {
			return 0, 0, fmt.Errorf("no field has id %q (the before/after anchor); fields: %s", anchor, d.fieldIDs())
		}
		if op.After != "" {
			fi++
		}
		return si, fi, nil
	}
	si := len(d.secs) - 1
	if given(op.Section) {
		var err error
		if si, err = d.section(op.Section); err != nil {
			return 0, 0, err
		}
	}
	if si < 0 {
		return 0, 0, errors.New("the template has no section: add one with section_add first")
	}
	return si, len(d.secs[si].fields), nil
}

func (d *formDoc) insert(si, fi int, f map[string]json.RawMessage) {
	fields := d.secs[si].fields
	fields = append(fields, nil)
	copy(fields[fi+1:], fields[fi:])
	fields[fi] = f
	d.secs[si].fields = fields
}

// newFieldID is an id like the editor's own ("item_<ms><random>").
func newFieldID() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1e6))
	if err != nil {
		n = big.NewInt(0)
	}
	return fmt.Sprintf("item_%d%06d", time.Now().UnixMilli(), n.Int64())
}

// checkField validates what the op changes in a field, so the error names the property, what it
// got and what it takes. Values the op does not touch stay as stored, legacy ones included; a new
// class re-checks the type and options it keeps. where is "field" or "set" for the error path.
func checkField(f, given map[string]json.RawMessage, where string) error {
	for k := range given {
		if !contains(fieldKeys, k) {
			return fmt.Errorf("%s.%s is not a field property; allowed: %s", where, k, strings.Join(fieldKeys, ", "))
		}
	}
	changed := func(k string) bool { _, ok := given[k]; return ok }
	var id, class string
	if changed("id") && (json.Unmarshal(f["id"], &id) != nil || strings.TrimSpace(id) == "" || len(id) > 255 || badFieldID.MatchString(id)) {
		return fmt.Errorf("%s.id must be a non-empty string up to 255 characters without '.', '[' or ']'", where)
	}
	_ = json.Unmarshal(f["class"], &class)
	if changed("class") && !contains(fieldClasses, class) {
		return fmt.Errorf("%s.class %s is not a field class; allowed: %s", where, shown(f["class"]), strings.Join(fieldClasses, ", "))
	}
	if changed("visibility") {
		if err := enumProp(f, "visibility", fieldVisibility, where); err != nil {
			return err
		}
	}
	// A value depends on the class: a new class re-checks the type and options the field keeps.
	if class == "edit" && (changed("type") || changed("class")) {
		if err := enumProp(f, "type", editTypes, where); err != nil {
			return err
		}
	}
	for _, k := range []string{"title", "description", "regexp", "errorMsg", "align", "row"} {
		if raw := f[k]; changed(k) && !isJSONString(raw) {
			return fmt.Errorf("%s.%s must be a string, got %s", where, k, shown(raw))
		}
	}
	if raw := f["required"]; changed("required") && string(raw) != "true" && string(raw) != "false" {
		return fmt.Errorf("%s.required must be true or false, got %s", where, shown(raw))
	}
	if raw := f["extra"]; changed("extra") && !isJSONString(raw) && !isJSONObject(raw) {
		return fmt.Errorf("%s.extra must be an object (optionsSource, multiline, time, minDate, ...), got %s", where, shown(raw))
	}
	if changed("options") || changed("class") {
		return checkOptions(f, class, where)
	}
	return nil
}

func checkOptions(f map[string]json.RawMessage, class, where string) error {
	raw, ok := f["options"]
	if !ok {
		return nil
	}
	if !contains(optionClasses, class) {
		return fmt.Errorf("%s.options belongs to a %s field, not %s", where, strings.Join(optionClasses, "/"), class)
	}
	var opts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &opts); err != nil {
		return fmt.Errorf("%s.options must be a list of {title, value, color?}", where)
	}
	seen := map[string]int{}
	for i, o := range opts {
		if !isJSONString(o["title"]) {
			return fmt.Errorf("%s.options[%d].title must be a string", where, i)
		}
		v, has := o["value"]
		if !has {
			return fmt.Errorf("%s.options[%d].value is required (the value stored in actor data)", where, i)
		}
		if j, dup := seen[string(v)]; dup {
			return fmt.Errorf("%s.options[%d].value %s repeats options[%d]: values must be unique", where, i, shown(v), j)
		}
		seen[string(v)] = i
	}
	return nil
}

func enumProp(f map[string]json.RawMessage, key string, allowed []string, where string) error {
	raw, ok := f[key]
	if !ok {
		return nil
	}
	var v string
	if json.Unmarshal(raw, &v) != nil || !contains(allowed, v) {
		return fmt.Errorf("%s.%s %s is not allowed; allowed: %s", where, key, shown(raw), strings.Join(allowed, ", "))
	}
	return nil
}

func isJSONString(raw json.RawMessage) bool { return len(raw) > 0 && raw[0] == '"' }
func isJSONObject(raw json.RawMessage) bool { return len(raw) > 0 && raw[0] == '{' }

// shown is a value for an error message, cut short.
func shown(raw json.RawMessage) string {
	s := string(raw)
	if s == "" {
		return "(missing)"
	}
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}
