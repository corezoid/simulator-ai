package mcpserver

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// knowledgeTools are the AI Console's knowledge-base reads on public sim-api methods: the
// workspace's ontology, one actor's card, walks and paths over chosen edge types, what points at an
// actor, an object's history and a whole tree.
var knowledgeTools = []parityTool{
	{Name: "ontology_get", Method: "ontology/get", Description: "Read the workspace's ontology FIRST, before reading or writing the graph; it is also THE list of edge types: id, name and role (hierarchy; dependency = the Gantt link directedInteraction, source = predecessor; transactions = money) with each role's meaning (fields: [\"edgeTypes\",\"edgeRoles\"]). Also templates (fields with ids and options, accounts, Corezoid processes), system templates and ledgerWrite (simapi: postings work; legacy: made in the web app). One page per call (hasMore, nextOffset); query/formIds narrow it, fields keeps sections; ifVersion answers notModified when nothing changed."},
	{Name: "entity_context_get", Method: "actor/context/get", Local: fullLocal, Around: contextCard, Description: "Everything about ONE actor in one call: its columns, data with field titles, neighbors by edge type and direction, accounts with balance (credit - debit), latest comments and changes, Corezoid processes. transfers sums the money edges (transfersAmount_*) per counterparty: who paid it or whom it paid, how much (transferIds: transfer_get {id, with: [legs]} for details; transferActorIds: Transfers actors you may not read). partial: the sum covers only the newest perType edges of that type — repeat with perType 50, then page graph_get {actorId, edgeTypeIds: [edgeTypeId]} and add up its transfers. payments: transfers of the last 366 days it is a party to or whose comment names its number (\"payment for contract №22\"): id, createdAt, from and to with amounts, comment. fields keeps sections (payments is one); full: values uncut, money edges as edges."},
	{Name: "graph_traverse", Method: "graph/traverse", Around: traverseAround, Description: "Walk the graph from 1-20 actors up to 3 steps along chosen edge types (ontology_get edgeTypes; hierarchy by default): direction down (children, successors, receivers), up, both. Gantt: what X depends on = direction up along the dependency type, what waits for X = down. Returns the reached nodes with depth, the edges (weight = the amount on a money edge, linkedActorId = its transfer), edgeTypes naming each type with its role, and countsByForm: the reached actors per template (start actors left out; hasMore: of those read) — answer \"how many\" from it, never by counting; denied: an actor you may not view stops the walk there."},
	{Name: "graph_path_find", Method: "graph/path/find", Around: withEdgeTypes, Description: "Find the shortest path (1-4 edges) between two actors along chosen edge types (hierarchy by default), either way unless directed: found, steps, nodes and edges in order, edgeTypes naming each type with its role."},
	{Name: "actor_backlinks_get", Method: "actor/backlinks/get", Description: "What points at an actor: incoming edges by type, and mentions — actors and comments whose text names it ([actor=<id>]), with a snippet and, for a comment, the actor it stands under (rootActorId, rootTitle). Use it for \"where is X mentioned\". AI Console dialogs and their messages are left out for an AI agent unless includeAiDialogs; page mentions with mentionsOffset."},
	{Name: "history_list", Method: "history/list", Around: historyAround, Describe: historyNote, Description: "Read an object's change history, newest first, in a window from (exclusive) and to (Unix ms): objType actor (field changes, links), formTemplate, templateActors or account; field narrows to one field (its id or title as on the card); the whole history: from 0, to now. limit up to 50, offset pages. What a value was on a date: the last change before it."},
	{Name: "graph_tree_get", Method: "graph/tree/get", Defaults: rowFields, Description: "Read an actor's whole subtree along a tree edge type (edgeTypeId: a type ontology_get marks isTree; hierarchy is not one: walk it with graph_traverse): nested children. id, title, formId per node unless you pass fields."},
}

// Context compaction: the card's long values are cut (full keeps them), and its money edges become
// one line per counterparty.
const (
	contextValueRunes = 300
	contextChangeRune = 200
	transfersPrefix   = "transfersAmount_"
)

// contextCard is the Around of entity_context_get: unless full, the card is compacted and its money
// edges summed, their transfers named by transfer id (one batch reads the Transfers actors' refs).
func contextCard(ctx context.Context, c *publicapi.Client, method string, args, local map[string]json.RawMessage) (json.RawMessage, error) {
	payments := wantsPayments(args)
	var actorID string
	_ = json.Unmarshal(args["actorId"], &actorID)
	v, err := c.Pass(ctx, method, args)
	if err == nil && payments {
		v = withPayments(ctx, c, actorID, v)
	}
	if err != nil || string(local["full"]) == "true" {
		return v, err
	}
	var card struct {
		Neighbors []struct {
			EdgeType  string `json:"edgeType"`
			Neighbors []struct {
				LinkedActorID string `json:"linkedActorId"`
			} `json:"neighbors"`
		} `json:"neighbors"`
	}
	var ids []string
	if json.Unmarshal(v, &card) == nil {
		for _, g := range card.Neighbors {
			if _, _, money := moneyPair(g.EdgeType); money {
				for _, n := range g.Neighbors {
					ids = append(ids, n.LinkedActorID)
				}
			}
		}
	}
	return compactContext(v, transferIDs(ctx, c, ids)), nil
}

// transferIDs names the transfers behind these Transfers actors; a failed read only leaves them
// as actors (the card still answers).
func transferIDs(ctx context.Context, c *publicapi.Client, actorIDs []string) map[string]string {
	seen := map[string]bool{}
	var ids []string
	for _, id := range actorIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	refs, err := c.TransferIDs(ctx, ids)
	if err != nil {
		return nil
	}
	return refs
}

// moneyPair splits a money edge type transfersAmount_<nameId>_<currencyId>; the name id may hold
// underscores itself, so the currency is the segment after the last one (as sim-api reads it).
func moneyPair(edgeType string) (nameID, currencyID string, ok bool) {
	rest, ok := strings.CutPrefix(edgeType, transfersPrefix)
	i := strings.LastIndex(rest, "_")
	if !ok || i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

func compactContext(v json.RawMessage, transfers map[string]string) json.RawMessage {
	var doc jsonObj
	if json.Unmarshal(v, &doc) != nil || doc == nil {
		return v
	}
	cutRows(doc, "data", []string{"value"}, contextValueRunes)
	cutRows(doc, "reactions", []string{"text"}, contextValueRunes)
	cutRows(doc, "history", []string{"prevValue", "nextValue"}, contextChangeRune)
	moneyEdges(doc, transfers)
	balances(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		return v
	}
	return out
}

// cutRows cuts the string values of these keys in every row of doc[key].
func cutRows(doc jsonObj, key string, fields []string, runes int) {
	var rows []jsonObj
	if json.Unmarshal(doc[key], &rows) != nil || rows == nil {
		return
	}
	for _, row := range rows {
		for _, f := range fields {
			var s string
			if json.Unmarshal(row[f], &s) == nil {
				if r := []rune(s); len(r) > runes {
					row[f], _ = json.Marshal(string(r[:runes]) + "…")
				}
			}
		}
	}
	doc[key], _ = json.Marshal(rows)
}

// counterparty is one line of the card's transfers: the money between the actor and one other
// actor on one account, in one direction.
type counterparty struct {
	Direction   string   `json:"direction"` // in: it paid this actor; out: this actor paid it
	ActorID     string   `json:"actorId"`
	Title       string   `json:"title,omitempty"`
	Amount      string   `json:"amount"`
	Account     string   `json:"account,omitempty"`
	Currency    string   `json:"currency,omitempty"`
	NameID      string   `json:"nameId,omitempty"`
	CurrencyID  string   `json:"currencyId,omitempty"`
	TransferIDs []string `json:"transferIds,omitempty"`
	// TransferActorIDs are the Transfers actors whose transfer id could not be read.
	TransferActorIDs []string `json:"transferActorIds,omitempty"`
	// Partial: the group had more edges than perType, so the amount is only the newest ones';
	// EdgeTypeID is the money type to page with graph_get.
	Partial    bool  `json:"partial,omitempty"`
	EdgeTypeID int64 `json:"edgeTypeId,omitempty"`
	sum        *big.Rat
}

// moneyEdges replaces the neighbor groups of transfersAmount_<nameId>_<currencyId> edge types
// with transfers: the summed weight per counterparty, account and direction, the transfers named by
// id through transfers (Transfers actor -> transfer id). A group with more edges than perType
// marks its lines partial (and transfersHasMore).
func moneyEdges(doc jsonObj, transfers map[string]string) {
	var groups []struct {
		EdgeType   string `json:"edgeType"`
		EdgeTypeID int64  `json:"edgeTypeId"`
		Direction  string `json:"direction"`
		HasMore    bool   `json:"hasMore"`
		Neighbors  []struct {
			ActorID       string `json:"actorId"`
			Title         string `json:"title"`
			Weight        string `json:"weight"`
			LinkedActorID string `json:"linkedActorId"`
		} `json:"neighbors"`
	}
	var raw []json.RawMessage
	if json.Unmarshal(doc["neighbors"], &raw) != nil || json.Unmarshal(doc["neighbors"], &groups) != nil || len(raw) != len(groups) {
		return
	}
	names, currencies := accountNames(doc)
	var kept []json.RawMessage
	var lines []*counterparty
	index := map[string]*counterparty{}
	more := false
	for i, g := range groups {
		nameID, currencyID, money := moneyPair(g.EdgeType)
		if !money {
			kept = append(kept, raw[i])
			continue
		}
		more = more || g.HasMore
		for _, n := range g.Neighbors {
			key := g.Direction + "|" + n.ActorID + "|" + g.EdgeType
			line := index[key]
			if line == nil {
				line = &counterparty{Direction: g.Direction, ActorID: n.ActorID, Title: n.Title, Account: names[nameID], Currency: currencies[currencyID], sum: new(big.Rat)}
				if line.Account == "" || line.Currency == "" {
					line.NameID, line.CurrencyID = nameID, currencyID
				}
				if g.HasMore {
					line.Partial, line.EdgeTypeID = true, g.EdgeTypeID
				}
				index[key] = line
				lines = append(lines, line)
			}
			if w, ok := new(big.Rat).SetString(n.Weight); ok {
				line.sum.Add(line.sum, w)
			}
			if id := transfers[n.LinkedActorID]; id != "" {
				line.TransferIDs = append(line.TransferIDs, id)
			} else if n.LinkedActorID != "" {
				line.TransferActorIDs = append(line.TransferActorIDs, n.LinkedActorID)
			}
		}
	}
	if len(lines) == 0 {
		return
	}
	for _, l := range lines {
		l.Amount = decimalString(l.sum)
	}
	doc["neighbors"], _ = json.Marshal(kept)
	if len(kept) == 0 {
		delete(doc, "neighbors")
	}
	doc["transfers"], _ = json.Marshal(lines)
	if more {
		doc["transfersHasMore"] = json.RawMessage("true")
	}
}

// balances adds to each account its balance, credit - debit (what the web app shows).
func balances(doc jsonObj) {
	var accounts []jsonObj
	if json.Unmarshal(doc["accounts"], &accounts) != nil || accounts == nil {
		return
	}
	for _, a := range accounts {
		credit, ok1 := new(big.Rat).SetString(string(a["credit"]))
		debit, ok2 := new(big.Rat).SetString(string(a["debit"]))
		if ok1 && ok2 {
			a["balance"] = json.RawMessage(decimalString(credit.Sub(credit, debit)))
		}
	}
	doc["accounts"], _ = json.Marshal(accounts)
}

// accountNames maps the card's account name ids and currency ids to their names.
func accountNames(doc jsonObj) (map[string]string, map[string]string) {
	var accounts []struct {
		NameID     string      `json:"nameId"`
		Name       string      `json:"name"`
		CurrencyID json.Number `json:"currencyId"`
		Currency   string      `json:"currency"`
	}
	names, currencies := map[string]string{}, map[string]string{}
	if json.Unmarshal(doc["accounts"], &accounts) == nil {
		for _, a := range accounts {
			names[a.NameID], currencies[a.CurrencyID.String()] = a.Name, a.Currency
		}
	}
	return names, currencies
}

// decimalString writes a sum without trailing zeros (exact: weights are decimal strings).
func decimalString(r *big.Rat) string {
	s := r.FloatString(10)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// graphMoney is the Around of graph_get: the answer as it is, plus the money its edges carry, each
// transfer named by id (one batch reads the Transfers actors' refs).
func graphMoney(ctx context.Context, c *publicapi.Client, method string, args, _ map[string]json.RawMessage) (json.RawMessage, error) {
	v, err := c.Pass(ctx, method, args)
	if err != nil {
		return nil, err
	}
	var page struct {
		Edges []struct {
			LinkedActorID string          `json:"linkedActorId"`
			SysLinkType   string          `json:"sysLinkType"`
			Weight        json.RawMessage `json:"weight"`
		} `json:"edges"`
	}
	if json.Unmarshal(v, &page) != nil {
		return v, nil
	}
	ids := make([]string, 0, len(page.Edges))
	for _, e := range page.Edges {
		if w := strings.Trim(string(e.Weight), `"`); e.SysLinkType == "" && w != "" && w != "null" {
			ids = append(ids, e.LinkedActorID)
		}
	}
	out, err := graphTransfers(v, transferIDs(ctx, c, ids))
	if err != nil {
		return nil, err
	}
	return namedEdgeTypes(ctx, c, out), nil
}

// graphTransfers adds to a graph_get answer the money its edges carry: an edge with a weight and a
// linked actor is a transfersAmount_* edge (source paid target; linkedActorId the transfer's
// Transfers actor, named by transfer id through transfers).
func graphTransfers(v json.RawMessage, transfers map[string]string) (json.RawMessage, error) {
	var doc jsonObj
	var page struct {
		Nodes []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"nodes"`
		Edges []struct {
			Source        string          `json:"source"`
			Target        string          `json:"target"`
			LinkedActorID string          `json:"linkedActorId"`
			SysLinkType   string          `json:"sysLinkType"`
			EdgeTypeID    int64           `json:"edgeTypeId"`
			Weight        json.RawMessage `json:"weight"`
		} `json:"edges"`
	}
	if json.Unmarshal(v, &doc) != nil || json.Unmarshal(v, &page) != nil {
		return v, nil
	}
	titles := map[string]string{}
	for _, n := range page.Nodes {
		titles[n.ID] = n.Title
	}
	type money struct {
		From            string `json:"from"`
		FromTitle       string `json:"fromTitle,omitempty"`
		To              string `json:"to"`
		ToTitle         string `json:"toTitle,omitempty"`
		Amount          string `json:"amount"`
		EdgeTypeID      int64  `json:"edgeTypeId"`
		TransferID      string `json:"transferId,omitempty"`
		TransferActorID string `json:"transferActorId,omitempty"`
	}
	var out []money
	for _, e := range page.Edges {
		w := strings.Trim(string(e.Weight), `"`)
		if e.LinkedActorID == "" || e.SysLinkType != "" || w == "" || w == "null" {
			continue
		}
		m := money{From: e.Source, FromTitle: titles[e.Source], To: e.Target, ToTitle: titles[e.Target], Amount: w, EdgeTypeID: e.EdgeTypeID, TransferID: transfers[e.LinkedActorID]}
		if m.TransferID == "" {
			m.TransferActorID = e.LinkedActorID
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return v, nil
	}
	doc["transfers"], _ = json.Marshal(out)
	return json.Marshal(doc)
}
