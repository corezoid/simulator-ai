package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Phase b: the ledger, after the structure. Every posting carries ref "<operationId>:<key>" and is
// looked up by it first, so a repeated patch posts nothing twice. A workspace whose ledger is still
// the legacy one refuses the first write (ledgerWrongAPI): the structure stays, the postings are
// skipped and the answer says so.

// errLegacyLedger stops phase b on a workspace still on the legacy ledger.
var errLegacyLedger = errors.New("legacy ledger")

// ledgerRun is phase b's state: account rows by slot, and the answer's postings.
type ledgerRun struct {
	c    *Client
	p    *ontologyPlan
	rows map[accountSlot][2]string // credit, debit
	out  []OntologyPostingOut
}

// row is the account row a leg posts to: its credit row, or its debit row.
func (r *ledgerRun) row(ctx context.Context, l OntologyLeg, debit bool) (string, error) {
	pair, ok := r.p.pairs[[2]string{trimmed(l.Account), trimmed(l.Currency)}]
	if !ok || pair.NameID == "" {
		return "", fmt.Errorf("account %s / %s is not resolved", l.Account, l.Currency)
	}
	slot := accountSlot{actor: l.Actor, pair: pair, kind: accountKindOf(l.AccountType)}
	got, ok := r.rows[slot]
	if !ok {
		credit, debitRow, err := r.c.accountRows(ctx, r.p.byKey[l.Actor].id, slot)
		if err != nil {
			return "", fmt.Errorf("%s account %s / %s of %q: %w", slot.kind, l.Account, l.Currency, l.Actor, err)
		}
		got = [2]string{credit, debitRow}
		r.rows[slot] = got
	}
	if debit {
		return got[1], nil
	}
	return got[0], nil
}

// applyLedger runs phase b and answers whether the ledger is the legacy one.
func (c *Client) applyLedger(ctx context.Context, p *ontologyPlan) ([]OntologyPostingOut, bool, error) {
	r := &ledgerRun{c: c, p: p, rows: map[accountSlot][2]string{}, out: p.plannedPostings("")}
	err := r.transactions(ctx)
	if err == nil {
		err = r.transfers(ctx)
	}
	if errors.Is(err, errLegacyLedger) {
		for i := range r.out {
			if r.out[i].Status == "plan" {
				r.out[i].Status = "skipped"
			}
		}
		return r.out, true, nil
	}
	return r.out, false, err
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// transactions posts the one-sided events not posted yet, all-or-nothing in one atom call.
func (r *ledgerRun) transactions(ctx context.Context) error {
	ts := r.p.in.Transactions
	if len(ts) == 0 {
		return nil
	}
	accounts := make([]string, len(ts))
	lookups := make([]any, len(ts))
	for i, t := range ts {
		ref := r.out[i].Ref
		row, err := r.row(ctx, t.OntologyLeg, t.Side == "debit")
		if err != nil {
			return err
		}
		accounts[i] = row
		lookups[i] = map[string]any{"accId": r.c.workspace(), "accountId": row, "ref": ref}
	}
	found, err := r.c.eachItems(ctx, "transaction/by-ref/get", lookups)
	if err != nil {
		return err
	}
	base := 0
	var legs []map[string]any
	var pending []int
	for i, t := range ts {
		var got struct {
			Transaction struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"transaction"`
		}
		if e := found[i].err; e != nil && e.Reason != "notFound" && e.Reason != "noFoundOrNoAccess" {
			// Unsure whether it was posted: posting again could post it twice.
			return r.refused(e, []int{base + i})
		}
		if found[i].err == nil && json.Unmarshal(found[i].raw, &got) == nil && got.Transaction.ID > 0 {
			o := &r.out[base+i]
			o.Status, o.ID = "exists", strconv.FormatInt(got.Transaction.ID, 10)
			if got.Transaction.Type == "declined" {
				o.Status, o.Error = "failed", "declined earlier (past the account's limits); its ref is spent: post it under a new key"
			}
			continue
		}
		leg := map[string]any{"accountId": accounts[i], "amount": t.Amount, "ref": r.out[base+i].Ref}
		if t.Comment != "" {
			leg["comment"] = t.Comment
		}
		legs, pending = append(legs, leg), append(pending, base+i)
	}
	if len(legs) == 0 {
		return nil
	}
	posted, err := call[map[string]any, struct {
		List []struct {
			ID   int64  `json:"id"`
			Ref  string `json:"ref"`
			Type string `json:"type"`
		} `json:"list"`
	}](ctx, r.c, "transaction/atom/create", map[string]any{"accId": r.c.workspace(), "transactions": legs})
	if err != nil {
		return r.refused(err, pending)
	}
	for j, i := range pending {
		r.out[i].Status = "posted"
		if j < len(posted.List) {
			r.out[i].ID = strconv.FormatInt(posted.List[j].ID, 10)
			if posted.List[j].Type == "declined" {
				r.out[i].Status, r.out[i].Error = "failed", "declined: past the account's limits; nothing moved"
			}
		}
	}
	return nil
}

func sideOf(side string) string {
	if side == "debit" {
		return "debit on"
	}
	return "credit on"
}

// refused marks the postings a ledger write did not make; a legacy ledger stops phase b quietly.
func (r *ledgerRun) refused(err error, pending []int) error {
	var api *APIError
	if asAPI(err, &api) && api.Reason == "ledgerWrongAPI" {
		return errLegacyLedger
	}
	for _, i := range pending {
		r.out[i].Status, r.out[i].Error = "failed", err.Error()
	}
	return fmt.Errorf("the structure is written, a posting was refused: %w", err)
}

type transferWire struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// transferByRef reads a transfer by its ref: transfer/by-ref/get answers {transfer: {...}}.
func (c *Client) transferByRef(ctx context.Context, ref string) (transferWire, error) {
	got, err := call[map[string]any, struct {
		Transfer transferWire `json:"transfer"`
	}](ctx, c, "transfer/by-ref/get", map[string]any{"accId": c.workspace(), "ref": ref})
	return got.Transfer, err
}

// transfers posts each exchange not posted yet, in patch order; a failure stops the rest.
func (r *ledgerRun) transfers(ctx context.Context) error {
	base := len(r.p.in.Transactions)
	for i, t := range r.p.in.Transfers {
		o := &r.out[base+i]
		if t.Hold != "" {
			// sim-api closes a hold in place: step 2 updates the step-1 transfer's type and keeps
			// no ref of its own, so a close is done once its hold is no longer authorized.
			done, err := r.closedHold(ctx, t, base+i)
			if err != nil {
				return err
			}
			if done {
				continue
			}
		}
		earlier, err := r.c.transferByRef(ctx, o.Ref)
		if err == nil && earlier.ID != "" {
			o.Status, o.ID = "exists", earlier.ID
			if earlier.Type == "declined" {
				o.Status, o.Error = "failed", "declined earlier (a leg past its account's limits); its ref is spent: post it under a new key"
			}
			continue
		}
		if err != nil && !isNotFound(err) {
			return r.refused(err, []int{base + i})
		}
		params, err := r.transferParams(ctx, t, o.Ref)
		if err != nil {
			o.Status, o.Error = "failed", err.Error()
			return fmt.Errorf("transfer %q: %w", t.Key, err)
		}
		made, err := call[map[string]any, struct {
			Transfer transferWire `json:"transfer"`
		}](ctx, r.c, "transfer/create", params)
		if err != nil {
			return r.refused(err, []int{base + i})
		}
		o.Status, o.ID = "posted", made.Transfer.ID
		if made.Transfer.Type == "declined" {
			o.Status, o.Error = "failed", "declined: a leg is past its account's limits; nothing moved"
		}
	}
	return nil
}

// isNotFound says a by-ref read found nothing (rather than failing).
func isNotFound(err error) bool {
	var api *APIError
	return asAPI(err, &api) && (api.Reason == "notFound" || api.Reason == "noFoundOrNoAccess")
}

// closedHold says whether the hold t closes is closed already: as t asks (exists), or otherwise
// (failed, nothing to do). A hold still authorized, or not posted yet, is for t to close.
func (r *ledgerRun) closedHold(ctx context.Context, t OntologyTransfer, i int) (bool, error) {
	o := &r.out[i]
	hold, err := r.c.transferByRef(ctx, postingRef(r.p.in.OperationID, t.Hold))
	switch {
	case err != nil && isNotFound(err):
		return false, nil
	case err != nil:
		return false, r.refused(err, []int{i})
	case hold.ID == "" || hold.Type == "authorized":
		return false, nil
	case hold.Type == t.Type:
		o.Status, o.ID = "exists", hold.ID
		return true, nil
	}
	o.Status, o.Error = "failed", fmt.Sprintf("hold %q is %s already, not %s", t.Hold, hold.Type, t.Type)
	return true, nil
}

func transferDetail(t OntologyTransfer) string {
	kind := "exchange"
	switch {
	case t.Hold != "":
		kind = t.Type + " hold " + t.Hold
	case t.Type == "authorized":
		kind = "hold"
	}
	from, to := "", ""
	if len(t.From) > 0 {
		from = t.From[0].Actor
	}
	if len(t.To) > 0 {
		to = t.To[0].Actor
	}
	return fmt.Sprintf("%s %s -> %s", kind, from, to)
}

// transferParams is transfer/create's input: from legs on debit rows, to legs on credit rows, each
// with its own ref; closing a hold names it and each leg's step-1 leg by its ref.
func (r *ledgerRun) transferParams(ctx context.Context, t OntologyTransfer, ref string) (map[string]any, error) {
	params := map[string]any{"accId": r.c.workspace(), "ref": ref}
	if t.Type != "" {
		params["type"] = t.Type
	}
	if t.Comment != "" {
		params["comment"] = t.Comment
	}
	from, to, hold := t.From, t.To, ""
	if t.Hold != "" {
		hold = postingRef(r.p.in.OperationID, t.Hold)
		step1, err := r.c.transferByRef(ctx, hold)
		if err != nil {
			return nil, fmt.Errorf("hold %q (ref %s) is not posted: %w", t.Hold, hold, err)
		}
		if step1.ID == "" {
			return nil, fmt.Errorf("hold %q (ref %s) is not posted", t.Hold, hold)
		}
		params["transferId"] = step1.ID
		from, to = r.p.legsOf(t)
	}
	legs := func(side string, list []OntologyLeg, debit bool) ([]map[string]any, error) {
		out := make([]map[string]any, len(list))
		for i, l := range list {
			row, err := r.row(ctx, l, debit)
			if err != nil {
				return nil, err
			}
			leg := map[string]any{"accountId": row, "ref": ref + ":" + side + strconv.Itoa(i)}
			if t.Type != "authorized" || side == "f" {
				leg["amount"] = l.Amount // a hold's to legs take none: step 1 holds, step 2 credits
			}
			if hold != "" {
				leg["parentRef"] = hold + ":" + side + strconv.Itoa(i)
			}
			out[i] = leg
		}
		return out, nil
	}
	var err error
	if params["from"], err = legs("f", from, true); err != nil {
		return nil, err
	}
	if params["to"], err = legs("t", to, false); err != nil {
		return nil, err
	}
	return params, nil
}
