package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type opened struct {
	NameID     string `json:"nameId"`
	CurrencyID int64  `json:"currencyId"`
	Accounts   []struct {
		AccountType string `json:"accountType"`
		Credit      string `json:"credit"`
		Debit       string `json:"debit"`
		Existed     bool   `json:"existed"`
	} `json:"accounts"`
}

// core (console): the Console scenarios the core catalog gained: accounts opened on actors by name
// and a transfer between them, an Account Template whose existing actor gets its account, a form
// card pinned to an actor, and a p2p chat with another member reused on the second call.
func core(c *client, stamp string) error {
	account, currency := "AI smoke core "+stamp, "AI smoke coin "+stamp
	var actors []id
	var accounts []opened
	for i := 1; i <= 2; i++ {
		var actor id
		if err := c.tool("layer_create", map[string]any{"title": fmt.Sprintf("AI smoke core %d %s", i, stamp)}, &actor); err != nil {
			return err
		}
		var got opened
		if err := c.tool("account_create", map[string]any{"actorId": actor.ID, "accountName": account, "currencyName": currency, "plan": i == 1}, &got); err != nil {
			return err
		}
		if len(got.Accounts) != map[bool]int{true: 2, false: 1}[i == 1] || got.Accounts[0].Debit == "" || got.Accounts[0].Credit == "" {
			return fmt.Errorf("account_create on %s returned %+v", actor.ID, got)
		}
		actors, accounts = append(actors, actor), append(accounts, got)
	}
	again := opened{}
	if err := c.tool("account_create", map[string]any{"actorId": actors[1].ID, "accountName": account, "currencyName": currency}, &again); err != nil {
		return err
	}
	if len(again.Accounts) != 1 || !again.Accounts[0].Existed || again.Accounts[0].Debit != accounts[1].Accounts[0].Debit {
		return fmt.Errorf("a repeated account_create returned %+v, want the existing account", again)
	}
	var ignored json.RawMessage
	if err := c.tool("transfer_create", map[string]any{"ref": "core-" + stamp, "from": []any{map[string]any{"accountId": accounts[0].Accounts[0].Debit, "amount": 3}}, "to": []any{map[string]any{"accountId": accounts[1].Accounts[0].Credit, "amount": 3}}}, &ignored); err != nil {
		return err
	}
	if err := c.tool("accounts_list", map[string]any{"actorId": actors[1].ID, "nameId": again.NameID}, &ignored); err != nil {
		return err
	}
	var form struct {
		ID int64 `json:"id"`
	}
	if err := c.tool("form_create", map[string]any{"title": "AI smoke core template " + stamp, "isTemplate": true}, &form); err != nil {
		return err
	}
	var record id
	if err := c.tool("actor_create", map[string]any{"formId": form.ID, "title": "AI smoke core record " + stamp}, &record); err != nil {
		return err
	}
	var bound struct {
		BindingID int64 `json:"bindingId"`
		Opened    int   `json:"opened"`
		HadIt     int   `json:"hadIt"`
	}
	if err := c.tool("form_account_create", map[string]any{"formId": form.ID, "accountName": account, "currencyName": currency}, &bound); err != nil {
		return err
	}
	if bound.BindingID < 1 || bound.Opened+bound.HadIt != 1 {
		return fmt.Errorf("form_account_create returned %+v, want a binding and the existing record opened", bound)
	}
	var card struct {
		TargetID string `json:"targetId"`
		EdgeID   string `json:"edgeId"`
	}
	if err := c.tool("actor_attach", map[string]any{"actorId": actors[0].ID, "kind": "form", "formId": form.ID, "title": "AI smoke card " + stamp}, &card); err != nil {
		return err
	}
	if err := c.tool("actor_attach", map[string]any{"actorId": actors[0].ID, "kind": "actor", "targetId": actors[1].ID}, &ignored); err != nil {
		return err
	}
	if err := coreCells(c, actors[0].ID, actors[1].ID); err != nil {
		return err
	}
	chat, err := coreChat(c, actors[0].ID)
	if err != nil {
		return err
	}
	fmt.Println("OK core actors", actors[0].ID, actors[1].ID, "template", form.ID, "card", card.TargetID, "chat", chat)
	return nil
}

// coreChat opens a p2p chat with a member other than the credential's own user (the owner of an
// actor it created) and checks that a second call reuses it.
func coreChat(c *client, own string) (string, error) {
	var owner struct {
		UserID int64 `json:"userId"`
	}
	if err := c.tool("actor_get", map[string]any{"actorId": own, "fields": []string{"id", "userId"}}, &owner); err != nil {
		return "", err
	}
	var users struct {
		List []struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"list"`
	}
	if err := c.tool("users_search", map[string]any{"limit": 20}, &users); err != nil {
		return "", err
	}
	other := int64(0)
	for _, u := range users.List {
		if u.ID != owner.UserID && u.Type != "api" {
			other = u.ID
			break
		}
	}
	if other == 0 {
		fmt.Println("SKIP chat_open: no other member in the workspace")
		return "", nil
	}
	var first, second struct {
		ChatID  string `json:"chatId"`
		Created bool   `json:"created"`
	}
	if err := c.tool("chat_open", map[string]any{"type": "p2p", "userIds": []int64{other}, "selfUserId": owner.UserID}, &first); err != nil {
		return "", err
	}
	time.Sleep(2 * time.Second) // the reuse lookup reads a replica
	if err := c.tool("chat_open", map[string]any{"type": "p2p", "userIds": []int64{other}, "selfUserId": owner.UserID}, &second); err != nil {
		return "", err
	}
	if second.ChatID != first.ChatID || second.Created {
		return "", fmt.Errorf("chat_open did not reuse chat %s: %+v", first.ChatID, second)
	}
	var ignored json.RawMessage
	if err := c.tool("reaction_create", map[string]any{"rootActorId": first.ChatID, "type": "comment", "description": "smoke message", "notify": false}, &ignored); err != nil {
		return "", err
	}
	return first.ChatID, nil
}

// coreCells places an actor on a layer by its Graph Editor cell, moves it to another, and reads
// both cells back with layer_get.
func coreCells(c *client, layer, actor string) error {
	var placed struct {
		Nodes []struct {
			LaID int64  `json:"laId"`
			Cell string `json:"cell"`
		} `json:"nodes"`
	}
	if err := c.tool("layer_placements_create", map[string]any{"layerId": layer, "nodes": []any{map[string]any{"actorId": actor, "cell": "C, 7"}}}, &placed); err != nil {
		return err
	}
	if len(placed.Nodes) != 1 || placed.Nodes[0].Cell != "C, 7" {
		return fmt.Errorf("placed by cell C, 7: %+v", placed.Nodes)
	}
	for _, cell := range []string{"C, 7", "-B, -1"} {
		if cell != "C, 7" {
			var ignored json.RawMessage
			if err := c.tool("layer_positions_update", map[string]any{"layerId": layer, "items": []any{map[string]any{"laId": placed.Nodes[0].LaID, "cell": cell}}}, &ignored); err != nil {
				return err
			}
		}
		var view struct {
			Nodes []struct {
				LaID int64  `json:"laId"`
				Cell string `json:"cell"`
			} `json:"nodes"`
		}
		if err := c.tool("layer_get", map[string]any{"layerId": layer}, &view); err != nil {
			return err
		}
		if len(view.Nodes) != 1 || view.Nodes[0].Cell != cell {
			return fmt.Errorf("layer_get after moving to %s: %+v", cell, view.Nodes)
		}
	}
	return nil
}
