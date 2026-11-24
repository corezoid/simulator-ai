package publicapi

import (
	"context"
	"fmt"
	"time"
)

// Which side of an account a one-sided posting goes to is the workspace's convention, not a rule:
// the skill's "income on credit, spending on debit" is the default, but a budget account may keep
// its spending on credit on every project (eval write-18 posted on debit against all the others).
// PostingSide reads the convention from the pair's own recent postings: one transaction/filter over
// the last 366 days, transfer legs left out (a transfer always debits the payer and credits the
// receiver), each actor counted once by its own majority side.

// postingSideRows bounds the read: enough recent postings to see the convention.
const postingSideRows = 500

// PostingSide is how other actors post one-sided transactions of an account pair.
type PostingSide struct {
	Side   string `json:"side"`             // credit or debit: the side most actors post on
	Actors int    `json:"actors"`           // actors posting on Side
	Other  int    `json:"otherSide"`        // actors posting on the other side
	Note   string `json:"note"`             // what to do with it
	Window string `json:"window,omitempty"` // the period read
}

// PostingSideOf answers the convention of a pair and account type (fact by default) among actors other
// than except, or nil when there is none (no one-sided postings, or as many actors on each side).
func (c *Client) PostingSideOf(ctx context.Context, pair AccountPair, accountType, except string) (*PostingSide, error) {
	tally, err := c.postingTally(ctx, pair, accountType)
	if err != nil {
		return nil, err
	}
	return tally.side(except), nil
}

// postingTally is each actor's one-sided postings of a pair: credit postings minus debit ones.
type postingTally map[string]int

// postingTally reads the pair's recent one-sided postings once; side() then leaves any actor out.
func (c *Client) postingTally(ctx context.Context, pair AccountPair, accountType string) (postingTally, error) {
	if pair.NameID == "" || pair.CurrencyID < 1 {
		return nil, nil
	}
	if accountType != "plan" {
		accountType = "fact"
	}
	now := time.Now().UnixMilli()
	from := now - int64(366*24*time.Hour/time.Millisecond)
	page, err := call[map[string]any, struct {
		List []struct {
			ActorID    string `json:"actorId"`
			IncomeType string `json:"incomeType"`
			TransferID string `json:"transferId"`
		} `json:"list"`
	}](ctx, c, "transaction/filter", map[string]any{"accId": c.workspace(), "accounts": []map[string]any{{"nameId": pair.NameID, "currencyId": pair.CurrencyID}},
		"accountType": accountType, "type": "completed", "from": from, "to": now, "limit": postingSideRows})
	if err != nil {
		return nil, err
	}
	perActor := postingTally{}
	for _, t := range page.List {
		if t.TransferID != "" || t.ActorID == "" {
			continue
		}
		switch t.IncomeType {
		case "credit":
			perActor[t.ActorID]++
		case "debit":
			perActor[t.ActorID]--
		}
	}
	return perActor, nil
}

// side is the convention among the actors other than except, or nil.
func (perActor postingTally) side(except string) *PostingSide {
	credit, debit := 0, 0
	for id, n := range perActor {
		if id == except {
			continue
		}
		switch {
		case n > 0:
			credit++
		case n < 0:
			debit++
		}
	}
	if credit == debit {
		return nil
	}
	out := &PostingSide{Side: "credit", Actors: credit, Other: debit, Window: "the last 366 days"}
	if debit > credit {
		out.Side, out.Actors, out.Other = "debit", debit, credit
	}
	out.Note = fmt.Sprintf("this account's one-sided postings go to %s on %d other actor(s) (%d on the other side): post on %s too, whatever the kind (income or spending), unless the person names the side", out.Side, out.Actors, out.Other, out.Side)
	return out
}

// resolveSides gives each transaction of the patch its side: the one it names, else the pair's
// convention among other actors, else credit (income). A named side against the convention is a
// warning the dry run shows; the convention is in the posting's detail.
func (p *ontologyPlan) resolveSides(ctx context.Context, c *Client) error {
	seen := map[string]postingTally{} // one read per pair and kind; each transaction leaves its own actor out
	for i := range p.in.Transactions {
		t := &p.in.Transactions[i]
		pair, ok := p.pairs[[2]string{trimmed(t.Account), trimmed(t.Currency)}]
		if !ok || pair.NameID == "" {
			continue
		}
		key := fmt.Sprintf("%s|%d|%s", pair.NameID, pair.CurrencyID, accountKindOf(t.AccountType))
		tally, done := seen[key]
		if !done {
			var err error
			if tally, err = c.postingTally(ctx, pair, accountKindOf(t.AccountType)); err != nil {
				p.warnings = append(p.warnings, fmt.Sprintf("posting side of %s / %s: not read (%v); the transaction keeps %s", t.Account, t.Currency, err, sideName(t.Side)))
			}
			seen[key] = tally
		}
		except := ""
		if a := p.byKey[t.Actor]; a != nil {
			except = a.id
		}
		conv := tally.side(except)
		if conv == nil {
			continue
		}
		if p.sides == nil {
			p.sides = map[string]string{}
		}
		p.sides[t.Key] = fmt.Sprintf("convention: %s (%d actors, %d on the other side)", conv.Side, conv.Actors, conv.Other)
		switch {
		case t.Side == "":
			t.Side = conv.Side
		case t.Side != conv.Side:
			p.warnings = append(p.warnings, fmt.Sprintf("transaction %q posts on %s, but %s / %s is posted on %s on %d other actor(s) (%d on %s): follow the convention (side %s, or leave side out) unless the person said %s",
				t.Key, t.Side, t.Account, t.Currency, conv.Side, conv.Actors, conv.Other, t.Side, conv.Side, t.Side))
		}
	}
	return nil
}

func sideName(side string) string {
	if side == "debit" {
		return "debit"
	}
	return "credit"
}
