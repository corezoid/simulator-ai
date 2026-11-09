package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/simfake"
)

// The budgets of a large Smart Form build: what the Console agent's host leaves of one tool answer,
// and what a step of the build may cost the context.
const (
	hostAnswerLimit = 50_000 // a longer answer is cut by the host
	patchArgsBudget = 8_000  // one section written: well under one model call's output
	writeAnswer     = 700    // a write answers ids and hashes, never sources
	outlineBudget   = 3_000  // a 50,000-character page as an outline two levels deep
	renderBudget    = 400    // a render says the page works; it is not the page
)

const (
	scalePages    = 10
	scaleSections = 8
	scaleRows     = 22
)

type scaleFiles struct {
	Files []struct {
		ID         int64           `json:"id"`
		Path       string          `json:"path"`
		ObjType    string          `json:"objType"`
		SourceHash string          `json:"sourceHash"`
		Source     *string         `json:"source"`
		Next       int             `json:"next"`
		Outline    string          `json:"outline"`
		Node       json.RawMessage `json:"node"`
	} `json:"files"`
	Note string `json:"note"`
}

// scaleRun drives the tools and keeps the size of everything they answered.
type scaleRun struct {
	t       *testing.T
	call    func(tool, args string) (string, bool)
	fake    *simfake.Server
	appID   string
	answers int
}

func (r *scaleRun) ok(tool, args string, budget int) string {
	r.t.Helper()
	reply, failed := r.call(tool, args)
	if failed {
		r.t.Fatalf("%s: %s", tool, reply)
	}
	if len(reply) > budget {
		r.t.Fatalf("%s answered %d bytes, budget %d: %.300s", tool, len(reply), budget, reply)
	}
	r.answers += len(reply)
	return reply
}

func (r *scaleRun) refused(tool, args string, budget int, wants ...string) string {
	r.t.Helper()
	reply, failed := r.call(tool, args)
	if !failed || len(reply) > budget {
		r.t.Fatalf("%s: failed %v, %d bytes (budget %d): %.400s", tool, failed, len(reply), budget, reply)
	}
	for _, want := range wants {
		if !strings.Contains(reply, want) {
			r.t.Fatalf("%s: no %q in %s", tool, want, reply)
		}
	}
	return reply
}

func (r *scaleRun) files(args string, budget int) scaleFiles {
	r.t.Helper()
	var out scaleFiles
	if err := json.Unmarshal([]byte(r.ok("smart_form_files_get", `{"appId":"`+r.appID+`"`+args+`}`, budget)), &out); err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *scaleRun) entry(path string) (int64, string) {
	r.t.Helper()
	for _, f := range r.files("", 12_000).Files {
		if f.Path == path {
			return f.ID, f.SourceHash
		}
	}
	r.t.Fatal("no file ", path)
	return 0, ""
}

func pageName(p int) string {
	if p == 0 {
		return "index"
	}
	return fmt.Sprintf("p%d", p)
}

// scaleSkeleton is a page as its first write: the root and its empty sections.
func scaleSkeleton(page string) string {
	var sections []string
	for k := range scaleSections {
		sections = append(sections, fmt.Sprintf(`{"id":"%s-s%d","type":"Stack","props":{"title":"Section %d"},"children":[]}`, page, k, k))
	}
	return fmt.Sprintf(`{"schemaVersion":"2.0","id":%q,"tree":{"id":"root","type":"Stack","children":[%s]}}`, page, strings.Join(sections, ","))
}

// scaleSection is what one patch adds to a section: rows of a label and an input, and an instance
// of the shared definition (not on the last page: a $ref may hold a submit, so a page with one is
// never pending).
func scaleSection(page string, k int) string {
	var rows []string
	for i := range scaleRows {
		id := fmt.Sprintf("%s-s%d-f%d", page, k, i)
		rows = append(rows, fmt.Sprintf(`{"id":"%s-row","type":"Stack","props":{"direction":"horizontal","size":"md"},"children":[{"id":"%s-label","type":"Label","props":{"text":"Field %d of section %d"}},{"id":"%s","type":"TextInput","props":{"label":"Field %d","placeholder":"Type the value of field %d","value":"{{ vm.%s }}"}}]}`, id, id, i, k, id, i, i, strings.ReplaceAll(id, "-", "_")))
	}
	if page != pageName(scalePages-1) {
		rows = append(rows, fmt.Sprintf(`{"id":"%s-s%d-note","$ref":"#/definitions/note"}`, page, k))
	}
	return `[` + strings.Join(rows, ",") + `]`
}

// A ten-page Smart Form with pages of 40,000-60,000 characters is built from skeletons and section
// patches, read by outline and node, guarded against a person's concurrent save, rendered and
// released, and no answer on the way is long enough for the host to cut.
func TestSmartFormScale(t *testing.T) {
	call, fake := fakeConsole(t)
	r := &scaleRun{t: t, call: call, fake: fake}
	var created struct {
		AppID string `json:"appId"`
	}
	if err := json.Unmarshal([]byte(r.ok("smart_form_create", `{"ref":"big_app","title":"Big"}`, writeAnswer)), &created); err != nil {
		t.Fatal(err)
	}
	r.appID = created.AppID
	root, _ := r.entry("/")
	pages, _ := r.entry("pages")

	// The plan first: every page's folder and the shared definition, then one skeleton per page.
	folders := []string{fmt.Sprintf(`{"folderId":%d,"title":"definitions"}`, root)}
	for p := 1; p < scalePages; p++ {
		folders = append(folders, fmt.Sprintf(`{"folderId":%d,"title":%q}`, pages, pageName(p)))
	}
	r.ok("smart_form_folders_create", `{"actorId":"`+r.appID+`","items":[`+strings.Join(folders, ",")+`]}`, 2_000)
	definitions, _ := r.entry("definitions")
	note, _ := json.Marshal(`{"id":"note","type":"Label","props":{"text":"Shared note"}}`)
	items := []string{fmt.Sprintf(`{"folderId":%d,"title":"note","type":"application/json","source":%s}`, definitions, note)}
	for p := 1; p < scalePages; p++ {
		folder, _ := r.entry("pages/" + pageName(p))
		source, _ := json.Marshal(scaleSkeleton(pageName(p)))
		items = append(items, fmt.Sprintf(`{"folderId":%d,"title":"config","type":"application/json","source":%s}`, folder, source))
	}
	r.ok("smart_form_files_create", `{"appId":"`+r.appID+`","items":[`+strings.Join(items, ",")+`]}`, 2_500)
	index, hash := r.entry("pages/index/config")
	source, _ := json.Marshal(scaleSkeleton("index"))
	r.refused("smart_form_files_update", fmt.Sprintf(`{"appId":%q,"items":[{"id":%d,"title":"config","type":"application/json","source":%s}]}`, r.appID, index, source), 300, "ifMatchHash required")
	r.ok("smart_form_files_update", fmt.Sprintf(`{"appId":%q,"items":[{"id":%d,"title":"config","type":"application/json","source":%s,"ifMatchHash":%q}]}`, r.appID, index, source, hash), writeAnswer)

	// One page per step, one section per patch; each page is verified by a render, not a read.
	for p := range scalePages {
		page := pageName(p)
		for k := range scaleSections {
			args := fmt.Sprintf(`{"appId":%q,"path":"pages/%s/config","nodeId":"%s-s%d","ops":[{"op":"add","path":"/children","value":%s}]}`, r.appID, page, page, k, scaleSection(page, k))
			if len(args) > patchArgsBudget {
				t.Fatalf("a section patch is %d bytes of arguments", len(args))
			}
			r.ok("smart_form_page_patch", args, writeAnswer)
		}
		var rendered struct {
			OK    bool `json:"ok"`
			Nodes int  `json:"nodes"`
			Bytes int  `json:"bytes"`
		}
		if err := json.Unmarshal([]byte(r.ok("smart_form_page_render", fmt.Sprintf(`{"ref":"big_app","envTitle":"develop","page":%q}`, page), renderBudget)), &rendered); err != nil {
			t.Fatal(err)
		}
		want := 1 + scaleSections*(2+3*scaleRows)
		if p == scalePages-1 {
			want -= scaleSections
		}
		if !rendered.OK || rendered.Nodes != want || rendered.Bytes < 40_000 {
			t.Fatalf("render of %s: %+v, want %d nodes", page, rendered, want)
		}
	}
	for path, source := range fake.AppFiles(r.appID, false) {
		if strings.HasSuffix(path, "/config") && (len(source) < 40_000 || len(source) > 60_000) {
			t.Fatalf("%s is %d characters, want a page of 40,000-60,000", path, len(source))
		}
	}
	if r.answers > 60_000 {
		t.Fatalf("the build answered %d bytes in all", r.answers)
	}
	t.Logf("build of %d pages: %d bytes of answers", scalePages, r.answers)

	r.reads()
	r.conflict()
	r.release()
}

// reads is every way a page comes back: whole (cut, never over the host's limit), as an outline, as
// one node, and from an offset on.
func (r *scaleRun) reads() {
	t := r.t
	t.Helper()
	whole := r.files(`,"paths":["pages/p3/config"]`, hostAnswerLimit)
	if f := whole.Files[0]; f.Source == nil || f.Next == 0 || !strings.Contains(whole.Note, "outline") {
		t.Fatalf("a page over the budget must be cut with a note: next %d, note %q", f.Next, whole.Note)
	}
	rest := r.files(fmt.Sprintf(`,"paths":["pages/p3/config"],"offset":%d`, whole.Files[0].Next), hostAnswerLimit)
	if got := *whole.Files[0].Source + *rest.Files[0].Source; got != r.fake.AppFiles(r.appID, false)["pages/p3/config"] || rest.Files[0].Next != 0 {
		t.Fatal("the cut source and its rest are not the file")
	}
	all := r.files(`,"withSource":true`, hostAnswerLimit)
	if !strings.Contains(all.Note, "files left without source") {
		t.Fatalf("every source of a large form must not fit: %q", all.Note)
	}

	outline := r.files(`,"paths":["pages/p3/config"],"outline":2`, outlineBudget).Files[0].Outline
	nodes := 1 + scaleSections*(2+3*scaleRows)
	for _, want := range []string{fmt.Sprintf("%d nodes\nStack#root\n Stack#p3-s0 title=\"Section 0\" +%d\n", nodes, 1+3*scaleRows), "Stack#p3-s7"} {
		if !strings.Contains(outline, want) {
			t.Fatalf("outline lacks %q:\n%s", want, outline)
		}
	}
	section := r.files(`,"paths":["pages/p3/config"],"outline":3,"nodeId":"p3-s2"`, 6_000).Files[0].Outline
	if !strings.Contains(section, " Stack#p3-s2-f5-row\n  Label#p3-s2-f5-label text=\"Field 5 of section 2\"\n  TextInput#p3-s2-f5 label=\"Field 5\"") || !strings.Contains(section, "$definitions/note#p3-s2-note") || strings.Contains(section, "p3-s3") {
		t.Fatalf("outline of one section:\n%s", section)
	}
	app := r.files(`,"outline":1`, 6_000)
	outlined := 0
	for _, f := range app.Files {
		if f.Outline != "" {
			outlined++
		}
	}
	if outlined != scalePages+1 {
		t.Fatalf("the app outline covers %d files, want every page and the definition", outlined)
	}

	node := r.files(`,"paths":["pages/p3/config"],"nodeId":"p3-s2-f5"`, 700).Files[0]
	if !strings.Contains(string(node.Node), `"id":"p3-s2-f5","type":"TextInput"`) || node.Source != nil || len(node.SourceHash) != 64 {
		t.Fatalf("node read: %s", node.Node)
	}
	byPointer := r.files(`,"paths":["pages/p3/config"],"nodeId":"/tree/children/2/children/5/children/1"`, 700).Files[0]
	if string(byPointer.Node) != string(node.Node) {
		t.Fatalf("the pointer names another node: %s", byPointer.Node)
	}
	t.Logf("a %d-character page: cut read %d, outline(2) %d, section outline(3) %d, app outline(1) %d, node %d bytes", len(r.fake.AppFiles(r.appID, false)["pages/p3/config"]),
		len(*whole.Files[0].Source), len(outline), len(section), appOutline(app), len(node.Node))
	r.refused("smart_form_files_get", `{"appId":"`+r.appID+`","paths":["pages/p3/config"],"nodeId":"root"}`, 300, "outline 2")
	r.refused("smart_form_files_get", `{"appId":"`+r.appID+`","paths":["pages"],"nodeId":"root"}`, 300, "one file")
}

// conflict is a person saving in the editor between the agent's read and its write.
func (r *scaleRun) conflict() {
	t := r.t
	t.Helper()
	id, read := r.entry("pages/p5/config")
	mine := r.fake.AppFiles(r.appID, false)["pages/p5/config"]
	r.fake.EditFile(r.appID, "pages/p5/config", func(source string) string {
		return strings.Replace(source, `"children":[`, `"children":[{"id":"human-note","type":"Label","props":{"text":"Added in the editor"}},`, 1)
	})
	_, now := r.entry("pages/p5/config")
	body, _ := json.Marshal(mine)
	update := fmt.Sprintf(`{"appId":%q,"items":[{"id":%d,"title":"config","type":"application/json","source":%s,"ifMatchHash":%q}]}`, r.appID, id, body, read)
	r.refused("smart_form_files_update", update, 600, "sourceHash is now "+now, "nodes yours lacks: human-note")
	patch := `{"appId":"` + r.appID + `","path":"pages/p5/config","nodeId":"p5-s0-f0","ifMatchHash":"%s","ops":[{"op":"replace","path":"/props/label","value":"Renamed"}]}`
	r.refused("smart_form_page_patch", fmt.Sprintf(patch, read), 300, now)
	if got := r.fake.AppFiles(r.appID, false)["pages/p5/config"]; !strings.Contains(got, "human-note") || strings.Contains(got, "Renamed") {
		t.Fatal("a refused write changed the file")
	}
	r.ok("smart_form_page_patch", fmt.Sprintf(patch, now), writeAnswer)
	if got := r.fake.AppFiles(r.appID, false)["pages/p5/config"]; !strings.Contains(got, "human-note") || !strings.Contains(got, "Renamed") {
		t.Fatal("the patch on the current revision lost the person's node or its own change")
	}
}

// release reads the page configs one by one for its check, refuses a page that sends nothing and
// ships the rest.
func (r *scaleRun) release() {
	t := r.t
	t.Helper()
	form := `{"appId":"` + r.appID + `","path":"pages/p9/config","nodeId":"p9-s0","ops":[{"op":"add","path":"/children/-","value":{"id":"ask","type":"Form","children":[{"id":"ask-name","type":"TextInput"}]}}]}`
	if reply := r.ok("smart_form_page_patch", form, writeAnswer); !strings.Contains(reply, "pending") {
		t.Fatal("a Form without its submit is pending:", reply)
	}
	r.refused("smart_form_release", `{"appId":"`+r.appID+`"}`, 600, `pages/p9/config: Form "ask" has no button with form.submit`)
	submit := `{"appId":"` + r.appID + `","path":"pages/p9/config","nodeId":"ask","ops":[{"op":"add","path":"/children/-","value":{"id":"ask-send","type":"Button","actions":{"onClick":[{"type":"form.submit","formId":"ask"}]}}}]}`
	r.ok("smart_form_page_patch", submit, writeAnswer)
	before := r.fake.Count("app_file/get")
	for _, c := range r.fake.Calls() {
		if c.Method == "application/struct" && c.Bytes > 400 {
			t.Fatalf("application/struct request of %d bytes", c.Bytes)
		}
	}
	if reply := r.ok("smart_form_release", `{"appId":"`+r.appID+`"}`, writeAnswer); !strings.Contains(reply, `"ready":true,"marker":"appId: `+r.appID+`"`) {
		t.Fatal(reply)
	}
	if got := r.fake.Count("app_file/get") - before; got != scalePages {
		t.Fatalf("the release check read %d files, want the %d page configs", got, scalePages)
	}
	served := r.fake.AppFiles(r.appID, true)
	if len(served["pages/p9/config"]) < 40_000 || !strings.Contains(served["pages/p5/config"], "human-note") {
		t.Fatal("production does not serve the released pages")
	}
	if reply := r.ok("smart_form_page_render", `{"ref":"big_app","envTitle":"production","page":"p9","full":true}`, 80_000); len(reply) < 40_000 || !strings.Contains(reply, `"tree"`) {
		t.Fatalf("full render is %d bytes", len(reply))
	}
}

func appOutline(app scaleFiles) int {
	n := 0
	for _, f := range app.Files {
		n += len(f.Outline)
	}
	return n
}
