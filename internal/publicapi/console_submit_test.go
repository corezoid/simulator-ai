package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestCreateDataFormBuildsAWiredPage(t *testing.T) {
	var sent map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		sent = q.Params
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":77}}`, q.ID)
	}))
	defer api.Close()
	c, err := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Bearer u"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.CreateDataForm(context.Background(), DataFormInput{Title: "Leave request", SubmitLabel: "Send", SuccessMessage: "Saved",
		Fields: []DataField{{Name: "start", Label: "Start", Type: "date", Required: true}, {Name: "days", Label: "Days", Type: "number"}, {Name: "kind", Label: "Kind", Type: "select", Options: []string{"Paid", "Unpaid"}}, {Name: "comment", Label: "Comment"}}})
	if err != nil || out.FormID != 77 {
		t.Fatal(out, err)
	}
	fields, _ := json.Marshal(sent["sections"])
	for _, want := range []string{`"class":"calendar","id":"start"`, `"class":"edit","id":"days","title":"Days","type":"float"`, `"class":"select","id":"kind"`, `"options":[{"title":"Paid","value":"Paid"}`, `"class":"edit","id":"comment"`} {
		if !strings.Contains(string(fields), want) {
			t.Errorf("template fields lack %s: %s", want, fields)
		}
	}
	for _, want := range []string{`"type":"Form"`, `"valueKey":"request"`, `"props":{"text":"Start"}`, `"path":"request.start","type":"state","value":"{{ {\"startDate\": $event.startDate, \"endDate\": $event.endDate} }}"`, `"props":{"mode":"single","value":"{{ vm.request.start }}"}`, `"props":{"error":true,"text":"{{ vm.errors.request.start }}"}`, `"label":"Days","required":false,"type":"float","value":"{{ vm.request.days }}"`,
		`"simapiExtraParams":{"accId":"ws","formData":"{{ $merge([{}, $sift(vm.request, function($v){ $v != \"\" and $v != null })]) }}","formId":77,"successMessage":"Saved"},"simapiMethod":"form/submit","type":"form.submit","via":"simapi"`, `"title":"Send"`} {
		if !strings.Contains(out.Page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if wired, err := wireSubmits(out.Page, "ws"); err != nil || wired != out.Page {
		t.Fatal("the generated page fails its own wiring check or is rewritten:", err)
	}
	var page bytes.Buffer
	if err = json.Indent(&page, []byte(out.Page), "", "  "); err != nil {
		t.Fatal(err)
	}
	golden := "testdata/console/data-form-page.json"
	if *update {
		_ = os.MkdirAll("testdata/console", 0o755)
		_ = os.WriteFile(golden, append(page.Bytes(), '\n'), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil || strings.TrimSpace(string(want)) != page.String() {
		t.Errorf("page differs from %s (go test -run DataForm -update to accept):\n%s", golden, page.String())
	}
}

// Without a text edit field form/submit would name the actor "<template> <date>"; the page names it.
func TestDataPageTitlesActorsWithoutTextField(t *testing.T) {
	params := func(title string, fields ...DataField) map[string]any {
		page, _ := json.Marshal(dataPage(DataFormInput{Title: title, Fields: fields}, 7, "ws"))
		var submit struct {
			Params map[string]any `json:"simapiExtraParams"`
		}
		i := strings.Index(string(page), `{"formId":"request"`)
		if i < 0 || json.NewDecoder(strings.NewReader(string(page[i:]))).Decode(&submit) != nil {
			t.Fatal("no form.submit in", string(page))
		}
		return submit.Params
	}
	date, days := DataField{Name: "start", Label: "Start", Type: "date"}, DataField{Name: "days", Label: "Days", Type: "number"}
	if got := params("Leave", date, days)["title"]; got != "Leave" {
		t.Errorf("title = %v, want the template name", got)
	}
	if got, ok := params("Leave", date, DataField{Name: "note", Label: "Note"})["title"]; ok {
		t.Errorf("title = %v, want none: form/submit takes it from the text field", got)
	}
	// form/submit refuses a title over 255 characters, and the renderer would evaluate {{ }}.
	if got, _ := params(strings.Repeat("я", 300), date)["title"].(string); got != strings.Repeat("я", 255) {
		t.Errorf("a long title is sent as %d characters, want 255", len([]rune(got)))
	}
	if got, ok := params("Q3 {{draft}}", date)["title"]; ok {
		t.Errorf("title = %v, want none for a title the renderer would evaluate", got)
	}
}

func TestWireSubmits(t *testing.T) {
	d7 := `{"schemaVersion":"2.0","id":"index","tree":{"id":"root","type":"Stack","children":[{"id":"c","type":"TextInput","props":{"label":"Comment"}},{"id":"send","type":"Button","props":{"title":"Send"}}]}}`
	formOnly := `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"id":"c","type":"TextInput"}]}}`
	noTemplate := `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit"}]}}]}}`
	good := `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"if","cond":"true","then":[{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit","simapiExtraParams":{"accId":"other","formId":5}}]}]}}]}}`
	for name, tc := range map[string]struct{ source, err string }{
		"D7: inputs and a button, no Form": {d7, "no Form node"},
		"Form without a submit button":     {formOnly, `Form "request" has no button with form.submit`},
		"no data template":                 {noTemplate, "needs simapiExtraParams.formId"},
		"a stale one of two submits":       {`{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","props":{"valueKey":"leave"},"children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit","simapiExtraParams":{"accId":"ws","formId":5,"formData":"{{ $merge([{}, $sift(vm.request, function($v){ $v != \"\" })]) }}"}},{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit","simapiExtraParams":{"accId":"ws","formId":5,"formData":"{{ $merge([{}, $sift(vm.leave, function($v){ $v != \"\" })]) }}"}}]}}]}}`, "reads vm.request, not vm.leave"},
		"a Form only validated":            {`{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","props":{"valueKey":"leave"},"children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.validate","form":"request"},{"type":"if","cond":"vm.valid.leave = true","then":[{"type":"notify","title":"ok"}]}]}}]}}`, "no action reads vm.leave"},
		"filter of a renamed Form":         {`{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","props":{"valueKey":"leave"},"children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit","simapiExtraParams":{"accId":"ws","formId":5,"formData":"{{ $merge([{}, $sift(vm.request, function($v){ $v != \"\" })]) }}"}}]}}]}}`, "reads vm.request, not vm.leave"},
	} {
		if _, err := wireSubmits(tc.source, "ws"); err == nil || !strings.Contains(err.Error(), tc.err) || !strings.Contains(err.Error(), "smart_form_data_form_create") {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, legit := range map[string]string{
		"corezoid submit":           `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"request","via":"corezoid"}]}}]}}`,
		"search with navigate":      `{"schemaVersion":"2.0","id":"index","tree":{"id":"r","type":"Stack","children":[{"id":"q","type":"TextInput"},{"id":"go","type":"Button","actions":{"onClick":[{"type":"navigate","to":"results"}]}}]}}`,
		"submit in a fragment":      `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"$ref":"#/submitButton"}]}}`,
		"spaced filter of its Form": `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","props":{"valueKey":"leave"},"children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit","simapiExtraParams":{"accId":"ws","formId":5,"formData":"{{ $merge([{}, $sift(vm.leave , function($v){ $v != \"\" })]) }}"}}]}}]}}`,
		"Form read by simapi.call":  `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","props":{"valueKey":"leave"},"children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"simapi.call","method":"actor/create","params":{"data":"{{ vm.leave }}"}}]}}]}}`,
		"Form read by bus.publish":  `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","props":{"valueKey":"leave"},"children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"bus.publish","topic":"leave","payload":"{{ vm.leave.start }}"}]}}]}}`,
		"expression template":       `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"id":"b","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"request","via":"simapi","simapiMethod":"form/submit","simapiExtraParams":{"accId":"ws","formId":"{{ vm.templateId }}"}}]}}]}}`,
	} {
		out, err := wireSubmits(legit, "ws")
		if err != nil || out != legit {
			t.Errorf("%s: refused or rewritten: %v", name, err)
		}
	}
	wired, err := wireSubmits(good, "ws")
	if err != nil || !strings.Contains(wired, `"accId": "ws"`) || strings.Contains(wired, `"other"`) {
		t.Fatal("accId must be the host workspace:", wired, err)
	}
	// A template id above 2^53 survives the rewrite of accId.
	big := strings.Replace(good, `"formId":5`, `"formId":9007199254740993`, 1)
	if wired, err := wireSubmits(big, "ws"); err != nil || !strings.Contains(wired, `"formId": 9007199254740993`) {
		t.Fatal("a large template id is rounded:", wired, err)
	}
	for _, plain := range []string{`{"startPage":"index"}`, "a { color: red }", `{"schemaVersion":"2.0","id":"x","tree":{"id":"r","type":"Stack","children":[{"id":"l","type":"Label"}]}}`, `{"schemaVersion":"2.0","id":"x","tree":{}} trailing`} {
		if out, err := wireSubmits(plain, "ws"); err != nil || out != plain {
			t.Errorf("%q changed or refused: %v", plain, err)
		}
	}
}
