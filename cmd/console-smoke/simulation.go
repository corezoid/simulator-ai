package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// simulation (console-full): a shop with a balance pays a supplier on a new layer; the snapshot
// reads both actors, their edge and the balance, then a model is checked and run in two scenarios.
func simulation(c *client, stamp string) error {
	var ignored json.RawMessage
	var form struct {
		ID int64 `json:"id"`
	}
	if err := c.tool("form_create", map[string]any{"title": "AI smoke sim template " + stamp, "isTemplate": true}, &form); err != nil {
		return err
	}
	shop, supplier := "AI smoke shop "+stamp, "AI smoke supplier "+stamp
	var created struct {
		Actors []id `json:"actors"`
	}
	if err := c.tool("actors_create", map[string]any{"actors": []map[string]any{{"formId": form.ID, "title": shop}, {"formId": form.ID, "title": supplier}}}, &created); err != nil {
		return err
	}
	if len(created.Actors) != 2 {
		return fmt.Errorf("actors_create returned %d actors, want 2", len(created.Actors))
	}
	var layer, edge id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke simulation " + stamp}, &layer); err != nil {
		return err
	}
	if err := c.tool("edge_create", map[string]any{"source": created.Actors[0].ID, "target": created.Actors[1].ID}, &edge); err != nil {
		return err
	}
	var placed struct {
		Nodes []struct {
			LaID int64 `json:"laId"`
		} `json:"nodes"`
	}
	nodes := []map[string]any{{"actorId": created.Actors[0].ID, "position": map[string]any{"x": 0, "y": 0}}, {"actorId": created.Actors[1].ID, "position": map[string]any{"x": 0, "y": 200}}}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "nodes": nodes}, &placed); err != nil {
		return err
	}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer.ID, "edges": []map[string]any{{"edgeId": edge.ID, "laIdSource": placed.Nodes[0].LaID, "laIdTarget": placed.Nodes[1].LaID}}}, &ignored); err != nil {
		return err
	}
	account, unit, err := fundShop(c, stamp, created.Actors[0].ID)
	if err != nil {
		return err
	}
	var snap struct {
		Graph string `json:"graph"`
		YAML  string `json:"yaml"`
	}
	if err := c.tool("simulation_snapshot_get", map[string]any{"layerId": layer.ID}, &snap); err != nil {
		return err
	}
	if snap.Graph != "2 actors, 1 links, 1 accounts" || !strings.Contains(snap.YAML, account) || !strings.Contains(snap.YAML, "value: \"100\"") {
		return fmt.Errorf("snapshot %q does not hold the two actors, their edge and the balance of 100:\n%s", snap.Graph, snap.YAML)
	}
	model := fmt.Sprintf(`value_types: {%[1]q: {kind: decimal, scale: 2, conserved: true}}
refs: {shop: {title: %[2]q}, supplier: {title: %[3]q}}
params: {price: 10}
horizon: 2d
initial_events:
  - {event: pay, target: shop, every: 1d}
behaviors:
  %[4]q:
    pay:
      - transfer: {from: {actor: self, account: %[5]q}, to: {actor: refs.supplier, account: %[5]q}, amount: params.price}
metrics:
  shop: "refs.shop.acc('%[5]s')"
  supplier: "refs.supplier.acc('%[5]s')"
goals:
  solvent: "shop >= 0"
`, unit, shop, supplier, "AI smoke sim template "+stamp, account)
	scenarios := "- {name: base}\n- {name: price20, params: {price: 20}}\n"
	var check struct {
		OK     bool     `json:"ok"`
		Errors []string `json:"errors"`
	}
	if err := c.tool("simulation_check", map[string]any{"model": model, "scenarios": scenarios, "graph": snap.YAML}, &check); err != nil {
		return err
	}
	if !check.OK {
		return fmt.Errorf("simulation_check found errors: %v", check.Errors)
	}
	var run struct {
		OK    bool   `json:"ok"`
		Table string `json:"table"`
	}
	if err := c.tool("simulation_run", map[string]any{"model": model, "scenarios": scenarios, "layerId": layer.ID}, &run); err != nil {
		return err
	}
	// Payments at 0, 1d and 2d (the horizon is inclusive): 3 x 10 and 3 x 20 move to the supplier.
	if !run.OK || !strings.Contains(run.Table, "| base | completed | 3 | 70 | 30 |") || !strings.Contains(run.Table, "| price20 | completed | 3 | 40 | 60 |") {
		return fmt.Errorf("simulation_run table:\n%s", run.Table)
	}
	fmt.Println("OK simulation layer", layer.ID, "base 70/30, price20 40/60")
	return nil
}

// fundShop opens a new account pair on the shop and credits it 100; it returns the account and
// currency names, which name the account and its value type in the snapshot.
func fundShop(c *client, stamp, actorID string) (string, string, error) {
	account, unit := "AI smoke sim balance "+stamp, "AI smoke sim unit "+stamp
	var pair struct {
		NameID     string `json:"nameId"`
		CurrencyID int64  `json:"currencyId"`
	}
	if err := c.tool("account_pair_create", map[string]any{"accountName": account, "currencyName": unit, "precision": 2}, &pair); err != nil {
		return "", "", err
	}
	var ignored json.RawMessage
	if err := c.tool("account_create", map[string]any{"actorId": actorID, "nameId": pair.NameID, "currencyId": pair.CurrencyID}, &ignored); err != nil {
		return "", "", err
	}
	var list struct {
		List []accountRow `json:"list"`
	}
	if err := c.tool("accounts_list", map[string]any{"actorId": actorID, "nameId": pair.NameID}, &list); err != nil {
		return "", "", err
	}
	for _, r := range list.List {
		if r.IncomeType == "credit" {
			err := c.tool("transaction_create", map[string]any{"accountId": r.ID, "amount": 100, "ref": "sim-smoke-" + stamp}, &ignored)
			return account, unit, err
		}
	}
	return "", "", fmt.Errorf("shop %s has no credit row: %v", actorID, list.List)
}
