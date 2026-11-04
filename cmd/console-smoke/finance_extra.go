package main

import (
	"encoding/json"
	"fmt"
)

// financeExtra (console-full, sim-api cda5b60e or later): on the pair of finance, an existing pair
// opened again with ignoreIfExist, a row read by id, a balance set, a formula previewed, set and
// read, two transactions posted atomically, a tag added and the set replaced as read, and the
// trigger lists of the pair, the actor's account and a data field.
func financeExtra(c *client, stamp, nameID string, currencyID int64, actors []string, rows map[string]map[string]string) error {
	var reopened struct {
		Existed bool `json:"existed"`
	}
	if err := c.tool("account_create", map[string]any{"actorId": actors[0], "nameId": nameID, "currencyId": currencyID, "ignoreIfExist": true}, &reopened); err != nil {
		return err
	}
	if !reopened.Existed {
		return fmt.Errorf("account_create with ignoreIfExist on an open pair did not answer existed")
	}
	credit, source, target := rows[actors[0]]["credit"], rows[actors[0]]["debit"], rows[actors[1]]["debit"]
	var row struct {
		List []accountRow `json:"list"`
	}
	if err := c.tool("account_amount_set", map[string]any{"id": credit, "amount": 7}, new(json.RawMessage)); err != nil {
		return err
	}
	if err := c.tool("account_by_id_get", map[string]any{"accountId": credit, "with": []string{"meta"}}, &row); err != nil {
		return err
	}
	if len(row.List) != 1 || row.List[0].Amount != 7 {
		return fmt.Errorf("account %s reads %v after account_amount_set 7", credit, row)
	}
	formula := source + " * 2"
	var preview struct {
		OK bool `json:"ok"`
	}
	if err := c.tool("account_formula_preview", map[string]any{"id": target, "formula": formula}, &preview); err != nil {
		return err
	}
	if !preview.OK {
		return fmt.Errorf("formula %q on %s does not validate", formula, target)
	}
	var ignored json.RawMessage
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"account_formula_set", map[string]any{"id": target, "formula": formula}},
		{"account_formula_get", map[string]any{"id": target}},
		{"atom_transaction_create", map[string]any{"transactions": []any{
			map[string]any{"accountId": source, "amount": 1, "ref": "atom-d-" + stamp},
			map[string]any{"accountId": credit, "amount": 1, "ref": "atom-c-" + stamp},
		}}},
		{"account_tag_add", map[string]any{"nameId": nameID, "currencyId": currencyID, "title": "AI smoke " + stamp}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	var tags struct {
		List []struct {
			ID string `json:"id"`
		} `json:"list"`
	}
	if err := c.tool("account_tags_list", map[string]any{"nameId": nameID, "currencyId": currencyID}, &tags); err != nil {
		return err
	}
	if len(tags.List) == 0 {
		return fmt.Errorf("the pair lists no tag after account_tag_add")
	}
	keep := []string{}
	for _, t := range tags.List {
		keep = append(keep, t.ID)
	}
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"account_tags_set", map[string]any{"nameId": nameID, "currencyId": currencyID, "tags": keep}},
		{"account_actor_tags_list", map[string]any{"actorId": actors[0]}},
		{"account_actor_tags_count", map[string]any{"actorId": actors[0]}},
		{"account_triggers_list", map[string]any{"nameId": nameID, "currencyId": currencyID}},
		{"account_actor_triggers_list", map[string]any{"actorId": actors[0], "nameId": nameID, "currencyId": currencyID}},
		{"actor_field_triggers_list", map[string]any{"actorId": actors[0], "dataField": "description"}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	if err := counterAccount(c, stamp, nameID, currencyID); err != nil {
		return err
	}
	fmt.Println("OK finance extras tags", len(keep), "formula on", target)
	return nil
}

// counterAccount opens the pair as a counter account on a new actor and reads its counterType back.
func counterAccount(c *client, stamp, nameID string, currencyID int64) error {
	var actor id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke counter " + stamp}, &actor); err != nil {
		return err
	}
	var opened struct {
		List []struct {
			ID string `json:"id"`
		} `json:"list"`
	}
	if err := c.tool("account_create", map[string]any{"actorId": actor.ID, "nameId": nameID, "currencyId": currencyID, "counterType": "counter"}, &opened); err != nil {
		return err
	}
	if len(opened.List) == 0 {
		return fmt.Errorf("account_create returned no rows for the counter account")
	}
	var row struct {
		List []struct {
			CounterType string `json:"counterType"`
		} `json:"list"`
	}
	if err := c.tool("account_by_id_get", map[string]any{"accountId": opened.List[0].ID}, &row); err != nil {
		return err
	}
	if len(row.List) != 1 || row.List[0].CounterType != "counter" {
		return fmt.Errorf("the counter account reads %+v, want counterType counter", row.List)
	}
	return nil
}
