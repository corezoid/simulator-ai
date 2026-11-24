package publicapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ontology_apply writes one "ontology patch": actors (new, or existing ones named by id), the edges
// between them, accounts on them, and the ledger postings on those accounts, all named by keys of the
// patch instead of ids. It runs in two phases, because a ledger write is not allowed inside an atomic
// batch (sim-api service_transaction): (a) the structure — actors, edges, accounts and, with a layer,
// their placements — in ONE atomic batch with result references; (b) the ledger — one-sided
// transactions (transaction/atom/create, up to 20 legs atomically) and exchanges (transfer/create),
// each with ref "<operationId>:<key>", so a repeated patch posts nothing twice.
//
// Retry key: a new actor carries ref BuildRef(operationId, key) (the graph_build ref, so its
// recovery and graph_build_delete apply), a transaction or transfer ref "<operationId>:<key>".

// Bounds of one patch. The structure must fit one atomic batch (MaxBatchItems in sim-api: 100).
const (
	maxOntologyActors   = 60
	maxOntologyEdges    = 90
	maxOntologyAccounts = 60
	maxOntologyPostings = 20
	maxOntologyLegs     = 20
	ontologyBatchItems  = 100
	// ontologyBulkObjects is the Console's bulk rule (sim-api HERMES_BULK_WARN_OBJECTS default):
	// a patch above it is a mass operation the person confirms knowingly.
	ontologyBulkObjects = 50
)

// OntologyInput is the ontology patch.
type OntologyInput struct {
	OperationID  string                `json:"operationId"`
	DryRun       bool                  `json:"dryRun,omitempty"`
	Actors       []OntologyActor       `json:"actors,omitempty"`
	Edges        []OntologyEdge        `json:"edges,omitempty"`
	Accounts     []OntologyAccount     `json:"accounts,omitempty"`
	Transactions []OntologyTransaction `json:"transactions,omitempty"`
	Transfers    []OntologyTransfer    `json:"transfers,omitempty"`
	Layer        *OntologyLayer        `json:"layer,omitempty"`
}

// OntologyActor is a new actor (form + title) or an existing one (Existing: its id) under a key.
// Parent is the key of its hierarchy parent. Source ("attachment:<fileName>", "actor:<id>",
// "reaction:<id>") puts a chip to where the knowledge came from into the description. Aliases are
// other names of the actor, kept in data.aliases and searched by recognition. New says a new actor
// is meant although an actor of the same title or alias exists.
type OntologyActor struct {
	Key      string          `json:"key"`
	Existing string          `json:"existing,omitempty"`
	FormID   int64           `json:"formId,omitempty"`
	Form     string          `json:"form,omitempty"`
	Title    string          `json:"title,omitempty"`
	Desc     string          `json:"desc,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Parent   string          `json:"parent,omitempty"`
	Source   string          `json:"source,omitempty"`
	Aliases  []string        `json:"aliases,omitempty"`
	Color    string          `json:"color,omitempty"`
	New      bool            `json:"new,omitempty"`
}

// OntologyEdge joins two keys; Type is an edge type name: hierarchy (default), gantt (a Gantt
// dependency: the directedInteraction type the Gantt draws with) or any other type of the workspace.
type OntologyEdge struct {
	Type string `json:"type,omitempty"`
	From string `json:"from"`
	To   string `json:"to"`
	Name string `json:"name,omitempty"`
}

// OntologyAccount opens an account (account name x currency, by name) on an actor of the patch.
type OntologyAccount struct {
	Actor       string `json:"actor"`
	Name        string `json:"name"`
	Currency    string `json:"currency"`
	AccountType string `json:"accountType,omitempty"` // fact (default) or plan
}

// OntologyLeg is an amount on one account of an actor of the patch.
type OntologyLeg struct {
	Actor       string  `json:"actor"`
	Account     string  `json:"account"`
	Currency    string  `json:"currency"`
	AccountType string  `json:"accountType,omitempty"`
	Amount      float64 `json:"amount"`
}

// OntologyTransaction is a one-sided event on one account: income on its credit row (default) or
// spending on its debit row (side debit).
type OntologyTransaction struct {
	Key string `json:"key"`
	OntologyLeg
	Side    string `json:"side,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// OntologyTransfer is an exchange: from legs (debit rows) to legs (credit rows), one currency,
// balanced. Type completed (default) or authorized (a hold); Hold names the key of an authorized
// transfer of this operationId to close with type completed or canceled.
type OntologyTransfer struct {
	Key     string        `json:"key"`
	Type    string        `json:"type,omitempty"`
	Hold    string        `json:"hold,omitempty"`
	From    []OntologyLeg `json:"from,omitempty"`
	To      []OntologyLeg `json:"to,omitempty"`
	Comment string        `json:"comment,omitempty"`
}

// OntologyLayer places the new actors on a layer; it is optional.
type OntologyLayer struct {
	ID     string `json:"id"`
	Layout string `json:"layout,omitempty"`
	Origin string `json:"origin,omitempty"`
}

// OntologyCandidate is an existing actor recognition found for a new one, and why.
type OntologyCandidate struct {
	ActorID string `json:"actorId"`
	Title   string `json:"title"`
	FormID  int64  `json:"formId,omitempty"`
	Reason  string `json:"reason"`
}

// OntologyActorOut is one actor of the answer. Status: create (dry run), created, exists (created by
// an earlier call of this operationId) or existing (named by id).
type OntologyActorOut struct {
	Key        string              `json:"key"`
	Status     string              `json:"status"`
	ActorID    string              `json:"actorId,omitempty"`
	Title      string              `json:"title,omitempty"`
	FormID     int64               `json:"formId,omitempty"`
	Candidates []OntologyCandidate `json:"candidates,omitempty"`
}

// OntologyPostingOut is one transaction or transfer: Status plan (dry run), posted, exists (posted
// by an earlier call), skipped (the ledger is not on sim-api) or failed.
type OntologyPostingOut struct {
	Key      string  `json:"key"`
	Kind     string  `json:"kind"`
	Status   string  `json:"status"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Detail   string  `json:"detail,omitempty"`
	Ref      string  `json:"ref"`
	ID       string  `json:"id,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// OntologyResult is the diff of a dry run or what an apply did.
type OntologyResult struct {
	Done     bool                 `json:"done"`
	DryRun   bool                 `json:"dryRun,omitempty"`
	Actors   []OntologyActorOut   `json:"actors"`
	Edges    int                  `json:"edges"`
	Accounts int                  `json:"accounts"`
	Postings []OntologyPostingOut `json:"postings,omitempty"`
	// Ledger is where postings go: simapi, legacy (this workspace posts in the web app for now) or
	// unknown; empty when the patch has none.
	Ledger  string `json:"ledger,omitempty"`
	LayerID string `json:"layerId,omitempty"`
	// LayerAccounts are the account pairs added to (dry run: to add to) the layer's settings so
	// the Graph Editor draws its transfers.
	LayerAccounts []string `json:"layerAccounts,omitempty"`
	Bulk          string   `json:"bulk,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	Summary       string   `json:"summary"`
}

// ontologyKeyPattern is the graph_build key alphabet.
var ontologyKeyPattern = buildKeyPattern

// checkShape refuses a patch whose own shape is wrong, before anything is read.
func (in OntologyInput) checkShape() error {
	if !operationPattern.MatchString(in.OperationID) {
		return errOperationID
	}
	switch {
	case len(in.Actors)+len(in.Edges)+len(in.Accounts)+len(in.Transactions)+len(in.Transfers) == 0:
		return fmt.Errorf("an empty patch: name actors, edges, accounts, transactions or transfers")
	case len(in.Actors) > maxOntologyActors || len(in.Edges) > maxOntologyEdges || len(in.Accounts) > maxOntologyAccounts:
		return fmt.Errorf("at most %d actors, %d edges and %d accounts per patch (one atomic batch): split it, or use graph_build for a large graph", maxOntologyActors, maxOntologyEdges, maxOntologyAccounts)
	case len(in.Transactions)+len(in.Transfers) > maxOntologyPostings:
		return fmt.Errorf("at most %d transactions and transfers per patch", maxOntologyPostings)
	}
	keys := map[string]bool{}
	for _, a := range in.Actors {
		if !ontologyKeyPattern.MatchString(a.Key) || keys[a.Key] {
			return fmt.Errorf("actor key %q: unique keys of 1..64 characters A-Z a-z 0-9 _ . - required", a.Key)
		}
		keys[a.Key] = true
		if a.Existing != "" {
			if !validID(a.Existing) || a.FormID != 0 || a.Form != "" || len(a.Data) > 0 || a.Source != "" || len(a.Aliases) > 0 || a.New {
				return fmt.Errorf("actor %q: existing takes only the actor id (and parent); change an existing actor with actor_update", a.Key)
			}
			continue
		}
		if (a.FormID == 0) == (a.Form == "") || !validTitle(a.Title, false) || utf8.RuneCountInString(a.Desc) > 9000 {
			return fmt.Errorf("actor %q: formId or form (the template title), and a 1..1000 character title required (or existing)", a.Key)
		}
		if len(a.Aliases) > 20 {
			return fmt.Errorf("actor %q: at most 20 aliases", a.Key)
		}
		for _, alias := range a.Aliases {
			if strings.TrimSpace(alias) == "" || utf8.RuneCountInString(alias) > 255 {
				return fmt.Errorf("actor %q: aliases are 1..255 characters", a.Key)
			}
		}
	}
	for _, a := range in.Actors {
		if a.Parent != "" && (!keys[a.Parent] || a.Parent == a.Key) {
			return fmt.Errorf("actor %q: parent %q is no other actor key of the patch", a.Key, a.Parent)
		}
	}
	for _, e := range in.Edges {
		if !keys[e.From] || !keys[e.To] || e.From == e.To || utf8.RuneCountInString(e.Name) > 1000 {
			return fmt.Errorf("edge %s -> %s: two different actor keys of the patch required", e.From, e.To)
		}
	}
	for _, a := range in.Accounts {
		if !keys[a.Actor] || strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Currency) == "" || !accountKind(a.AccountType) {
			return fmt.Errorf("account %s/%s on %q: an actor key, the account name, the currency and accountType fact or plan required", a.Name, a.Currency, a.Actor)
		}
	}
	return in.checkPostingShape(keys)
}

func accountKind(t string) bool { return t == "" || t == "fact" || t == "plan" }

func (in OntologyInput) checkPostingShape(keys map[string]bool) error {
	refs := map[string]bool{}
	leg := func(what string, l OntologyLeg) error {
		if !keys[l.Actor] || strings.TrimSpace(l.Account) == "" || strings.TrimSpace(l.Currency) == "" || !accountKind(l.AccountType) || l.Amount <= 0 {
			return fmt.Errorf("%s: an actor key, account, currency and a positive amount required", what)
		}
		return nil
	}
	for _, t := range in.Transactions {
		if !ontologyKeyPattern.MatchString(t.Key) || refs[t.Key] {
			return fmt.Errorf("transaction key %q: a unique key required (it makes the ref)", t.Key)
		}
		refs[t.Key] = true
		if t.Side != "" && t.Side != "credit" && t.Side != "debit" {
			return fmt.Errorf("transaction %q: side is credit (income, default) or debit (spending)", t.Key)
		}
		if err := leg("transaction "+t.Key, t.OntologyLeg); err != nil {
			return err
		}
	}
	for _, t := range in.Transfers {
		if !ontologyKeyPattern.MatchString(t.Key) || refs[t.Key] {
			return fmt.Errorf("transfer key %q: a unique key required (it makes the ref)", t.Key)
		}
		refs[t.Key] = true
		switch {
		case t.Hold == "" && t.Type != "" && t.Type != "completed" && t.Type != "authorized":
			return fmt.Errorf("transfer %q: type completed (default) or authorized; close a hold with hold and type completed or canceled", t.Key)
		case t.Hold != "" && t.Type != "completed" && t.Type != "canceled":
			return fmt.Errorf("transfer %q: closing hold %q takes type completed or canceled", t.Key, t.Hold)
		case t.Hold == "" && (len(t.From) == 0 || len(t.To) == 0):
			return fmt.Errorf("transfer %q: from and to legs required (an exchange has two sides; one account is a transaction)", t.Key)
		case len(t.From) > maxOntologyLegs || len(t.To) > maxOntologyLegs:
			return fmt.Errorf("transfer %q: at most %d legs per side", t.Key, maxOntologyLegs)
		}
		for i, l := range t.From {
			if err := leg(fmt.Sprintf("transfer %s from[%d]", t.Key, i), l); err != nil {
				return err
			}
		}
		for i, l := range t.To {
			if err := leg(fmt.Sprintf("transfer %s to[%d]", t.Key, i), l); err != nil {
				return err
			}
		}
	}
	return nil
}

// postingRef is the ledger ref of a transaction or transfer of the patch.
func postingRef(operationID, key string) string { return operationID + ":" + key }
