package publicapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
)

// siftedKey finds the vm key a generated formData filter reads.
var siftedKey = regexp.MustCompile(`\$sift\(\s*vm\.\s*([^\s,()]+)\s*,`)

// pageFacts is what wireSubmits learns from one walk of a page config.
type pageFacts struct {
	forms       map[string]string // Form id -> the vm key holding its values
	submitted   map[string]bool   // Form ids a form.submit names
	filters     [][2]string       // Form id and the vm key a form/submit's formData filter reads
	reads       []string          // strings of actions that send or pass on data, for the vm keys they read
	inputs      int
	idleButtons int
	refs        bool // a $ref fragment may hold the submit
	changed     bool // an accId or appId was set
	problems    []string
}

// wireSubmits checks a page config before it is stored and returns the source to store. It refuses
// what made a Console form send nothing (acceptance D7): a Form node nothing consumes, and a button
// without any action beside inputs on a page without a Form. A Form is consumed by a form.submit that
// names it or by an action passing on vm.<its value key> (simapi.call, widget.call, bus.publish,
// navigate, copy), wherever it sits (an actions key or a page binding). A form.submit
// to form/submit must name its data template; its accId is set to the host workspace and its appId to
// the Smart Form written (form/submit lets that form's audience submit). Other
// submits (via corezoid, other methods) are the page author's and pass as they are. A generated
// formData filter must read the values of the Form its submit names, or a renamed form would send
// nothing. A source that is not a page config, or needs no accId, is stored byte for byte.
func wireSubmits(source, workspace, appID string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(source))
	dec.UseNumber() // an id above 2^53 is written back as it came
	var doc any
	if dec.Decode(&doc) != nil || dec.Decode(new(any)) != io.EOF {
		return source, nil
	}
	if page, ok := doc.(map[string]any); !ok || page["tree"] == nil || page["schemaVersion"] == nil {
		return source, nil
	}
	facts := pageFacts{forms: map[string]string{}, submitted: map[string]bool{}}
	facts.collect(doc, wiring{workspace, appID}, false)
	if problems := facts.judge(); len(problems) > 0 {
		return "", errors.New("page submit wiring: " + strings.Join(problems, "; ") + ". Create the data template and a ready page with smart_form_data_form_create")
	}
	if !facts.changed {
		return source, nil
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(doc)
	return strings.TrimSuffix(out.String(), "\n"), err
}

// wiring is what a form/submit of the written page is bound to.
type wiring struct{ workspace, appID string }

// consumers are the client actions that send or pass on what they read.
var consumers = map[string]bool{"simapi.call": true, "widget.call": true, "bus.publish": true, "navigate.local": true, "navigate.route": true, "copy": true}

// followUps are the keys of a consumer action that hold further actions or a message, not its payload.
var followUps = map[string]bool{"onError": true, "onSuccess": true, "then": true, "else": true, "notify": true}

// collect walks the page; inConsumer marks the subtree of a consumer action.
func (f *pageFacts) collect(v any, w wiring, inConsumer bool) {
	switch t := v.(type) {
	case string:
		if inConsumer {
			f.reads = append(f.reads, t)
		}
	case []any:
		for _, x := range t {
			f.collect(x, w, inConsumer)
		}
	case map[string]any:
		if _, ok := t["$ref"]; ok {
			f.refs = true
		}
		switch t["type"] {
		case "TextInput", "DatePicker", "Select", "Checkbox", "Radio", "Toggle", "Slider", "Upload":
			f.inputs++
		case "Button":
			if actions, _ := t["actions"].(map[string]any); len(actions) == 0 {
				f.idleButtons++
			}
		case "Form":
			if id, ok := t["id"].(string); ok {
				props, _ := t["props"].(map[string]any)
				key, named := props["valueKey"].(string)
				if !named {
					key = id // the renderer's valueKey ?? id
				}
				f.forms[id] = key
			}
		case "form.submit":
			f.submit(t, w)
		}
		kind, _ := t["type"].(string) // a props schema may hold "type": [...]
		inConsumer = inConsumer || consumers[kind]
		for k, x := range t {
			// A consumer's own follow-up chains (onError, a copy's notify) do not send what they read.
			f.collect(x, w, inConsumer && !followUps[k])
		}
	}
}

func (f *pageFacts) submit(t map[string]any, w wiring) {
	id, _ := t["formId"].(string)
	f.submitted[id] = true
	if t["via"] != "simapi" || t["simapiMethod"] != "form/submit" {
		return
	}
	params, _ := t["simapiExtraParams"].(map[string]any)
	if params == nil || !positive(params["formId"]) && !expression(params["formId"]) {
		f.problems = append(f.problems, fmt.Sprintf("form.submit of %q needs simapiExtraParams.formId, the data template id", id))
		return
	}
	if params["accId"] != w.workspace {
		params["accId"], f.changed = w.workspace, true
	}
	if w.appID != "" && params["appId"] != w.appID {
		params["appId"], f.changed = w.appID, true
	}
	if filter, _ := params["formData"].(string); siftedKey.MatchString(filter) {
		f.filters = append(f.filters, [2]string{id, siftedKey.FindStringSubmatch(filter)[1]})
	}
}

// judge lists what would make the page send nothing.
func (f *pageFacts) judge() []string {
	problems := f.problems
	if f.inputs > 0 && f.idleButtons > 0 && len(f.forms) == 0 {
		problems = append(problems, "the page has inputs and a button with no actions but no Form node, so what is typed would not be sent (give a decorative button an action, or wrap the inputs in a Form with a form.submit button)")
	}
	for id, key := range f.forms {
		if !f.submitted[id] && !f.refs && !f.read(key) {
			problems = append(problems, fmt.Sprintf("Form %q has no button with form.submit and no action sends vm.%s", id, key))
		}
	}
	for _, s := range f.filters {
		if key, ok := f.forms[s[0]]; ok && s[1] != key {
			problems = append(problems, fmt.Sprintf("simapiExtraParams.formData of a form.submit of %q reads vm.%s, not vm.%s, the values of that Form", s[0], s[1], key))
		}
	}
	return problems
}

// read says a consumer action reads vm.<key> (not vm.valid.<key> or vm.<key>x).
func (f *pageFacts) read(key string) bool {
	if key == "" {
		return false
	}
	reads := regexp.MustCompile(`(^|[^\w.$])vm\.\s*` + regexp.QuoteMeta(key) + `($|[^\w])`)
	for _, s := range f.reads {
		if reads.MatchString(s) {
			return true
		}
	}
	return false
}

func expression(v any) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, "{{")
}

func positive(v any) bool {
	n, ok := v.(json.Number)
	if i, err := n.Int64(); ok && err == nil {
		return i >= 1
	}
	f, err := n.Float64() // 5.0 or 5e0 is a whole number too
	return ok && err == nil && f >= 1 && f <= math.MaxInt64 && f == math.Trunc(f)
}
