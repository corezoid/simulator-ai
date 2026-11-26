package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func asAPI(err error, target **APIError) bool { return errors.As(err, target) }

// legacyLedgerNote is what the person hears when the workspace's ledger is still the legacy one.
const (
	legacyLedgerNote    = "postings in this workspace are still made in the web app (its ledger is on the legacy API): the structure is written, the transactions and transfers are not"
	legacyLedgerDryNote = "postings in this workspace are still made in the web app (its ledger is on the legacy API): the apply will write the structure, not the transactions and transfers"
)

// ApplyOntology checks the patch against the workspace and, unless dryRun, writes it: the structure
// in one atomic batch, then the ledger. Problems come back as one error before anything is written.
func (c *Client) ApplyOntology(ctx context.Context, in OntologyInput, layout OntologyLayout) (OntologyResult, error) {
	if e := consoleOnly(c); e != nil {
		return OntologyResult{}, e
	}
	if err := in.checkShape(); err != nil {
		return OntologyResult{}, err
	}
	p, err := c.planOntology(ctx, in)
	if err != nil {
		return OntologyResult{}, err
	}
	if !in.DryRun {
		p.refuseDuplicates()
	}
	if len(p.problems) > 0 {
		return OntologyResult{}, problemsError(p.problems)
	}
	var b *GraphBuild
	if in.Layer != nil {
		if b, err = c.OpenGraphBuild(ctx, GraphBuildTarget{LayerID: in.Layer.ID, OperationID: in.OperationID}); err != nil {
			return OntologyResult{}, fmt.Errorf("layer: %w", err)
		}
	}
	at, err := p.layout(b, layout)
	if err != nil {
		return OntologyResult{}, err
	}
	s, err := c.buildStructure(p, b, at)
	if err != nil {
		return OntologyResult{}, err
	}
	out := OntologyResult{DryRun: in.DryRun, Edges: s.edges, Accounts: len(p.accounts), Warnings: p.warnings}
	if b != nil {
		out.LayerID = b.LayerID
	}
	if n := p.objects(); n > ontologyBulkObjects {
		out.Bulk = fmt.Sprintf("a mass operation: %d objects (more than %d); show the person the whole diff and get an explicit yes", n, ontologyBulkObjects)
	}
	postings := len(in.Transactions) + len(in.Transfers)
	if in.DryRun {
		if postings > 0 {
			var note string
			if out.Ledger, note = c.ledgerMode(ctx); note != "" {
				out.Warnings = append(out.Warnings, "ledger: "+note)
			}
		}
		out.Postings = p.plannedPostings(out.Ledger)
		if b != nil && out.Ledger != "legacy" {
			if out.LayerAccounts, err = c.showTransfers(ctx, p, b.LayerID, true); err != nil {
				out.Warnings = append(out.Warnings, "layer accounts: "+err.Error())
			}
		}
		out.Actors = p.actorsOut()
		out.Summary = p.summary(out, true)
		return out, nil
	}
	if err = c.applyStructure(ctx, p, s, b); err != nil {
		return OntologyResult{}, err
	}
	out.Actors = p.actorsOut()
	if postings > 0 {
		var legacy bool
		out.Postings, legacy, err = c.applyLedger(ctx, p)
		out.Ledger = "simapi"
		if legacy {
			out.Ledger = "legacy"
		}
		if err != nil {
			out.Warnings = append(out.Warnings, err.Error())
		}
		if b != nil && !legacy {
			var shown error
			if out.LayerAccounts, shown = c.showTransfers(ctx, p, b.LayerID, false); shown != nil {
				out.Warnings = append(out.Warnings, "layer accounts: "+shown.Error())
			}
		}
	}
	out.Done = err == nil
	for _, o := range out.Postings {
		out.Done = out.Done && o.Status != "failed"
	}
	out.Summary = p.summary(out, false)
	return out, nil
}

// refuseDuplicates refuses to create an actor recognition found a likely twin of, unless the patch
// says so: name it existing, or set new.
func (p *ontologyPlan) refuseDuplicates() {
	for _, a := range p.actors {
		if a.status == "create" && len(a.candidates) > 0 && !a.in.New {
			c := a.candidates[0]
			p.problem("actor %q: %q (%s, %s) may be the same: set existing to its id, or new: true when it is another one (dry run lists all candidates)", a.in.Key, c.Title, c.ActorID, c.Reason)
		}
	}
}

func problemsError(problems []string) error {
	const most = 25
	more := ""
	if len(problems) > most {
		more = fmt.Sprintf("\n… and %d more", len(problems)-most)
		problems = problems[:most]
	}
	return fmt.Errorf("nothing was written; fix the patch:\n- %s%s", strings.Join(problems, "\n- "), more)
}

// layout places the actors this call puts on the layer: new ones, and ones an earlier call of the
// operation created off the layer.
func (p *ontologyPlan) layout(b *GraphBuild, layout OntologyLayout) (map[string]GraphPoint, error) {
	if b == nil {
		return nil, nil
	}
	var keys []string
	for _, a := range p.actors {
		if _, on := b.laIDs[a.id]; a.status == "create" || a.status == "exists" && !on {
			keys = append(keys, a.in.Key)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if layout == nil {
		return nil, errors.New("no layout for the layer")
	}
	edges := make([][2]string, len(p.edges))
	for i, e := range p.edges {
		edges[i] = [2]string{e.from, e.to}
	}
	return layout(b, keys, edges)
}

// objects counts what the patch writes, for the bulk rule.
func (p *ontologyPlan) objects() int {
	n := len(p.edges) + len(p.accounts) + len(p.in.Transactions) + len(p.in.Transfers)
	for _, a := range p.actors {
		if a.status == "create" {
			n++
		}
	}
	return n
}

func (p *ontologyPlan) actorsOut() []OntologyActorOut {
	out := make([]OntologyActorOut, len(p.actors))
	for i, a := range p.actors {
		out[i] = OntologyActorOut{Key: a.in.Key, Status: a.status, ActorID: a.id, Title: a.title, FormID: a.formID, Candidates: a.candidates}
	}
	return out
}

// plannedPostings is the dry run's view of phase b.
func (p *ontologyPlan) plannedPostings(ledger string) []OntologyPostingOut {
	status := "plan"
	if ledger == "legacy" {
		status = "skipped"
	}
	var out []OntologyPostingOut
	for _, t := range p.in.Transactions {
		out = append(out, OntologyPostingOut{Key: t.Key, Kind: "transaction", Status: status, Amount: t.Amount, Currency: t.Currency,
			Ref: postingRef(p.in.OperationID, t.Key), Detail: strings.TrimSuffix(fmt.Sprintf("%s %s of %q; %s", sideOf(t.Side), t.Account, t.Actor, p.sides[t.Key]), "; ")})
	}
	for _, t := range p.in.Transfers {
		t.From, t.To = p.legsOf(t)
		o := OntologyPostingOut{Key: t.Key, Kind: "transfer", Status: status, Amount: sumLegs(t.From), Ref: postingRef(p.in.OperationID, t.Key), Detail: transferDetail(t)}
		if len(t.From) > 0 {
			o.Currency = t.From[0].Currency
		}
		out = append(out, o)
	}
	return out
}

// legsOf is a transfer's legs: its own, or, closing a hold of this patch without them, the hold's.
func (p *ontologyPlan) legsOf(t OntologyTransfer) (from, to []OntologyLeg) {
	if t.Hold != "" && len(t.From) == 0 {
		for _, h := range p.in.Transfers {
			if h.Key == t.Hold {
				return h.From, h.To
			}
		}
	}
	return t.From, t.To
}

// summary says in one paragraph what the patch will do or did.
func (p *ontologyPlan) summary(out OntologyResult, dry bool) string {
	counts := map[string]int{}
	for _, a := range out.Actors {
		counts[a.Status]++
	}
	var parts []string
	verb := "created"
	if dry {
		verb = "will create"
	}
	parts = append(parts, fmt.Sprintf("actors: %s %d, already made by this operation %d, existing %d", verb, counts["create"]+counts["created"], counts["exists"], counts["existing"]))
	parts = append(parts, fmt.Sprintf("edges %d, accounts %d", out.Edges, out.Accounts))
	for _, a := range out.Actors {
		if len(a.Candidates) > 0 && (a.Status == "create") {
			parts = append(parts, fmt.Sprintf("%q may already exist (%d candidates): confirm with the person", a.Key, len(a.Candidates)))
		}
	}
	posted := map[string]int{}
	for _, o := range out.Postings {
		posted[o.Status]++
	}
	if len(out.Postings) > 0 {
		parts = append(parts, fmt.Sprintf("postings: planned %d, posted %d, already posted %d, skipped %d, failed %d", posted["plan"], posted["posted"], posted["exists"], posted["skipped"], posted["failed"]))
	}
	switch {
	case out.Ledger == "legacy" && dry:
		parts = append(parts, legacyLedgerDryNote)
	case out.Ledger == "legacy":
		parts = append(parts, legacyLedgerNote)
	}
	if dry {
		parts = append(parts, "nothing was written: show this diff to the person and apply the same patch with dryRun false after their yes")
	}
	return strings.Join(parts, "; ")
}
