package publicapi

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// accountSlot is an account of an actor of the patch: the actor's key, the pair and the type.
type accountSlot struct {
	actor string
	pair  AccountPair
	kind  string
}

func accountKindOf(t string) string {
	if t == "" {
		return "fact"
	}
	return t
}

// pair finds an account pair by account name and currency; both must exist (account_pair_create
// makes a new one). Results are kept per name.
func (p *ontologyPlan) pair(ctx context.Context, c *Client, name, currency string) (AccountPair, bool) {
	key := [2]string{strings.TrimSpace(name), strings.TrimSpace(currency)}
	if got, ok := p.pairs[key]; ok {
		return got, got.NameID != ""
	}
	p.pairs[key] = AccountPair{}
	nameID, err := lookup[string](ctx, c, "account_name/list", key[0])
	if err != nil || nameID == "" {
		p.problem("account name %q is not in the workspace: create the pair first (account_pair_create) or use an existing name (account_names_list)", key[0])
		return AccountPair{}, false
	}
	currencyID, err := lookup[int64](ctx, c, "account_currency/list", key[1])
	if err != nil || currencyID == 0 {
		p.problem("currency %q is not in the workspace: create it first (account_pair_create) or use an existing one (currencies_list)", key[1])
		return AccountPair{}, false
	}
	// A name and a currency make a pair once registered (account_pair_create); an unregistered one
	// has no access rules, and nothing can be posted to it, so the apply would write the structure
	// and fail the postings. sim-api answers notFound for such a pair.
	if _, err = call[map[string]any, accessList](ctx, c, "access_rule/account/list", map[string]any{"accId": c.workspace(), "nameId": nameID, "currencyId": currencyID}); isNotFound(err) {
		p.problem("account %s / %s is not registered as a pair: create it first (account_pair_create)", key[0], key[1])
		return AccountPair{}, false
	}
	got := AccountPair{NameID: nameID, CurrencyID: currencyID}
	p.pairs[key] = got
	return got, true
}

// resolveAccounts resolves the accounts to open and checks every posting's account will exist: one
// the patch opens, one an existing actor has, or one the new actor's template opens on it.
func (p *ontologyPlan) resolveAccounts(ctx context.Context, c *Client) error {
	declared := map[accountSlot]bool{}
	for _, a := range p.in.Accounts {
		pair, ok := p.pair(ctx, c, a.Name, a.Currency)
		if !ok {
			continue
		}
		slot := accountSlot{actor: a.Actor, pair: pair, kind: accountKindOf(a.AccountType)}
		if !declared[slot] {
			declared[slot] = true
			p.accounts = append(p.accounts, planAccount{actor: a.Actor, pair: pair, kind: slot.kind})
		}
	}
	bound := map[int64]map[accountSlot]bool{}
	checked := map[accountSlot]bool{}
	has := func(l OntologyLeg) (AccountPair, bool) {
		pair, ok := p.pair(ctx, c, l.Account, l.Currency)
		if !ok {
			return pair, false
		}
		slot := accountSlot{actor: l.Actor, pair: pair, kind: accountKindOf(l.AccountType)}
		if declared[slot] || checked[slot] {
			return pair, true
		}
		a := p.byKey[l.Actor]
		var found bool
		if a.id != "" {
			found = c.actorHasAccount(ctx, a.id, slot)
		} else {
			if _, read := bound[a.formID]; !read {
				bound[a.formID] = c.templateAccounts(ctx, a.formID)
			}
			found = bound[a.formID][accountSlot{pair: pair, kind: slot.kind}]
		}
		if !found {
			p.problem("actor %q has no %s account %s / %s: add {actor: %q, name: %q, currency: %q} to accounts", l.Actor, slot.kind, l.Account, l.Currency, l.Actor, l.Account, l.Currency)
			return pair, false
		}
		checked[slot] = true
		return pair, true
	}
	for _, t := range p.in.Transactions {
		has(t.OntologyLeg)
	}
	for _, t := range p.in.Transfers {
		legs := append(append([]OntologyLeg{}, t.From...), t.To...)
		currencies := map[string]bool{}
		for _, l := range legs {
			has(l)
			currencies[strings.ToLower(strings.TrimSpace(l.Currency))] = true
		}
		if len(currencies) > 1 {
			p.problem("transfer %q: one currency per transfer (its legs name %d)", t.Key, len(currencies))
		}
		if t.Hold != "" {
			p.checkHold(t)
			continue
		}
		if math.Abs(sumLegs(t.From)-sumLegs(t.To)) > 1e-9 {
			p.problem("transfer %q: from legs give %v and to legs receive %v; an exchange is balanced", t.Key, sumLegs(t.From), sumLegs(t.To))
		}
	}
	return nil
}

// checkHold checks the closing of a hold: legs to repeat (its own, or the hold's in this patch).
func (p *ontologyPlan) checkHold(t OntologyTransfer) {
	for _, h := range p.in.Transfers {
		if h.Key == t.Key {
			break // the hold, if it is in this patch, comes first: transfers post in patch order
		}
		if h.Key == t.Hold {
			if h.Type != "authorized" {
				p.problem("transfer %q: hold %q is not an authorized transfer", t.Key, t.Hold)
			}
			return
		}
	}
	for _, h := range p.in.Transfers {
		if h.Key == t.Hold {
			p.problem("transfer %q: put hold %q before the transfer that closes it (postings run in patch order)", t.Key, t.Hold)
			return
		}
	}
	if len(t.From) == 0 || len(t.To) == 0 {
		p.problem("transfer %q: closing hold %q of an earlier call: repeat its from and to legs, in its order", t.Key, t.Hold)
	}
}

func sumLegs(legs []OntologyLeg) float64 {
	total := 0.0
	for _, l := range legs {
		total += l.Amount
	}
	return total
}

// accountRowsWire is account_actor/get's rows of a pair.
type accountRowsWire struct {
	List []struct {
		ID         string `json:"id"`
		IncomeType string `json:"incomeType"`
		Type       string `json:"type"`
	} `json:"list"`
}

// accountRows reads an actor's rows of one account: credit and debit ids.
func (c *Client) accountRows(ctx context.Context, actorID string, slot accountSlot) (credit, debit string, err error) {
	rows, err := call[map[string]any, accountRowsWire](ctx, c, "account_actor/get", map[string]any{"accId": c.workspace(), "actorId": actorID,
		"nameId": slot.pair.NameID, "currencyId": slot.pair.CurrencyID, "accountType": slot.kind})
	if err != nil {
		return "", "", err
	}
	for _, r := range rows.List {
		switch r.IncomeType {
		case "credit":
			credit = r.ID
		case "debit":
			debit = r.ID
		}
	}
	if credit == "" || debit == "" {
		return "", "", fmt.Errorf("the %s account has no credit and debit rows", slot.kind)
	}
	return credit, debit, nil
}

func (c *Client) actorHasAccount(ctx context.Context, actorID string, slot accountSlot) bool {
	_, _, err := c.accountRows(ctx, actorID, slot)
	return err == nil
}

// templateAccounts is the accounts a template opens on each actor created from it.
func (c *Client) templateAccounts(ctx context.Context, formID int64) map[accountSlot]bool {
	out := map[accountSlot]bool{}
	page, err := call[map[string]any, struct {
		List []struct {
			NameID     string `json:"nameId"`
			CurrencyID int64  `json:"currencyId"`
			Type       string `json:"type"`
		} `json:"list"`
	}](ctx, c, "account_form/list", map[string]any{"accId": c.workspace(), "formId": formID})
	if err != nil {
		return out
	}
	for _, b := range page.List {
		out[accountSlot{pair: AccountPair{NameID: b.NameID, CurrencyID: b.CurrencyID}, kind: accountKindOf(b.Type)}] = true
	}
	return out
}

// ledgerMode reads who writes this workspace's ledger from ontology/get (ledgerWrite: simapi, or
// legacy while the workspace is still on the previous backend), writing nothing. A stand without
// ontology/get answers unknown: the apply then learns it from the first posting (ledgerWrongAPI).
func (c *Client) ledgerMode(ctx context.Context) (string, string) {
	got, err := call[map[string]any, struct {
		LedgerWrite string `json:"ledgerWrite"`
	}](ctx, c, "ontology/get", map[string]any{"accId": c.workspace(), "fields": []string{"ledgerWrite"}, "limit": 1})
	switch {
	case err != nil:
		return "unknown", "ontology/get: " + err.Error()
	case got.LedgerWrite == "simapi" || got.LedgerWrite == "legacy":
		return got.LedgerWrite, ""
	}
	return "unknown", "ontology/get did not name the ledger"
}
