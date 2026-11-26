package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/simfake"
)

// kbWorkspace is a workspace with a contract template (an amount and a status select), a
// counterparty template that opens a Money account on each actor, a project template, two account
// names and a currency, and an existing project.
func kbWorkspace(t *testing.T) (func(tool, args string) (string, bool), *simfake.Server, string) {
	t.Helper()
	call, f := fakeConsole(t)
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
	project := f.AddActor(14, "Project X", nil)
	return call, f, project
}

// scenarioPatch is the brief's scenario: a counterparty, a contract under the project with its
// accounts, a plan transaction, a payment transfer and a Gantt dependency.
func scenarioPatch(op, project string, dry bool, extra string) string {
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

type ontologyReply struct {
	Done   bool `json:"done"`
	Actors []struct {
		Key        string `json:"key"`
		Status     string `json:"status"`
		ActorID    string `json:"actorId"`
		Candidates []struct {
			ActorID string `json:"actorId"`
			Reason  string `json:"reason"`
		} `json:"candidates"`
	} `json:"actors"`
	Edges    int `json:"edges"`
	Accounts int `json:"accounts"`
	Postings []struct {
		Key    string `json:"key"`
		Status string `json:"status"`
	} `json:"postings"`
	Ledger  string `json:"ledger"`
	Summary string `json:"summary"`
}

func applyOK(t *testing.T, call func(string, string) (string, bool), args string) ontologyReply {
	t.Helper()
	reply, failed := call("ontology_apply", args)
	if failed {
		t.Fatalf("ontology_apply failed: %s", reply)
	}
	var out ontologyReply
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		t.Fatal(err, reply)
	}
	return out
}

func statuses(out ontologyReply) string {
	var parts []string
	for _, a := range out.Actors {
		parts = append(parts, a.Key+"="+a.Status)
	}
	for _, p := range out.Postings {
		parts = append(parts, p.Key+"="+p.Status)
	}
	return strings.Join(parts, " ")
}

// A dry run reads only; the apply writes the structure in one atomic batch and the postings with
// refs; the same patch again writes nothing new.
func TestOntologyApplyScenario(t *testing.T) {
	call, f, project := kbWorkspace(t)
	dry := applyOK(t, call, scenarioPatch("kb-test-1", project, true, ""))
	if got := statuses(dry); got != "project=existing acme=create c1=create delivery=create plan=plan pay1=plan" || dry.Edges != 3 || dry.Accounts != 2 || dry.Ledger != "simapi" || dry.Done {
		t.Fatalf("dry run: %s edges %d accounts %d ledger %q", got, dry.Edges, dry.Accounts, dry.Ledger)
	}
	if n := f.Count("transaction/atom/create"); n != 0 || !strings.Contains(dry.Summary, "nothing was written") {
		t.Fatalf("dry run wrote: %d atom calls; %s", n, dry.Summary)
	}
	actors, _, _ := f.Snapshot("")
	if len(actors) != 1 {
		t.Fatalf("dry run created actors: %d", len(actors))
	}

	out := applyOK(t, call, scenarioPatch("kb-test-1", project, false, ""))
	if got := statuses(out); got != "project=existing acme=created c1=created delivery=created plan=posted pay1=posted" || !out.Done {
		t.Fatalf("apply: %s (%s)", got, out.Summary)
	}
	c1, _ := f.ActorData(out.Actors[2].ActorID)
	status, _ := json.Marshal(c1.Data["status"])
	if string(status) != `[{"title":"Signed","value":"signed"}]` || !strings.Contains(c1.Description, "[file=contract.pdf]contract.pdf[/file]") {
		t.Fatalf("contract stored %s, %q", status, c1.Description)
	}
	acme, _ := f.ActorData(out.Actors[1].ActorID)
	if aliases, _ := json.Marshal(acme.Data["aliases"]); string(aliases) != `["Акме"]` {
		t.Fatalf("aliases %s", aliases)
	}
	accounts, postings, transfers := f.LedgerSnapshot()
	if len(accounts) != 3 || len(postings) != 3 || len(transfers) != 1 || transfers[0].Ref != "kb-test-1:pay1" || postings[0].Ref != "kb-test-1:plan" {
		t.Fatalf("ledger: %d accounts, %+v, %+v", len(accounts), postings, transfers)
	}
	if n := f.Count("batch"); n < 1 {
		t.Fatal("no batch")
	}
	edges := countEdges(f)

	again := applyOK(t, call, scenarioPatch("kb-test-1", project, false, ""))
	if got := statuses(again); got != "project=existing acme=exists c1=exists delivery=exists plan=exists pay1=exists" || !again.Done {
		t.Fatalf("repeat: %s", got)
	}
	accounts2, postings2, transfers2 := f.LedgerSnapshot()
	if len(accounts2) != 3 || len(postings2) != 3 || len(transfers2) != 1 || countEdges(f) != edges {
		t.Fatalf("repeat wrote again: %d accounts, %d postings, %d transfers, %d edges (was %d)", len(accounts2), len(postings2), len(transfers2), countEdges(f), edges)
	}
}

func countEdges(f *simfake.Server) int {
	n := 0
	f.Set(func(s *simfake.Server) { n = len(s.Edges) })
	return n
}

// With a layer the new actors are placed and the edges between them drawn, in the same batch.
func TestOntologyApplyOnLayer(t *testing.T) {
	call, f, project := kbWorkspace(t)
	out := applyOK(t, call, scenarioPatch("kb-test-5", project, false, `,"layer":{"id":"layer-kb"}`))
	if !out.Done {
		t.Fatal(out.Summary)
	}
	_, nodes, edges := f.Snapshot("layer-kb")
	if len(nodes) != 3 || len(edges) != 1 { // the project is not on the layer: only c1 -> delivery is drawn
		t.Fatalf("%d nodes and %d edges on the layer", len(nodes), len(edges))
	}
	again := applyOK(t, call, scenarioPatch("kb-test-5", project, false, `,"layer":{"id":"layer-kb"}`))
	if _, nodes, edges = f.Snapshot("layer-kb"); !again.Done || len(nodes) != 3 || len(edges) != 1 {
		t.Fatalf("repeat placed again: %d nodes and %d edges", len(nodes), len(edges))
	}
}
