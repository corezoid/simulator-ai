package publicapi

import (
	"context"
	"errors"
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
}

// AccountPair is a registered pair: the ids account_create and the transaction tools take.
type AccountPair struct {
	NameID     string `json:"nameId"`
	CurrencyID int64  `json:"currencyId"`
}

type named struct {
	ID any `json:"id"`
}

// CreateAccountPair creates the account name and the currency when missing, then registers the
// pair, which makes the caller its owner. All three calls are idempotent by name, so a repeat
// returns the same ids.
func (c *Client) CreateAccountPair(ctx context.Context, in AccountPairInput) (AccountPair, error) {
	if e := consoleOnly(c); e != nil {
		return AccountPair{}, e
	}
	if !validTitle(in.AccountName, false) || utf8.RuneCountInString(in.AccountName) > 255 || !validTitle(in.CurrencyName, false) || utf8.RuneCountInString(in.CurrencyName) > 255 {
		return AccountPair{}, errors.New("an accountName and a currencyName of 1..255 characters required")
	}
	name, err := call[map[string]any, struct {
		ID string `json:"id"`
	}](ctx, c, "account_name/create", map[string]any{"accId": c.config.Workspace, "name": in.AccountName})
	if err != nil {
		return AccountPair{}, err
	}
	currency := map[string]any{"accId": c.config.Workspace, "name": in.CurrencyName}
	if in.Symbol != "" {
		currency["symbol"] = in.Symbol
	}
	if in.Precision != nil {
		currency["precision"] = *in.Precision
	}
	if in.Type != "" {
		currency["type"] = in.Type
	}
	cur, err := call[map[string]any, struct {
		ID int64 `json:"id"`
	}](ctx, c, "account_currency/create", currency)
	if err != nil {
		return AccountPair{}, err
	}
	if name.ID == "" || cur.ID < 1 {
		return AccountPair{}, errors.New("account_name/create or account_currency/create returned no id")
	}
	if _, err = call[map[string]any, named](ctx, c, "account/create", map[string]any{"accId": c.config.Workspace, "nameId": name.ID, "currencyId": cur.ID}); err != nil {
		return AccountPair{}, err
	}
	return AccountPair{NameID: name.ID, CurrencyID: cur.ID}, nil
}
