package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/simfake"
)

// kbClient is a console client over a fake workspace with a contract template (an amount and a
// status select), a counterparty template that opens a Money account on each actor, a project
// template, two account names and a currency, and an existing project.
func kbClient(t *testing.T) (*Client, *simfake.Server, string) {
	t.Helper()
	f := simfake.New()
	t.Cleanup(f.Close)
	f.Set(func(s *simfake.Server) {
		s.KB.Strict = true
		s.KB.Forms[12] = simfake.Form{Title: "Contract", Body: `{"sections":[{"content":[{"id":"amount","class":"edit","title":"Amount","type":"float"},` +
			`{"id":"status","class":"select","title":"Status","options":[{"value":"signed","title":"Signed"},{"value":"draft","title":"Draft"}]},{"id":"note","class":"label","title":"Note"}]}]}`}
		s.KB.Forms[13] = simfake.Form{Title: "Counterparty", Body: `{"sections":[{"content":[{"id":"code","class":"edit","title":"Code"}]}]}`}
		s.KB.Forms[14] = simfake.Form{Title: "Project", Body: `{"sections":[]}`}
		s.KB.Names["Contract sum"], s.KB.Names["Money"] = "n-sum", "n-money"
		s.KB.Currencies["UAH"] = 980
		s.KB.FormAccounts[13] = []simfake.FormAccount{{NameID: "n-money", CurrencyID: 980, Type: "fact"}}
		s.KB.SystemForms["ActorFilters"] = 20
	})
	c, err := New(Config{Origin: f.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator user-jwt"})
	if err != nil {
		t.Fatal(err)
	}
	return c, f, f.AddActor(14, "Project X", nil)
}

// kbApply runs a patch given as JSON, as the tool does; failed carries the error text.
func kbApply(t *testing.T, c *Client, patch string) (OntologyResult, string, bool) {
	t.Helper()
	var in OntologyInput
	if err := json.Unmarshal([]byte(patch), &in); err != nil {
		t.Fatal(err, patch)
	}
	out, err := c.ApplyOntology(context.Background(), in, nil)
	if err != nil {
		return out, err.Error(), true
	}
	return out, out.Summary, false
}

func scenario(op, project string, dry bool, extra string) string {
	return fmt.Sprintf(`{"operationId":%q,"dryRun":%t,"actors":[{"key":"project","existing":%q},`+
		`{"key":"acme","form":"Counterparty","title":"kb-test ACME","aliases":["Акме"],"data":{"code":"A-1"}},`+
		`{"key":"c1","formId":12,"title":"kb-test Contract 17","parent":"project","data":{"amount":120000,"status":"Signed"},"source":"attachment:contract.pdf"},`+
		`{"key":"delivery","formId":12,"title":"kb-test Delivery","parent":"project"}],`+
		`"edges":[{"type":"gantt","from":"c1","to":"delivery"}],`+
		`"accounts":[{"actor":"c1","name":"Contract sum","currency":"UAH"},{"actor":"c1","name":"Contract sum","currency":"UAH","accountType":"plan"}],`+
		`"transactions":[{"key":"plan","actor":"c1","account":"Contract sum","currency":"UAH","accountType":"plan","amount":120000}],`+
		`"transfers":[{"key":"pay1","from":[{"actor":"acme","account":"Money","currency":"UAH","amount":60000}],"to":[{"actor":"c1","account":"Contract sum","currency":"UAH","amount":60000}]}]%s}`,
		op, dry, project, extra)
}

func postingStatuses(out OntologyResult) string {
	var parts []string
	for _, a := range out.Actors {
		parts = append(parts, a.Key+"="+a.Status)
	}
	for _, p := range out.Postings {
		parts = append(parts, p.Key+"="+p.Status)
	}
	return strings.Join(parts, " ")
}

// The dry run is strict: an unknown field, an option that is not one, an edge a transfer draws, a
// missing account and an unbalanced transfer are refused with what to do instead.
func TestOntologyApplyRefusals(t *testing.T) {
	c, f, project := kbClient(t)
	f.Set(func(s *simfake.Server) { s.KB.Names["Bonus pool"], s.KB.Unregistered["n-bonus/980"] = "n-bonus", true })
	for _, tc := range []struct{ patch, want string }{
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","formId":12,"title":"C","data":{"stauts":"signed"}}]}`,
			`field "stauts" is not in template "Contract" (12); its fields: amount (Amount, edit float); status (Status, select: signed=Signed | draft=Draft)`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","formId":12,"title":"C","data":{"status":"archived"}}]}`, `"archived" is not one of its options`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","formId":12,"title":"C","data":{"amount":"lots"}}]}`, `takes a number`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"p","existing":"` + project + `"},{"key":"c","formId":12,"title":"C"}],"edges":[{"type":"transfersAmount_n-sum_980","from":"p","to":"c"}]}`, `drawn by a transfer, never created`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"p","existing":"` + project + `"},{"key":"c","formId":12,"title":"C"}],"edges":[{"type":"likes","from":"p","to":"c"}]}`, `edge type "likes" is unknown`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","formId":12,"title":"C"}],"transactions":[{"key":"t","actor":"c","account":"Contract sum","currency":"UAH","amount":5}]}`, `has no fact account Contract sum / UAH: add`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","formId":12,"title":"C"}],"accounts":[{"actor":"c","name":"Bonus","currency":"UAH"}]}`, `account name "Bonus" is not in the workspace`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"a","form":"Counterparty","title":"A"},{"key":"b","form":"Counterparty","title":"B"}],"transfers":[{"key":"x","from":[{"actor":"a","account":"Money","currency":"UAH","amount":5}],"to":[{"actor":"b","account":"Money","currency":"UAH","amount":4}]}]}`, `an exchange is balanced`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","form":"Nope","title":"C"}]}`, `no template titled "Nope"`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","existing":"missing-actor"}],"edges":[]}`, `existing missing-actor is not found or not visible`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"c","formId":12,"title":"C"}],"accounts":[{"actor":"c","name":"Bonus pool","currency":"UAH"}]}`, `account Bonus pool / UAH is not registered as a pair`},
		{`{"operationId":"kb-test-2","dryRun":true,"actors":[{"key":"a","form":"Counterparty","title":"A"},{"key":"b","form":"Counterparty","title":"B"}],"transfers":[{"key":"c","hold":"h","type":"completed"},{"key":"h","type":"authorized","from":[{"actor":"a","account":"Money","currency":"UAH","amount":5}],"to":[{"actor":"b","account":"Money","currency":"UAH","amount":5}]}]}`, `put hold "h" before the transfer that closes it`},
	} {
		_, reply, failed := kbApply(t, c, tc.patch)
		if !failed || !strings.Contains(reply, tc.want) || !strings.Contains(reply, "nothing was written") {
			t.Errorf("%s\n  got %v %s\n  want %s", tc.patch, failed, reply, tc.want)
		}
	}
}

// On a workspace still on the legacy ledger the structure is written and the postings are skipped,
// with a sentence for the person; the dry run says so beforehand.
func TestOntologyApplyLegacyLedger(t *testing.T) {
	c, f, project := kbClient(t)
	f.Set(func(s *simfake.Server) { s.KB.LegacyLedger = true })
	dry := mustApply(t, c, scenario("kb-test-3", project, true, ""))
	if dry.Ledger != "legacy" || postingStatuses(dry) != "project=existing acme=create c1=create delivery=create plan=skipped pay1=skipped" {
		t.Fatalf("dry run: %s ledger %q", postingStatuses(dry), dry.Ledger)
	}
	out := mustApply(t, c, scenario("kb-test-3", project, false, ""))
	if out.Ledger != "legacy" || !out.Done || !strings.Contains(out.Summary, "still made in the web app") || postingStatuses(out) != "project=existing acme=created c1=created delivery=created plan=skipped pay1=skipped" {
		t.Fatalf("apply: %s ledger %q: %s", postingStatuses(out), out.Ledger, out.Summary)
	}
	if _, postings, _ := f.LedgerSnapshot(); len(postings) != 0 {
		t.Fatalf("legacy ledger posted %d", len(postings))
	}
}

// An actor that may exist already (same title, or the name among another actor's aliases) is a
// candidate in the dry run and refused by the apply until the patch names it existing or new.
func TestOntologyApplyRecognition(t *testing.T) {
	c, f, project := kbClient(t)
	acme := f.AddActor(13, "ACME Ltd", map[string]any{"aliases": []any{"Акме"}})
	patch := func(dry bool, extra string) string {
		return fmt.Sprintf(`{"operationId":"kb-test-4","dryRun":%t,"actors":[{"key":"p","formId":14,"title":"project x"%s},{"key":"a","form":"Counterparty","title":"Акме"}]}`, dry, extra)
	}
	dry := mustApply(t, c, patch(true, ""))
	if len(dry.Actors[0].Candidates) != 1 || dry.Actors[0].Candidates[0].ActorID != project || dry.Actors[0].Candidates[0].Reason != "same title" ||
		len(dry.Actors[1].Candidates) != 1 || dry.Actors[1].Candidates[0].ActorID != acme || dry.Actors[1].Candidates[0].Reason != "alias: Акме" {
		t.Fatalf("candidates: %+v", dry.Actors)
	}
	if _, reply, failed := kbApply(t, c, patch(false, "")); !failed || !strings.Contains(reply, "may be the same") {
		t.Fatalf("a likely duplicate was created: %s", reply)
	}
	out, reply, failed := kbApply(t, c, strings.Replace(patch(false, `,"new":true`), `{"key":"a","form":"Counterparty","title":"Акме"}`, `{"key":"a","existing":"`+acme+`"}`, 1))
	if failed || postingStatuses(out) != "p=created a=existing" {
		t.Fatalf("new/existing: %s %s", postingStatuses(out), reply)
	}
}

// A hold and its closing in one patch: step 1 authorized, step 2 completed naming it.
func TestOntologyApplyHold(t *testing.T) {
	c, f, _ := kbClient(t)
	legs := `"from":[{"actor":"a","account":"Money","currency":"UAH","amount":5}],"to":[{"actor":"b","account":"Money","currency":"UAH","amount":5}]`
	out := mustApply(t, c, `{"operationId":"kb-test-6","actors":[{"key":"a","form":"Counterparty","title":"A1"},{"key":"b","form":"Counterparty","title":"B1"}],`+
		`"transfers":[{"key":"h","type":"authorized",`+legs+`},{"key":"close","hold":"h","type":"completed"}]}`)
	if postingStatuses(out) != "a=created b=created h=posted close=posted" {
		t.Fatalf("%s: %s", postingStatuses(out), out.Summary)
	}
	_, postings, transfers := f.LedgerSnapshot()
	if len(transfers) != 1 || transfers[0].Type != "completed" || len(postings) != 4 || postings[2].Ref != "kb-test-6:close:f0" {
		t.Fatalf("%+v %+v", transfers, postings)
	}
	// The close keeps no ref of its own (sim-api closes the hold in place): a repeat finds the hold
	// completed and posts nothing; a repeat asking another outcome is refused, not posted.
	patch := `{"operationId":"kb-test-6","actors":[{"key":"a","form":"Counterparty","title":"A1"},{"key":"b","form":"Counterparty","title":"B1"}],` +
		`"transfers":[{"key":"h","type":"authorized",` + legs + `},{"key":"close","hold":"h","type":"%s"}]}`
	again := mustApply(t, c, fmt.Sprintf(patch, "completed"))
	if postingStatuses(again) != "a=exists b=exists h=exists close=exists" || !again.Done {
		t.Fatalf("%s: %s", postingStatuses(again), again.Summary)
	}
	other := mustApply(t, c, fmt.Sprintf(patch, "canceled"))
	if postingStatuses(other) != "a=exists b=exists h=exists close=failed" || other.Done {
		t.Fatalf("%s: %s", postingStatuses(other), other.Summary)
	}
	if _, after, _ := f.LedgerSnapshot(); len(after) != 4 {
		t.Fatalf("a repeat posted: %+v", after)
	}
}

// A posting declined earlier is not reported as posted on a repeat.
func TestOntologyApplyDeclinedRepeat(t *testing.T) {
	c, f, _ := kbClient(t)
	patch := `{"operationId":"kb-test-13","actors":[{"key":"a","form":"Counterparty","title":"A2"},{"key":"b","form":"Counterparty","title":"B2"}],` +
		`"transfers":[{"key":"t","from":[{"actor":"a","account":"Money","currency":"UAH","amount":5}],"to":[{"actor":"b","account":"Money","currency":"UAH","amount":5}]}]}`
	mustApply(t, c, patch)
	f.Set(func(s *simfake.Server) { s.DeclineTransfer("kb-test-13:t") })
	again := mustApply(t, c, patch)
	if postingStatuses(again) != "a=exists b=exists t=failed" || again.Done {
		t.Fatalf("%s: %s", postingStatuses(again), again.Summary)
	}
}

func mustApply(t *testing.T, c *Client, patch string) OntologyResult {
	t.Helper()
	out, reply, failed := kbApply(t, c, patch)
	if failed {
		t.Fatalf("ontology_apply failed: %s", reply)
	}
	return out
}

// The scenario of the brief, at the client: a dry run writes nothing, the apply writes the structure
// and the postings, and the same patch again writes nothing more.
func TestApplyOntologyScenarioRepeats(t *testing.T) {
	c, f, project := kbClient(t)
	dry := mustApply(t, c, scenario("kb-test-7", project, true, ""))
	if got := postingStatuses(dry); got != "project=existing acme=create c1=create delivery=create plan=plan pay1=plan" || dry.Ledger != "simapi" || dry.Edges != 3 || dry.Accounts != 2 {
		t.Fatalf("dry %s %+v", got, dry)
	}
	out := mustApply(t, c, scenario("kb-test-7", project, false, ""))
	if got := postingStatuses(out); got != "project=existing acme=created c1=created delivery=created plan=posted pay1=posted" || !out.Done {
		t.Fatalf("apply %s: %s %v", got, out.Summary, out.Warnings)
	}
	again := mustApply(t, c, scenario("kb-test-7", project, false, ""))
	if got := postingStatuses(again); got != "project=existing acme=exists c1=exists delivery=exists plan=exists pay1=exists" {
		t.Fatalf("repeat %s", got)
	}
	if accounts, postings, transfers := f.LedgerSnapshot(); len(accounts) != 3 || len(postings) != 3 || len(transfers) != 1 {
		t.Fatalf("ledger %d %d %d", len(accounts), len(postings), len(transfers))
	}
}

// On a layer: new actors are placed and their edges drawn in the batch; an edge between two actors
// on the layer already is drawn after it, once.
func TestApplyOntologyLayerDrawsOnce(t *testing.T) {
	c, f, project := kbClient(t)
	other := f.AddActor(14, "Project Y", nil)
	f.Set(func(s *simfake.Server) {
		s.Nodes = append(s.Nodes, simfake.Node{LaID: 9001, Layer: "L", Actor: project, X: 25, Y: 25}, simfake.Node{LaID: 9002, Layer: "L", Actor: other, X: 125, Y: 25})
	})
	grid := func(_ *GraphBuild, keys []string, _ [][2]string) (map[string]GraphPoint, error) {
		out := map[string]GraphPoint{}
		for i, k := range keys {
			out[k] = GraphPoint{X: 1025 + 100*float64(i), Y: 1025}
		}
		return out, nil
	}
	patch := OntologyInput{OperationID: "kb-test-8", Layer: &OntologyLayer{ID: "L"},
		Actors: []OntologyActor{{Key: "p", Existing: project}, {Key: "q", Existing: other}, {Key: "t", FormID: 12, Title: "Task", Parent: "p"}},
		Edges:  []OntologyEdge{{From: "p", To: "q", Type: "interaction"}}}
	for range 2 {
		out, err := c.ApplyOntology(context.Background(), patch, grid)
		if err != nil || !out.Done || out.LayerID != "L" {
			t.Fatalf("%+v %v", out, err)
		}
	}
	if _, nodes, edges := f.Snapshot("L"); len(nodes) != 3 || len(edges) != 2 {
		t.Fatalf("%d nodes, %d edges on the layer", len(nodes), len(edges))
	}
	if _, err := c.ApplyOntology(context.Background(), OntologyInput{OperationID: "kb-test-9", Layer: &OntologyLayer{ID: "L"},
		Actors: []OntologyActor{{Key: "t", FormID: 12, Title: "Other"}}}, nil); err == nil {
		t.Fatal("a new actor on a layer without a layout")
	}
}

// The patch's own shape is refused before anything is read.
func TestOntologyShape(t *testing.T) {
	for _, tc := range []struct{ patch, want string }{
		{`{"operationId":"bad id"}`, "operationId"},
		{`{"operationId":"x"}`, "an empty patch"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"},{"key":"a","formId":1,"title":"B"}]}`, "unique keys"},
		{`{"operationId":"x","actors":[{"key":"a","existing":"id","data":{"k":1}}]}`, "existing takes only"},
		{`{"operationId":"x","actors":[{"key":"a","title":"A"}]}`, "formId or form"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A","parent":"zz"}]}`, `parent "zz"`},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A","aliases":[" "]}]}`, "aliases are"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"edges":[{"from":"a","to":"a"}]}`, "two different actor keys"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"accounts":[{"actor":"a","name":"N","currency":"C","accountType":"avg"}]}`, "accountType fact or plan"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"transactions":[{"key":"t","actor":"a","account":"N","currency":"C","amount":0}]}`, "positive amount"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"transactions":[{"key":"t","actor":"a","account":"N","currency":"C","amount":1,"side":"left"}]}`, "side is credit"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"transfers":[{"key":"t","from":[{"actor":"a","account":"N","currency":"C","amount":1}]}]}`, "from and to legs"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"transfers":[{"key":"t","type":"reversed","from":[],"to":[]}]}`, "type completed (default) or authorized"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"transfers":[{"key":"t","hold":"h","type":"authorized"}]}`, "closing hold"},
		{`{"operationId":"x","actors":[{"key":"a","formId":1,"title":"A"}],"transactions":[{"key":"t","actor":"a","account":"N","currency":"C","amount":1}],"transfers":[{"key":"t","from":[{"actor":"a","account":"N","currency":"C","amount":1}],"to":[{"actor":"a","account":"N","currency":"C","amount":1}]}]}`, "unique key"},
	} {
		var in OntologyInput
		if err := json.Unmarshal([]byte(tc.patch), &in); err != nil {
			t.Fatal(err)
		}
		if err := in.checkShape(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.patch, err)
		}
	}
}

// A source becomes a chip in the description: an actor chip, also for a comment (an actor; [quote]
// is not in the actor-description tag set) labelled with its text without markup; a template
// that refuses the batch leaves nothing written and names the actor.
func TestApplyOntologySourcesAndRefusedBatch(t *testing.T) {
	c, f, project := kbClient(t)
	comment := f.AddActor(30, "", nil)
	f.Set(func(s *simfake.Server) {
		s.Actors[comment].Description = "[b]Pay[/b] in two parts,\n[i]by Friday[/i] " + strings.Repeat("and more ", 20)
	})
	out := mustApply(t, c, `{"operationId":"kb-test-10","actors":[{"key":"a","formId":14,"title":"From actor","desc":"Notes.","source":"actor:`+project+`","new":true},`+
		`{"key":"b","formId":14,"title":"From comment","source":"reaction:`+comment+`"}]}`)
	a, _ := f.ActorData(out.Actors[0].ActorID)
	b, _ := f.ActorData(out.Actors[1].ActorID)
	if a.Description != "Notes.\n\nSource: [actor="+project+"]Project X[/actor]" || b.Description != "Source: [actor="+comment+"]comment: Pay in two parts, by Friday and more and more and more and more and more and mo…[/actor]" || strings.Contains(b.Description, "[quote") {
		t.Fatalf("%q / %q", a.Description, b.Description)
	}
	for _, src := range []string{"web:x", "actor:missing", "attachment:"} {
		if _, reply, failed := kbApply(t, c, `{"operationId":"kb-test-11","dryRun":true,"actors":[{"key":"a","formId":14,"title":"A","source":"`+src+`"}]}`); !failed || !strings.Contains(reply, "source") {
			t.Errorf("source %s: %s", src, reply)
		}
	}
	f.Set(func(s *simfake.Server) { s.BadForm = 12 })
	before, _, _ := f.Snapshot("")
	_, reply, failed := kbApply(t, c, `{"operationId":"kb-test-12","actors":[{"key":"ok","formId":14,"title":"Fine"},{"key":"bad","formId":12,"title":"Refused"}]}`)
	after, _, _ := f.Snapshot("")
	if !failed || !strings.Contains(reply, `actor "bad" was refused, so nothing of the structure was written`) || len(after) != len(before) {
		t.Fatalf("%v %s (%d -> %d actors)", failed, reply, len(before), len(after))
	}
}

// With transfers and a layer, the transfers' account pairs join the layer's accounts (the Graph
// Editor draws transfers only for those), after the pairs it already shows; the dry run only says
// which. The ledger mode comes from ontology/get.
func TestApplyOntologyLayerShowsTransfers(t *testing.T) {
	c, f, _ := kbClient(t)
	shown := map[string]any{"value": "n-other_840", "title": "Other, USD"}
	layer := f.AddActor(14, "kb-test layer", map[string]any{"accounts": []any{shown}})
	grid := func(_ *GraphBuild, keys []string, _ [][2]string) (map[string]GraphPoint, error) {
		out := map[string]GraphPoint{}
		for i, k := range keys {
			out[k] = GraphPoint{X: 1025 + 100*float64(i), Y: 1025}
		}
		return out, nil
	}
	patch := OntologyInput{OperationID: "kb-test-10", Layer: &OntologyLayer{ID: layer},
		Actors: []OntologyActor{{Key: "a", Form: "Counterparty", Title: "Payer"}, {Key: "b", Form: "Counterparty", Title: "Payee"}},
		Transfers: []OntologyTransfer{{Key: "t", From: []OntologyLeg{{Actor: "a", Account: "Money", Currency: "UAH", Amount: 5}},
			To: []OntologyLeg{{Actor: "b", Account: "Money", Currency: "UAH", Amount: 5}}}}}
	accounts := func() string {
		a, _ := f.ActorData(layer)
		b, _ := json.Marshal(a.Data["accounts"])
		return string(b)
	}
	before := accounts()
	dry := patch
	dry.DryRun = true
	out, err := c.ApplyOntology(context.Background(), dry, grid)
	if err != nil || out.Ledger != "simapi" || strings.Join(out.LayerAccounts, ";") != "Money, UAH" || accounts() != before {
		t.Fatalf("dry run: %+v %v; layer %s", out, err, accounts())
	}
	out, err = c.ApplyOntology(context.Background(), patch, grid)
	if err != nil || !out.Done || strings.Join(out.LayerAccounts, ";") != "Money, UAH" {
		t.Fatalf("apply: %+v %v", out, err)
	}
	if got := accounts(); !strings.HasPrefix(got, `[{"title":"Other, USD","value":"n-other_840"},{"accountName":"Money"`) || !strings.Contains(got, `"value":"n-money_980"`) {
		t.Fatalf("layer accounts %s", got)
	}
	if again, err := c.ApplyOntology(context.Background(), patch, grid); err != nil || len(again.LayerAccounts) != 0 {
		t.Fatalf("repeat: %+v %v", again, err)
	}
}

// A transaction without a side follows how the pair is posted on other actors; a side against that
// convention is a warning in the dry run (eval write-18: a budget kept on credit everywhere).
func TestOntologyPostingSideConvention(t *testing.T) {
	c, _, _ := kbClient(t)
	mustApply(t, c, `{"operationId":"kb-test-11","actors":[{"key":"a","form":"Counterparty","title":"Budget A"},{"key":"b","form":"Counterparty","title":"Budget B"}],`+
		`"transactions":[{"key":"sa","actor":"a","account":"Money","currency":"UAH","amount":10,"side":"credit"},{"key":"sb","actor":"b","account":"Money","currency":"UAH","amount":20,"side":"credit"}]}`)
	dry, _, failed := kbApply(t, c, `{"operationId":"kb-test-12","dryRun":true,"actors":[{"key":"p","form":"Counterparty","title":"Budget P"}],`+
		`"transactions":[{"key":"plain","actor":"p","account":"Money","currency":"UAH","amount":15},{"key":"against","actor":"p","account":"Money","currency":"UAH","amount":5,"side":"debit"}]}`)
	if failed || len(dry.Postings) != 2 {
		t.Fatalf("%+v", dry)
	}
	if d := dry.Postings[0].Detail; !strings.HasPrefix(d, "credit") || !strings.Contains(d, "convention: credit (2 actors, 0 on the other side)") {
		t.Fatal(d)
	}
	if w := strings.Join(dry.Warnings, "\n"); !strings.Contains(w, `transaction "against" posts on debit`) || strings.Contains(w, `"plain"`) {
		t.Fatal(w)
	}
}

// The convention is read once per pair, but each transaction leaves its own actor out: an actor's
// own postings never make the convention it is checked against.
func TestOntologyPostingSideLeavesEachActorOut(t *testing.T) {
	c, _, _ := kbClient(t)
	made := mustApply(t, c, `{"operationId":"kb-test-13","actors":[{"key":"a","form":"Counterparty","title":"Side A"},{"key":"b","form":"Counterparty","title":"Side B"},{"key":"c","form":"Counterparty","title":"Side C"}],`+
		`"transactions":[{"key":"ta","actor":"a","account":"Money","currency":"UAH","amount":1,"side":"credit"},{"key":"tb","actor":"b","account":"Money","currency":"UAH","amount":1,"side":"debit"},`+
		`{"key":"tc","actor":"c","account":"Money","currency":"UAH","amount":1,"side":"debit"}]}`)
	ids := map[string]string{}
	for _, a := range made.Actors {
		ids[a.Key] = a.ActorID
	}
	dry, _, failed := kbApply(t, c, `{"operationId":"kb-test-14","dryRun":true,"actors":[{"key":"a","existing":"`+ids["a"]+`"},{"key":"b","existing":"`+ids["b"]+`"}],`+
		`"transactions":[{"key":"pa","actor":"a","account":"Money","currency":"UAH","amount":2},{"key":"pb","actor":"b","account":"Money","currency":"UAH","amount":2}]}`)
	if failed || len(dry.Postings) != 2 {
		t.Fatalf("%+v", dry)
	}
	// a: b and c post on debit. b: a on credit, c on debit, a tie: no convention, the default.
	if d := dry.Postings[0].Detail; !strings.HasPrefix(d, "debit") || !strings.Contains(d, "convention: debit (2 actors, 0 on the other side)") {
		t.Fatal("a:", d)
	}
	if d := dry.Postings[1].Detail; !strings.HasPrefix(d, "credit") || strings.Contains(d, "convention") {
		t.Fatal("b:", d)
	}
}
