package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// PairRef names an account pair by its account name and currency ("Default", "USD") or by their ids.
type PairRef struct {
	AccountName  string `json:"accountName,omitempty"`
	CurrencyName string `json:"currencyName,omitempty"`
	NameID       string `json:"nameId,omitempty"`
	CurrencyID   int64  `json:"currencyId,omitempty"`
	// TakeOwnership registers an existing pair nobody was given access to (see AccountPairInput).
	TakeOwnership bool `json:"takeOwnership,omitempty"`
}

// AccountOpenInput opens an account pair on an actor as ui-kit's create-account block does.
type AccountOpenInput struct {
	ActorID string `json:"actorId"`
	PairRef
	AccountType string   `json:"accountType,omitempty"` // fact (default), plan, min, max, avg
	Plan        bool     `json:"plan,omitempty"`        // also open the plan twin of a fact pair
	CounterType string   `json:"counterType,omitempty"`
	MinLimit    *float64 `json:"minLimit,omitempty"`
	MaxLimit    *float64 `json:"maxLimit,omitempty"`
}

// OpenedAccount is one opened pair: its credit and debit row ids, the accountId of transactions.
type OpenedAccount struct {
	AccountType string `json:"accountType"`
	Credit      string `json:"credit"`
	Debit       string `json:"debit"`
	Existed     bool   `json:"existed,omitempty"`
}

type AccountsOpened struct {
	ActorID    string          `json:"actorId"`
	NameID     string          `json:"nameId"`
	CurrencyID int64           `json:"currencyId"`
	Registered bool            `json:"registered,omitempty"`
	Accounts   []OpenedAccount `json:"accounts"`
}

type accountRows struct {
	Existed bool `json:"existed"`
	List    []struct {
		ID         string `json:"id"`
		IncomeType string `json:"incomeType"`
	} `json:"list"`
}

// resolvePair turns a pair given by names into ids, creating a missing name or currency and
// registering the pair (CreateAccountPair); ids are taken as they are.
func (c *Client) resolvePair(ctx context.Context, p PairRef) (AccountPair, error) {
	byName, byID := p.AccountName != "" || p.CurrencyName != "", p.NameID != "" || p.CurrencyID != 0
	switch {
	case byName == byID:
		return AccountPair{}, errors.New("name the pair by accountName and currencyName, or by nameId and currencyId")
	case byID && (p.NameID == "" || p.CurrencyID < 1):
		return AccountPair{}, errors.New("nameId and a positive currencyId required")
	case byID:
		return AccountPair{NameID: p.NameID, CurrencyID: p.CurrencyID}, nil
	}
	return c.CreateAccountPair(ctx, AccountPairInput{AccountName: p.AccountName, CurrencyName: p.CurrencyName, TakeOwnership: p.TakeOwnership})
}

// OpenAccount is ui-kit's create-account save: one atomic batch that registers the pair
// (account/create, idempotent), opens it on the actor and, with plan, its planned twin. A pair the
// actor already has is answered as it is (existed).
func (c *Client) OpenAccount(ctx context.Context, in AccountOpenInput) (AccountsOpened, error) {
	if e := consoleOnly(c); e != nil {
		return AccountsOpened{}, e
	}
	if !validID(in.ActorID) {
		return AccountsOpened{}, errors.New("actorId required")
	}
	kind := in.AccountType
	if kind == "" {
		kind = "fact"
	}
	if in.Plan && kind != "fact" {
		return AccountsOpened{}, errors.New("plan opens the planned twin of a fact account: leave accountType fact")
	}
	pair, err := c.resolvePair(ctx, in.PairRef)
	if err != nil {
		return AccountsOpened{}, err
	}
	ws := c.config.Workspace
	base := map[string]any{"accId": ws, "nameId": pair.NameID, "currencyId": pair.CurrencyID}
	open := func(accountType string, limits bool) map[string]any {
		p := map[string]any{"accId": ws, "actorId": in.ActorID, "nameId": pair.NameID, "currencyId": pair.CurrencyID, "accountType": accountType, "ignoreIfExist": true}
		if accountType == "fact" || accountType == "plan" {
			p["search"] = true // sim-api reads an omitted search as false; ui-kit and legacy default to true
		}
		if in.CounterType != "" {
			p["counterType"] = in.CounterType
		}
		if limits && in.MinLimit != nil {
			p["minLimit"] = *in.MinLimit
		}
		if limits && in.MaxLimit != nil {
			p["maxLimit"] = *in.MaxLimit
		}
		return p
	}
	items := []atomicItem{{Method: "account/create", Params: base}, {Method: "account_actor/create", Params: open(kind, true)}}
	if in.Plan {
		items = append(items, atomicItem{Method: "account_actor/create", Params: open("plan", false)}) // legacy's twin has no limits
	}
	results, err := c.atomicItems(ctx, items)
	if err != nil {
		return AccountsOpened{}, err
	}
	out := AccountsOpened{ActorID: in.ActorID, NameID: pair.NameID, CurrencyID: pair.CurrencyID, Registered: pair.Registered}
	for i, r := range results[1:] {
		accountType := kind
		if i == 1 {
			accountType = "plan"
		}
		opened, err := openedRows(r, accountType)
		if err != nil {
			return AccountsOpened{}, err
		}
		out.Accounts = append(out.Accounts, opened)
	}
	return out, nil
}

func openedRows(raw json.RawMessage, accountType string) (OpenedAccount, error) {
	var rows accountRows
	if json.Unmarshal(raw, &rows) != nil {
		return OpenedAccount{}, errors.New("account_actor/create committed but returned no rows; read accounts_list before retrying")
	}
	out := OpenedAccount{AccountType: accountType, Existed: rows.Existed}
	for _, row := range rows.List {
		switch row.IncomeType {
		case "credit":
			out.Credit = row.ID
		case "debit":
			out.Debit = row.ID
		}
	}
	if out.Credit == "" || out.Debit == "" {
		return OpenedAccount{}, errors.New("account_actor/create committed without a credit and a debit row; read accounts_list before retrying")
	}
	return out, nil
}

// FormAccountInput binds an account pair to an account template (legacy createFormAccount): every
// actor of the template has it, the ones created later and, unless skipExisting, the existing ones.
type FormAccountInput struct {
	FormID int64 `json:"formId"`
	PairRef
	AccountType  string `json:"accountType,omitempty"` // fact (default) or plan
	MinLimit     *int64 `json:"minLimit,omitempty"`
	MaxLimit     *int64 `json:"maxLimit,omitempty"`
	SkipExisting bool   `json:"skipExisting,omitempty"`
	Offset       int64  `json:"offset,omitempty"` // where to resume opening it on existing actors
}

type FormAccountBound struct {
	FormID     int64  `json:"formId"`
	BindingID  int64  `json:"bindingId"`
	NameID     string `json:"nameId"`
	CurrencyID int64  `json:"currencyId"`
	WasBound   bool   `json:"wasBound,omitempty"`
	// Opened counts existing actors that got the pair now, HadIt those that had it already.
	Opened     int            `json:"opened"`
	HadIt      int            `json:"hadIt"`
	Failed     []ActorFailure `json:"failed,omitempty"`
	NextOffset int64          `json:"nextOffset,omitempty"`
}

type ActorFailure struct {
	ActorID string `json:"actorId"`
	Error   string `json:"error"`
}

// existingPages bounds one form_account_create call to 500 existing actors; nextOffset resumes.
const existingPages = 5

// BindFormAccount binds the pair to the template unless it already is (account_form/create binds
// only actors created from now on) and opens it on the template's existing actors you see.
func (c *Client) BindFormAccount(ctx context.Context, in FormAccountInput) (FormAccountBound, error) {
	if e := consoleOnly(c); e != nil {
		return FormAccountBound{}, e
	}
	kind := in.AccountType
	if kind == "" {
		kind = "fact"
	}
	if in.FormID < 1 || (kind != "fact" && kind != "plan") || in.Offset < 0 {
		return FormAccountBound{}, errors.New("a positive formId and accountType fact or plan required")
	}
	pair, err := c.resolvePair(ctx, in.PairRef)
	if err != nil {
		return FormAccountBound{}, err
	}
	ws := c.config.Workspace
	out := FormAccountBound{FormID: in.FormID, NameID: pair.NameID, CurrencyID: pair.CurrencyID}
	bound, err := call[map[string]any, struct {
		List []named `json:"list"`
	}](ctx, c, "account_form/list", map[string]any{"accId": ws, "formId": in.FormID, "nameId": pair.NameID, "currencyId": pair.CurrencyID, "accountType": kind, "limit": 1})
	if err != nil {
		return FormAccountBound{}, fmt.Errorf("account_form/list: %w", err)
	}
	if len(bound.List) > 0 {
		out.WasBound, out.BindingID = true, intID(bound.List[0].ID)
	} else {
		binding := map[string]any{"accId": ws, "formId": in.FormID, "nameId": pair.NameID, "currencyId": pair.CurrencyID, "accountType": kind, "search": true}
		if in.MinLimit != nil {
			binding["minLimit"] = *in.MinLimit
		}
		if in.MaxLimit != nil {
			binding["maxLimit"] = *in.MaxLimit
		}
		results, err := c.atomicItems(ctx, []atomicItem{{Method: "account/create", Params: map[string]any{"accId": ws, "nameId": pair.NameID, "currencyId": pair.CurrencyID}}, {Method: "account_form/create", Params: binding}})
		if err != nil {
			return FormAccountBound{}, err
		}
		var created named
		_ = json.Unmarshal(results[1], &created)
		out.BindingID = intID(created.ID)
	}
	if in.SkipExisting {
		return out, nil
	}
	return out, c.openOnExisting(ctx, in, pair, kind, &out)
}

// openOnExisting opens the pair on up to existingPages pages of the template's actors, each page
// one non-atomic batch, so an actor you may not modify fails alone.
func (c *Client) openOnExisting(ctx context.Context, in FormAccountInput, pair AccountPair, kind string, out *FormAccountBound) error {
	offset := in.Offset
	for page := 0; page < existingPages; page++ {
		actors, err := call[map[string]any, struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
		}](ctx, c, "actor/filter/list", map[string]any{"accId": c.config.Workspace, "formId": in.FormID, "fields": []string{"id"}, "orderBy": "createdAt", "orderValue": "ASC", "limit": 100, "offset": offset})
		if err != nil {
			return fmt.Errorf("actor/filter/list (the binding is saved; repeat with offset %d to open it on existing actors): %w", offset, err)
		}
		if len(actors.List) == 0 {
			return nil
		}
		items := make([]map[string]json.RawMessage, len(actors.List))
		for i, a := range actors.List {
			params := map[string]any{"actorId": a.ID, "nameId": pair.NameID, "currencyId": pair.CurrencyID, "accountType": kind, "search": true, "ignoreIfExist": true}
			if in.MinLimit != nil { // the binding's limits, as an actor created from the template gets them
				params["minLimit"] = *in.MinLimit
			}
			if in.MaxLimit != nil {
				params["maxLimit"] = *in.MaxLimit
			}
			items[i] = rawParams(params)
		}
		results, err := c.PassEach(ctx, "account_actor/create", items)
		if err != nil {
			return fmt.Errorf("account_actor/create (the binding is saved; repeat with offset %d): %w", offset, err)
		}
		for _, r := range results {
			var rows accountRows
			switch {
			case r.Error != "":
				out.Failed = append(out.Failed, ActorFailure{ActorID: actors.List[r.Index].ID, Error: r.Error})
			case json.Unmarshal(r.Result, &rows) == nil && rows.Existed:
				out.HadIt++
			default:
				out.Opened++
			}
		}
		offset += int64(len(actors.List))
		if len(actors.List) < 100 {
			return nil
		}
	}
	out.NextOffset = offset
	return nil
}

func rawParams(m map[string]any) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k], _ = json.Marshal(v)
	}
	return out
}

// intID reads a numeric id decoded into any.
func intID(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}
