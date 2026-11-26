package simfake

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The knowledge-base surface: templates with fields, account names and currencies, accounts on
// actors, transactions and transfers with refs, saved filters, users and their twin actors. Only
// what the plugin's knowledge-base tools call, with sim-api's refusals they rely on.

// KB is the workspace's fixed data, set with Set before a test calls.
type KB struct {
	// Forms are templates: title and body ({"sections":[{"content":[fields]}]}).
	Forms map[int64]Form
	// Names and Currencies are the account dictionaries: name -> id.
	Names      map[string]string
	Currencies map[string]int64
	// FormAccounts are the pairs each template opens on its actors.
	FormAccounts map[int64][]FormAccount
	// SystemForms names system templates (ActorFilters, ...) by title.
	SystemForms map[string]int64
	// LegacyLedger refuses every ledger write as sim-api does for a workspace not in ledger_workspaces.
	LegacyLedger bool
	// Users are members (userId -> saId); Twins their twin actors (userId -> actor id).
	Users map[int64]int64
	Twins map[int64]string
	// Strict answers actor/get of an unknown actor notFound.
	Strict bool
	// BeforeRead runs on a twin actor/by-system_obj/get returns, after the answer is taken: a
	// concurrent writer that changes the twin between a save's first read and its write.
	BeforeRead func(*Actor)
	// NoViewUsers refuses user/list as sim-api does a member without the viewUsers permission.
	NoViewUsers bool
	// Unregistered are pairs ("<nameId>/<currencyId>") with no access rules: access_rule/account/list
	// answers notFound for them, as sim-api does for a pair nobody registered.
	Unregistered map[string]bool
}

// Form is a template.
type Form struct {
	Title string
	Body  string
}

// FormAccount is a pair a template opens.
type FormAccount struct {
	NameID     string
	CurrencyID int64
	Type       string
}

func newKB() KB {
	return KB{Forms: map[int64]Form{}, Names: map[string]string{}, Currencies: map[string]int64{}, FormAccounts: map[int64][]FormAccount{},
		SystemForms: map[string]int64{}, Users: map[int64]int64{}, Twins: map[int64]string{}, Unregistered: map[string]bool{}}
}

// Account is an actor's account of one pair and type: its credit and debit rows.
type Account struct {
	ActorID, NameID string
	CurrencyID      int64
	Type            string
	Credit, Debit   string
}

// Posting is a stored transaction leg.
type Posting struct {
	ID              int64
	AccountID, Ref  string
	Amount          float64
	TransferID, Typ string
}

// Transfer is a stored transfer.
type Transfer struct {
	ID, Ref, Type string
	Legs          int
}

type ledger struct {
	Accounts  []Account
	Postings  []Posting
	Transfers []Transfer
}

func newLedger() ledger { return ledger{} }

func (l ledger) clone() ledger {
	return ledger{Accounts: slices.Clone(l.Accounts), Postings: slices.Clone(l.Postings), Transfers: slices.Clone(l.Transfers)}
}

// LedgerSnapshot is the accounts, postings and transfers stored.
func (s *Server) LedgerSnapshot() ([]Account, []Posting, []Transfer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.Ledger.Accounts), slices.Clone(s.Ledger.Postings), slices.Clone(s.Ledger.Transfers)
}

// ActorData is a stored actor's title, description and data.
func (s *Server) ActorData(id string) (Actor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.Actors[id]
	if a == nil {
		return Actor{}, false
	}
	out := *a
	out.Data = maps.Clone(a.Data)
	return out, true
}

// AddActor stores an actor as if created earlier.
func (s *Server) AddActor(formID int64, title string, data map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := &Actor{ID: s.id("actor"), Title: title, FormID: formID, Data: data}
	s.Actors[a.ID] = a
	return a.ID
}

var kbMethods = map[string]func(*Server, json.RawMessage) (any, *rpcError){
	"form/get":                 (*Server).formGet,
	"form/list":                (*Server).formList,
	"actor/search/list":        (*Server).actorSearch,
	"actor/filter/list":        (*Server).actorFilter,
	"actor/filter/count":       (*Server).actorFilterCount,
	"actor/update":             (*Server).actorUpdate,
	"actor/by-system_obj/get":  (*Server).systemActor,
	"user/list":                (*Server).userList,
	"account_name/list":        func(s *Server, p json.RawMessage) (any, *rpcError) { return dictionary(p, s.KB.Names) },
	"account_currency/list":    func(s *Server, p json.RawMessage) (any, *rpcError) { return dictionary(p, s.KB.Currencies) },
	"account_form/list":        (*Server).formAccounts,
	"account_actor/create":     (*Server).accountCreate,
	"account_actor/get":        (*Server).accountGet,
	"transaction/by-ref/get":   (*Server).postingByRef,
	"transaction/atom/create":  (*Server).atomCreate,
	"transfer/by-ref/get":      (*Server).transferByRef,
	"transfer/create":          (*Server).transferCreate,
	"edge/by-node/delete":      (*Server).edgeDelete,
	"access_rule/account/list": (*Server).pairRules,
	"access_rule/actor/list":   (*Server).twinRules,
	"ontology/get": func(s *Server, _ json.RawMessage) (any, *rpcError) {
		ledger := "simapi"
		if s.LegacyLedger() {
			ledger = "legacy"
		}
		return map[string]any{"version": "v1", "ledgerWrite": ledger, "hasMore": false}, nil
	},
}

// twinRules answers a twin's access rules: its user owns it.
func (s *Server) twinRules(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ActorID string `json:"actorId"`
	}
	_ = json.Unmarshal(params, &p)
	list := []any{}
	for userID, twin := range s.KB.Twins {
		if twin == p.ActorID {
			list = append(list, map[string]any{"userId": userID, "saId": s.KB.Users[userID]})
		}
	}
	return map[string]any{"list": list}, nil
}

func (s *Server) pairRules(params json.RawMessage) (any, *rpcError) {
	var p struct {
		NameID     string `json:"nameId"`
		CurrencyID int64  `json:"currencyId"`
	}
	_ = json.Unmarshal(params, &p)
	if s.KB.Unregistered[fmt.Sprintf("%s/%d", p.NameID, p.CurrencyID)] {
		return nil, fail("notFound", "no access rules")
	}
	return map[string]any{"list": []any{map[string]any{"userId": 1, "privs": map[string]any{"view": true}}}}, nil
}

func (s *Server) edgeDelete(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Source, Target string
		EdgeTypeID     int64 `json:"edgeTypeId"`
	}
	_ = json.Unmarshal(params, &p)
	key := fmt.Sprintf("%s>%s|%d", p.Source, p.Target, p.EdgeTypeID)
	id, ok := s.EdgeKeys[key]
	if !ok {
		return nil, fail("notFound", "edge not found")
	}
	delete(s.Edges, id)
	delete(s.EdgeKeys, key)
	return map[string]any{}, nil
}

func (s *Server) formGet(params json.RawMessage) (any, *rpcError) {
	var p struct {
		FormID int64 `json:"formId"`
	}
	_ = json.Unmarshal(params, &p)
	f, ok := s.KB.Forms[p.FormID]
	if !ok {
		return nil, fail("notFound", "form not found")
	}
	return map[string]any{"id": p.FormID, "title": f.Title, "form": json.RawMessage(f.Body)}, nil
}

func (s *Server) formList(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(params, &p)
	list := []any{}
	for _, id := range slices.Sorted(maps.Keys(s.KB.Forms)) {
		if f := s.KB.Forms[id]; strings.Contains(strings.ToLower(f.Title), strings.ToLower(p.Query)) {
			list = append(list, map[string]any{"id": id, "title": f.Title})
		}
	}
	return map[string]any{"list": list}, nil
}

func (s *Server) actorsOf(formID int64, keep func(*Actor) bool) []any {
	var ids []string
	for id, a := range s.Actors {
		if (formID == 0 || a.FormID == formID) && keep(a) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	list := []any{}
	for _, id := range ids {
		a := s.Actors[id]
		list = append(list, map[string]any{"id": a.ID, "title": a.Title, "formId": a.FormID, "data": a.Data})
	}
	return list
}

func (s *Server) actorSearch(params json.RawMessage) (any, *rpcError) {
	var p struct {
		FormID int64  `json:"formId"`
		Query  string `json:"query"`
	}
	_ = json.Unmarshal(params, &p)
	return map[string]any{"list": s.actorsOf(p.FormID, func(a *Actor) bool {
		return strings.Contains(strings.ToLower(a.Title), strings.ToLower(p.Query))
	})}, nil
}

type filterParams struct {
	FormID        int64  `json:"formId"`
	FilterActorID string `json:"filterActorId"`
	Search        string `json:"search"`
	QueryData     struct {
		Or []map[string]struct {
			All []string `json:"$all"`
		} `json:"$or"`
	} `json:"queryData"`
}

// filterMatch reads a filter request: the saved filter's template, the search, and $or of
// {key: {$all: [...]}} terms on list-valued data keys.
func (s *Server) filterMatch(params json.RawMessage) (int64, func(*Actor) bool, *rpcError) {
	var p filterParams
	_ = json.Unmarshal(params, &p)
	formID := p.FormID
	if p.FilterActorID != "" {
		saved := s.Actors[p.FilterActorID]
		if saved == nil {
			return 0, nil, fail("notFound", "filter actor not found")
		}
		var f struct {
			FormID int64 `json:"formId"`
		}
		raw, _ := saved.Data["filter"].(string)
		_ = json.Unmarshal([]byte(raw), &f)
		formID = f.FormID
	}
	return formID, func(a *Actor) bool {
		if p.Search != "" && !strings.Contains(strings.ToLower(a.Title), strings.ToLower(p.Search)) {
			return false
		}
		if len(p.QueryData.Or) == 0 {
			return true
		}
		for _, term := range p.QueryData.Or {
			for key, cond := range term {
				held, _ := a.Data[key].([]any)
				for _, want := range cond.All {
					if slices.Contains(held, any(want)) {
						return true
					}
				}
			}
		}
		return false
	}, nil
}

func (s *Server) actorFilter(params json.RawMessage) (any, *rpcError) {
	formID, keep, err := s.filterMatch(params)
	if err != nil {
		return nil, err
	}
	return map[string]any{"list": s.actorsOf(formID, keep)}, nil
}

func (s *Server) actorFilterCount(params json.RawMessage) (any, *rpcError) {
	formID, keep, err := s.filterMatch(params)
	if err != nil {
		return nil, err
	}
	return map[string]any{"count": len(s.actorsOf(formID, keep))}, nil
}

func (s *Server) actorUpdate(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ActorID     string         `json:"actorId"`
		Data        map[string]any `json:"data"`
		Description *string        `json:"description"`
	}
	_ = json.Unmarshal(params, &p)
	a := s.Actors[p.ActorID]
	if a == nil {
		return nil, fail("notFound", "actor not found")
	}
	if a.Data == nil {
		a.Data = map[string]any{}
	}
	maps.Copy(a.Data, p.Data) // a merge, as sim-api's: a key sent null is stored null
	if p.Description != nil {
		a.Description = *p.Description
	}
	return map[string]any{"id": a.ID}, nil
}

func (s *Server) systemActor(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ObjType string `json:"objType"`
		ObjID   string `json:"objId"`
	}
	_ = json.Unmarshal(params, &p)
	id, _ := strconv.ParseInt(p.ObjID, 10, 64)
	a := s.Actors[s.KB.Twins[id]]
	if p.ObjType != "user" || a == nil {
		return nil, fail("notFound", "system actor not found")
	}
	out := map[string]any{"id": a.ID, "title": a.Title, "description": a.Description, "data": maps.Clone(a.Data)}
	if s.KB.BeforeRead != nil {
		s.KB.BeforeRead(a) // a writer that changes the twin after this read
	}
	return out, nil
}

func (s *Server) userList(json.RawMessage) (any, *rpcError) {
	if s.KB.NoViewUsers {
		return nil, fail("accessDenied", "viewUsers required")
	}
	list := []any{}
	for _, id := range slices.Sorted(maps.Keys(s.KB.Users)) {
		list = append(list, map[string]any{"id": id, "saId": s.KB.Users[id]})
	}
	return map[string]any{"list": list}, nil
}

func dictionary[ID any](params json.RawMessage, entries map[string]ID) (any, *rpcError) {
	var p struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(params, &p)
	list := []any{}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if strings.Contains(strings.ToLower(name), strings.ToLower(p.Query)) {
			list = append(list, map[string]any{"id": entries[name], "name": name})
		}
	}
	return map[string]any{"list": list}, nil
}

func (s *Server) formAccounts(params json.RawMessage) (any, *rpcError) {
	var p struct {
		FormID int64 `json:"formId"`
	}
	_ = json.Unmarshal(params, &p)
	list := []any{}
	for _, b := range s.KB.FormAccounts[p.FormID] {
		list = append(list, map[string]any{"nameId": b.NameID, "currencyId": b.CurrencyID, "type": b.Type})
	}
	return map[string]any{"list": list}, nil
}

type accountKey struct {
	ActorID    string `json:"actorId"`
	NameID     string `json:"nameId"`
	CurrencyID int64  `json:"currencyId"`
	Type       string `json:"accountType"`
}

func (s *Server) account(k accountKey) *Account {
	if k.Type == "" {
		k.Type = "fact"
	}
	for i, a := range s.Ledger.Accounts {
		if a.ActorID == k.ActorID && a.NameID == k.NameID && a.CurrencyID == k.CurrencyID && a.Type == k.Type {
			return &s.Ledger.Accounts[i]
		}
	}
	return nil
}

func rows(a *Account, existed bool) map[string]any {
	return map[string]any{"existed": existed, "list": []any{map[string]any{"id": a.Credit, "incomeType": "credit"}, map[string]any{"id": a.Debit, "incomeType": "debit"}}}
}

func (s *Server) accountCreate(params json.RawMessage) (any, *rpcError) {
	var k accountKey
	_ = json.Unmarshal(params, &k)
	if s.Actors[k.ActorID] == nil {
		return nil, fail("notFound", "actor not found")
	}
	if a := s.account(k); a != nil {
		return rows(a, true), nil
	}
	if k.Type == "" {
		k.Type = "fact"
	}
	a := Account{ActorID: k.ActorID, NameID: k.NameID, CurrencyID: k.CurrencyID, Type: k.Type, Credit: s.id("credit"), Debit: s.id("debit")}
	s.Ledger.Accounts = append(s.Ledger.Accounts, a)
	return rows(&a, false), nil
}

func (s *Server) accountGet(params json.RawMessage) (any, *rpcError) {
	var k accountKey
	_ = json.Unmarshal(params, &k)
	if a := s.account(k); a != nil {
		return rows(a, true), nil
	}
	return map[string]any{"list": []any{}}, nil
}

func (s *Server) postingByRef(params json.RawMessage) (any, *rpcError) {
	var p struct {
		AccountID string `json:"accountId"`
		Ref       string `json:"ref"`
	}
	_ = json.Unmarshal(params, &p)
	for _, t := range s.Ledger.Postings {
		if t.AccountID == p.AccountID && t.Ref == p.Ref {
			return map[string]any{"transaction": map[string]any{"id": t.ID, "ref": t.Ref}}, nil
		}
	}
	return nil, fail("notFound", "transaction not found")
}

func wrongLedger() *rpcError {
	e := fail("ledgerWrongAPI", "wrong financial API: this workspace is still handled by the legacy API; send financial write requests to the legacy transactions/transfers endpoints")
	e.Data.ReasonParams = map[string]any{"requiredApi": "legacy"}
	return e
}

type legIn struct {
	AccountID string   `json:"accountId"`
	Amount    *float64 `json:"amount"`
	Ref       string   `json:"ref"`
	ParentRef string   `json:"parentRef"`
}

// post stores legs, refusing a repeated (account, ref) as sim-api does (trs005).
func (s *Server) post(legs []legIn, transfer, typ string) ([]any, *rpcError) {
	out := []any{}
	for _, l := range legs {
		for _, t := range s.Ledger.Postings {
			if l.Ref != "" && t.AccountID == l.AccountID && t.Ref == l.Ref {
				return nil, fail("trs005", "duplicate transaction ref")
			}
		}
		amount := 0.0
		if l.Amount != nil {
			amount = *l.Amount
		}
		s.Seq++
		p := Posting{ID: s.Seq, AccountID: l.AccountID, Ref: l.Ref, Amount: amount, TransferID: transfer, Typ: typ}
		s.Ledger.Postings = append(s.Ledger.Postings, p)
		out = append(out, map[string]any{"id": p.ID, "ref": p.Ref, "type": "completed", "accountId": p.AccountID})
	}
	return out, nil
}

func (s *Server) atomCreate(params json.RawMessage) (any, *rpcError) {
	if s.LegacyLedger() {
		return nil, wrongLedger()
	}
	var p struct {
		Transactions []legIn `json:"transactions"`
	}
	_ = json.Unmarshal(params, &p)
	before := s.Ledger.clone()
	list, err := s.post(p.Transactions, "", "completed")
	if err != nil {
		s.Ledger = before
		return nil, err
	}
	return map[string]any{"list": list}, nil
}

func (s *Server) transferByRef(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Ref string `json:"ref"`
	}
	_ = json.Unmarshal(params, &p)
	for _, t := range s.Ledger.Transfers {
		if t.Ref == p.Ref {
			return map[string]any{"transfer": map[string]any{"id": t.ID, "ref": t.Ref, "type": t.Type}}, nil
		}
	}
	return nil, fail("notFound", "transfer not found")
}

func (s *Server) transferCreate(params json.RawMessage) (any, *rpcError) {
	if s.LegacyLedger() {
		return nil, wrongLedger()
	}
	var p struct {
		Ref        string  `json:"ref"`
		Type       string  `json:"type"`
		TransferID string  `json:"transferId"`
		From       []legIn `json:"from"`
		To         []legIn `json:"to"`
	}
	_ = json.Unmarshal(params, &p)
	if len(p.From)+len(p.To) == 0 {
		return nil, fail("invalidParams", "transfer must have 1..20 legs per side")
	}
	for _, t := range s.Ledger.Transfers {
		if p.Ref != "" && t.Ref == p.Ref && p.TransferID == "" {
			return nil, fail("alreadyExists", "transfer ref is taken")
		}
	}
	if p.Type == "" {
		p.Type = "completed"
	}
	if p.TransferID != "" {
		return s.closeTransfer(p.TransferID, p.Type, append(append([]legIn{}, p.From...), p.To...))
	}
	before := s.Ledger.clone()
	id := s.id("transfer")
	if _, err := s.post(append(append([]legIn{}, p.From...), p.To...), id, p.Type); err != nil {
		s.Ledger = before
		return nil, err
	}
	s.Ledger.Transfers = append(s.Ledger.Transfers, Transfer{ID: id, Ref: p.Ref, Type: p.Type, Legs: len(p.From) + len(p.To)})
	return map[string]any{"transfer": map[string]any{"id": id, "ref": p.Ref, "type": p.Type}}, nil
}

// closeTransfer is step 2 as sim-api makes it: the step-1 transfer itself changes its type (no new
// header, no ref of its own); a transfer no longer authorized is refused.
func (s *Server) closeTransfer(id, typ string, legs []legIn) (any, *rpcError) {
	for i, t := range s.Ledger.Transfers {
		if t.ID != id {
			continue
		}
		if t.Type != "authorized" {
			return nil, fail("invalidParams", "transfer is closed")
		}
		before := s.Ledger.clone()
		if _, err := s.post(legs, id, typ); err != nil {
			s.Ledger = before
			return nil, err
		}
		s.Ledger.Transfers[i].Type = typ
		s.Ledger.Transfers[i].Legs += len(legs)
		return map[string]any{"transfer": map[string]any{"id": id, "ref": t.Ref, "type": typ}}, nil
	}
	return nil, fail("invalidParams", "no transfer by id")
}

// LegacyLedger reads the knob under the lock the request already holds.
func (s *Server) LegacyLedger() bool { return s.KB.LegacyLedger }

// TwinOf gives a member a twin actor (System template 9).
func (s *Server) TwinOf(userID, saID int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := &Actor{ID: s.id("twin"), Title: fmt.Sprintf("user %d", userID), FormID: 9, Data: map[string]any{}}
	s.Actors[a.ID] = a
	s.KB.Users[userID], s.KB.Twins[userID] = saID, a.ID
	return a.ID
}

// DeclineTransfer marks the transfer of ref declined, as a limit that refused it would (call it
// inside Set: the lock is held).
func (s *Server) DeclineTransfer(ref string) {
	for i, t := range s.Ledger.Transfers {
		if t.Ref == ref {
			s.Ledger.Transfers[i].Type = "declined"
		}
	}
}
