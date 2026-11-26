package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// The Graph Editor draws transfers on a layer only for the account pairs its settings name
// (data.accounts, the pairs layer_settings_update writes): from those it reads the transfersAmount_*
// edges a transfer leaves (W8). A patch with transfers and a layer therefore adds the transfers'
// pairs to the layer's accounts, keeping the ones already there, so what was posted shows.

// layerAccountsMissing are the stored entries for the transfers' pairs the layer does not show yet,
// and every entry it already has, in its order.
func (c *Client) layerAccountsMissing(ctx context.Context, p *ontologyPlan, layerID string) (have []json.RawMessage, add []any, err error) {
	if len(p.in.Transfers) == 0 {
		return nil, nil, nil
	}
	layer, err := call[map[string]any, struct {
		Data struct {
			Accounts json.RawMessage `json:"accounts"`
		} `json:"data"`
	}](ctx, c, "actor/get", map[string]any{"accId": c.workspace(), "actorId": layerID, "fields": []string{"id", "data"}})
	if err != nil {
		return nil, nil, err
	}
	shown := map[string]bool{}
	if len(layer.Data.Accounts) > 0 && string(layer.Data.Accounts) != "null" {
		if json.Unmarshal(layer.Data.Accounts, &have) != nil {
			return nil, nil, fmt.Errorf("the layer's data.accounts is not a list")
		}
	}
	for _, e := range have {
		var entry struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal(e, &entry)
		shown[entry.Value] = true
	}
	for _, t := range p.in.Transfers {
		for _, l := range append(append([]OntologyLeg{}, t.From...), t.To...) {
			name, currency := trimmed(l.Account), trimmed(l.Currency)
			pair := p.pairs[[2]string{name, currency}]
			if pair.NameID == "" {
				continue // a close names no legs; an unresolved pair is a problem already
			}
			value := pair.NameID + "_" + strconv.FormatInt(pair.CurrencyID, 10)
			if shown[value] {
				continue
			}
			shown[value] = true
			add = append(add, map[string]any{"value": value, "title": name + ", " + currency,
				"nameId": pair.NameID, "accountName": name, "currencyId": pair.CurrencyID, "currencyName": currency})
		}
	}
	if len(have)+len(add) > maxLayerAccounts {
		return nil, nil, fmt.Errorf("the layer already shows %d account pairs; %d more would pass the %d it draws: add them with layer_settings_update", len(have), len(add), maxLayerAccounts)
	}
	return have, add, nil
}

// showTransfers adds the transfers' pairs to the layer's accounts (as layer_settings_update writes
// them: the old accountName x currency keys are cleared) and answers their titles; on a dry run it
// only answers what it would add.
func (c *Client) showTransfers(ctx context.Context, p *ontologyPlan, layerID string, dry bool) ([]string, error) {
	have, add, err := c.layerAccountsMissing(ctx, p, layerID)
	if err != nil || len(add) == 0 {
		return nil, err
	}
	titles := make([]string, len(add))
	for i, e := range add {
		titles[i] = e.(map[string]any)["title"].(string)
	}
	if dry {
		return titles, nil
	}
	accounts := make([]any, 0, len(have)+len(add))
	for _, e := range have {
		accounts = append(accounts, e)
	}
	accounts = append(accounts, add...)
	data := map[string]any{"accounts": accounts, "accountName": nil, "currency": nil}
	if _, err = call[map[string]any, named](ctx, c, "actor/update", map[string]any{"accId": c.workspace(), "actorId": layerID, "data": data, "fields": []string{"id"}}); err != nil {
		return nil, fmt.Errorf("actor/update: %w", err)
	}
	return titles, nil
}
