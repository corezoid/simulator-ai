package main

import (
	"encoding/json"
	"fmt"
)

type accountRow struct {
	ID         string  `json:"id"`
	IncomeType string  `json:"incomeType"`
	Amount     float64 `json:"amount"`
}

// finance (console-full): a pair registered by name, opened on two actors, a transaction posted
// and read back by its ref, a transfer between them and read back by its ref.
func finance(c *client, stamp string) error {
	// New names per run: the pair is new, so the run's identity owns it whoever ran before.
	var pair struct {
		NameID     string `json:"nameId"`
		CurrencyID int64  `json:"currencyId"`
		Registered bool   `json:"registered"`
	}
	if err := c.tool("account_pair_create", map[string]any{"accountName": "AI smoke balance " + stamp, "currencyName": "AI smoke unit " + stamp, "precision": 2}, &pair); err != nil {
		return err
	}
	if !pair.Registered {
		return fmt.Errorf("a new pair %s / %d is not registered", pair.NameID, pair.CurrencyID)
	}
	// A repeat finds the same pair, now shared with the owner, and registers nothing.
	repeat := pair
	if err := c.tool("account_pair_create", map[string]any{"accountName": "AI smoke balance " + stamp, "currencyName": "AI smoke unit " + stamp}, &repeat); err != nil {
		return err
	}
	if repeat.NameID != pair.NameID || repeat.CurrencyID != pair.CurrencyID || repeat.Registered {
		return fmt.Errorf("a repeated account_pair_create returned %+v, want %+v unregistered", repeat, pair)
	}
	rows := map[string]map[string]string{} // actor -> incomeType -> account row id
	var actors []string
	for i := 1; i <= 2; i++ {
		var actor id
		if err := c.tool("layer_create", map[string]any{"title": fmt.Sprintf("AI smoke finance %d %s", i, stamp)}, &actor); err != nil {
			return err
		}
		var ignored json.RawMessage
		if err := c.tool("account_create", map[string]any{"actorId": actor.ID, "nameId": pair.NameID, "currencyId": pair.CurrencyID}, &ignored); err != nil {
			return err
		}
		var list struct {
			List []accountRow `json:"list"`
		}
		if err := c.tool("accounts_list", map[string]any{"actorId": actor.ID, "nameId": pair.NameID}, &list); err != nil {
			return err
		}
		rows[actor.ID] = map[string]string{}
		for _, r := range list.List {
			rows[actor.ID][r.IncomeType] = r.ID
		}
		if rows[actor.ID]["debit"] == "" || rows[actor.ID]["credit"] == "" {
			return fmt.Errorf("actor %s got rows %v, want a debit and a credit", actor.ID, list.List)
		}
		actors = append(actors, actor.ID)
	}
	debit, ref := rows[actors[0]]["debit"], "smoke-"+stamp
	var posted, again struct {
		Transaction struct {
			ID     any     `json:"id"`
			Amount float64 `json:"amount"`
		} `json:"transaction"`
	}
	if err := c.tool("transaction_create", map[string]any{"accountId": debit, "amount": 10, "ref": ref, "comment": "smoke"}, &posted); err != nil {
		return err
	}
	if err := c.tool("transaction_by_ref_get", map[string]any{"accountId": debit, "ref": ref}, &again); err != nil {
		return err
	}
	if fmt.Sprint(again.Transaction.ID) != fmt.Sprint(posted.Transaction.ID) || again.Transaction.Amount != 10 {
		return fmt.Errorf("transaction by ref %s is %v, posted %v", ref, again.Transaction, posted.Transaction)
	}
	var ignored json.RawMessage
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"account_get", map[string]any{"actorId": actors[0], "nameId": pair.NameID, "currencyId": pair.CurrencyID}},
		{"account_transactions_list", map[string]any{"accountId": debit, "limit": 5}},
		{"transactions_list", map[string]any{"actorId": actors[0], "nameId": pair.NameID, "currencyId": pair.CurrencyID, "limit": 5}},
		{"transfer_create", map[string]any{"ref": ref, "comment": "smoke", "from": []any{map[string]any{"accountId": debit, "amount": 5}}, "to": []any{map[string]any{"accountId": rows[actors[1]]["credit"], "amount": 5}}}},
		{"transfer_by_ref_get", map[string]any{"ref": ref, "with": []string{"legs"}}},
		{"transfers_filter", map[string]any{"ref": ref, "limit": 5}},
		{"currencies_list", map[string]any{"query": "AI smoke", "limit": 5}},
		{"account_names_list", map[string]any{"query": "AI smoke", "limit": 5}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	fmt.Println("OK finance pair", pair.NameID, pair.CurrencyID, "actors", actors[0], actors[1])
	return nil
}
