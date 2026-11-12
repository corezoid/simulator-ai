package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// AccountPairInput names a workspace account pair by its account name and currency, created when
// missing (legacy createAccountPair).
type AccountPairInput struct {
	AccountName  string `json:"accountName"`
	CurrencyName string `json:"currencyName"`
	Symbol       string `json:"symbol,omitempty"`    // only for a new currency
	Precision    *int64 `json:"precision,omitempty"` // only for a new currency
	Type         string `json:"type,omitempty"`      // only for a new currency
	// TakeOwnership registers a pair whose name and currency exist but which nobody was given
	// access to: the caller becomes its only subject, and others stop seeing its balances.
	TakeOwnership bool `json:"takeOwnership,omitempty"`
}

// AccountPair is a registered pair: the ids account_create and the transaction tools take.
// Registered says the pair was new (or taken over), so the caller now owns it; otherwise the
// caller's access is what the pair's rules already give.
type AccountPair struct {
	NameID     string `json:"nameId"`
	CurrencyID int64  `json:"currencyId"`
	Registered bool   `json:"registered"`
}

type dictionaryEntry[ID any] struct {
	List []struct {
		ID   ID     `json:"id"`
		Name string `json:"name"`
	} `json:"list"`
}

// CreateAccountPair finds the account name and the currency by name, creates the missing ones, and
// registers the pair. A pair of an existing name and currency with no access rules is not claimed
// unless takeOwnership says so: registering it would make the caller its only subject.
func (c *Client) CreateAccountPair(ctx context.Context, in AccountPairInput) (AccountPair, error) {
	if e := consoleOnly(c); e != nil {
		return AccountPair{}, e
	}
	if strings.TrimSpace(in.AccountName) == "" || utf8.RuneCountInString(in.AccountName) > 255 || strings.TrimSpace(in.CurrencyName) == "" || utf8.RuneCountInString(in.CurrencyName) > 255 {
		return AccountPair{}, errors.New("an accountName and a currencyName of 1..255 characters required")
	}
	ws := c.workspace()
	nameID, err := lookup[string](ctx, c, "account_name/list", in.AccountName)
	if err != nil {
		return AccountPair{}, fmt.Errorf("account_name/list: %w", err)
	}
	currencyID, err := lookup[int64](ctx, c, "account_currency/list", in.CurrencyName)
	if err != nil {
		return AccountPair{}, fmt.Errorf("account_currency/list: %w", err)
	}
	existed := nameID != "" && currencyID != 0
	if nameID == "" {
		created, err := call[map[string]any, struct {
			ID string `json:"id"`
		}](ctx, c, "account_name/create", map[string]any{"accId": ws, "name": in.AccountName})
		if err != nil {
			return AccountPair{}, fmt.Errorf("account_name/create: %w", err)
		}
		nameID = created.ID
	}
	if currencyID == 0 {
		currency := map[string]any{"accId": ws, "name": in.CurrencyName}
		if in.Symbol != "" {
			currency["symbol"] = in.Symbol
		}
		if in.Precision != nil {
			currency["precision"] = *in.Precision
		}
		if in.Type != "" {
			currency["type"] = in.Type
		}
		created, err := call[map[string]any, struct {
			ID int64 `json:"id"`
		}](ctx, c, "account_currency/create", currency)
		if err != nil {
			return AccountPair{}, fmt.Errorf("account_currency/create: %w", err)
		}
		currencyID = created.ID
	}
	if nameID == "" || currencyID < 1 {
		return AccountPair{}, errors.New("account_name/create or account_currency/create returned no id")
	}
	pair := AccountPair{NameID: nameID, CurrencyID: currencyID, Registered: !existed}
	if existed {
		// sim-api answers notFound for a pair with no rules, and accessDenied for a shared one the
		// caller is not a subject of.
		rules, err := call[map[string]any, accessList](ctx, c, "access_rule/account/list", map[string]any{"accId": ws, "nameId": nameID, "currencyId": currencyID})
		var api *APIError
		switch {
		case err != nil && !(errors.As(err, &api) && api.Reason == "notFound"):
			return AccountPair{}, fmt.Errorf("access_rule/account/list: %w", err)
		case err == nil && len(rules.List) > 0:
			return pair, nil // already shared: registering it again changes nothing
		case !in.TakeOwnership:
			return AccountPair{}, fmt.Errorf("the pair %s / %s exists and nobody was given access to it: its balances can only be read through each actor (nameId %s, currencyId %d with account_get or accounts_list), and nothing can be posted to it. Registering it makes you its only subject and hides it from everyone else: repeat with takeOwnership true only once the user agrees", in.AccountName, in.CurrencyName, nameID, currencyID)
		}
		pair.Registered = true
	}
	if _, err = call[map[string]any, named](ctx, c, "account/create", map[string]any{"accId": ws, "nameId": nameID, "currencyId": currencyID}); err != nil {
		return AccountPair{}, fmt.Errorf("account/create: %w", err)
	}
	return pair, nil
}

type named struct {
	ID any `json:"id"`
}

// lookup finds a dictionary entry (account name or currency) by its name: the exact one, else one
// that differs only in case (names are unique per workspace with case); the zero id means none.
func lookup[ID comparable](ctx context.Context, c *Client, method, name string) (ID, error) {
	var none, folded ID
	for offset := 0; offset < 1000; offset += 100 {
		page, err := call[map[string]any, dictionaryEntry[ID]](ctx, c, method, map[string]any{"accId": c.workspace(), "query": name, "limit": 100, "offset": offset})
		if err != nil {
			return none, err
		}
		for _, e := range page.List {
			switch {
			case e.Name == name:
				return e.ID, nil
			case folded == none && strings.EqualFold(e.Name, name):
				folded = e.ID
			}
		}
		if len(page.List) < 100 {
			break
		}
	}
	return folded, nil
}
