package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
)

// layer_digest summarises a layer for a person in one call (brief phase 5.3; eval layer-07 made 14
// calls and still missed the departments' balances): its counts, its actors by template and by
// hierarchy level, each actor's key fields and balances, the totals per account and the money moved
// between its actors. Reads, with no call per actor: the layer and its stats, every placement (paged),
// the edge types (ontology/get) and one batch of actor/context/get per 100 actors.

// LayerDigestInput names the layer; Full answers every actor, field and non-zero balance uncut.
type LayerDigestInput struct {
	LayerID string `json:"layerId"`
	Full    bool   `json:"full,omitempty"`
}

const (
	digestActors     = 80        // actors a compact digest lists (all are counted)
	digestFields     = 4         // filled fields per actor in a compact digest
	digestValueRunes = 80        // a compact field value is cut here
	digestMaxActors  = 500       // actors whose cards are read; past it the digest says so
	digestMoneyTypes = 19        // money edge types read with the hierarchy (actor/context/get takes 20)
	digestTransfers  = 40        // transfer lines a compact digest lists, the largest first
	digestMaxBytes   = 256 << 10 // a full digest drops its deepest actors past this
)

type LayerDigest struct {
	LayerID   string           `json:"layerId"`
	Title     string           `json:"title,omitempty"`
	Stats     json.RawMessage  `json:"stats,omitempty"`
	Forms     []DigestForm     `json:"forms"`
	Levels    []DigestLevel    `json:"levels"`
	Actors    []DigestActor    `json:"actors"`
	Totals    []DigestBalance  `json:"totals,omitempty"`
	Transfers []DigestTransfer `json:"transfers,omitempty"`
	Notes     []string         `json:"notes,omitempty"`
}

type DigestForm struct {
	FormID int64  `json:"formId"`
	Title  string `json:"title,omitempty"`
	Count  int    `json:"count"`
}

// DigestLevel counts the actors at one depth of the layer's hierarchy (0: no parent on the layer).
type DigestLevel struct {
	Level int `json:"level"`
	Count int `json:"count"`
}

type DigestActor struct {
	ID       string          `json:"id"`
	Title    string          `json:"title,omitempty"`
	Form     string          `json:"form,omitempty"`
	Level    int             `json:"level"`
	ParentID string          `json:"parentId,omitempty"`
	Data     []DigestField   `json:"data,omitempty"`
	Balances []DigestBalance `json:"balances,omitempty"`
}

type DigestField struct {
	Title string `json:"title"`
	Value any    `json:"value"`
}

// DigestBalance is credit − debit of one account (name × currency, fact); in totals, summed over
// the layer's actors that hold it.
type DigestBalance struct {
	Account  string `json:"account"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
	Actors   int    `json:"actors,omitempty"`
	sum      *big.Rat
}

// DigestTransfer is the money one layer actor moved to another (transfersAmount edges, summed).
type DigestTransfer struct {
	From      string `json:"from"`
	FromTitle string `json:"fromTitle,omitempty"`
	To        string `json:"to"`
	ToTitle   string `json:"toTitle,omitempty"`
	Amount    string `json:"amount"`
	Account   string `json:"account,omitempty"`
	Currency  string `json:"currency,omitempty"`
	Edges     int    `json:"edges"`
	sum       *big.Rat
}

// digestCard is the part of actor/context/get the digest reads.
type digestCard struct {
	Actor struct {
		Title     string `json:"title"`
		FormID    int64  `json:"formId"`
		FormTitle string `json:"formTitle"`
	} `json:"actor"`
	Data []struct {
		Title string `json:"title"`
		Value any    `json:"value"`
	} `json:"data"`
	Accounts []struct {
		NameID     string      `json:"nameId"`
		Name       string      `json:"name"`
		CurrencyID json.Number `json:"currencyId"`
		Currency   string      `json:"currency"`
		Type       string      `json:"type"`
		Debit      json.Number `json:"debit"`
		Credit     json.Number `json:"credit"`
	} `json:"accounts"`
	Neighbors []struct {
		EdgeTypeID int64  `json:"edgeTypeId"`
		EdgeType   string `json:"edgeType"`
		Role       string `json:"role"`
		Direction  string `json:"direction"`
		HasMore    bool   `json:"hasMore"`
		Neighbors  []struct {
			ActorID string `json:"actorId"`
			Weight  string `json:"weight"`
		} `json:"neighbors"`
	} `json:"neighbors"`
}

// LayerDigest reads and summarises the layer.
func (c *Client) LayerDigest(ctx context.Context, in LayerDigestInput) (LayerDigest, error) {
	if e := consoleOnly(c); e != nil {
		return LayerDigest{}, e
	}
	if !validID(in.LayerID) {
		return LayerDigest{}, errors.New("layerId required")
	}
	ws := c.workspace()
	layer, err := call[map[string]any, struct {
		Title string `json:"title"`
		Data  struct {
			Accounts []struct {
				Value string `json:"value"`
			} `json:"accounts"`
		} `json:"data"`
	}](ctx, c, "actor/get", map[string]any{"accId": ws, "actorId": in.LayerID, "fields": []string{"id", "title", "data"}})
	if err != nil {
		return LayerDigest{}, fmt.Errorf("layer: %w", err)
	}
	out := LayerDigest{LayerID: in.LayerID, Title: layer.Title, Forms: []DigestForm{}, Levels: []DigestLevel{}, Actors: []DigestActor{}}
	if stats, err := call[map[string]any, json.RawMessage](ctx, c, "graph/layer/stats/get", map[string]any{"accId": ws, "actorId": in.LayerID}); err == nil {
		out.Stats = stats
	}
	placements, err := c.AllNodePlacements(ctx, in.LayerID)
	if err != nil {
		return LayerDigest{}, fmt.Errorf("placements: %w", err)
	}
	var ids []string
	seen := map[string]bool{}
	for _, p := range placements {
		if !seen[p.ActorID] {
			seen[p.ActorID] = true
			ids = append(ids, p.ActorID)
		}
	}
	if len(ids) > digestMaxActors {
		out.Notes = append(out.Notes, fmt.Sprintf("the layer has %d actors: the first %d by placement are summarised", len(ids), digestMaxActors))
		ids = ids[:digestMaxActors]
	}
	shown := map[string]bool{}
	for _, a := range layer.Data.Accounts {
		shown[a.Value] = true
	}
	types, note := c.digestEdgeTypes(ctx, shown)
	if note != "" {
		out.Notes = append(out.Notes, note)
	}
	cards, err := c.digestCards(ctx, ids, types)
	if err != nil {
		return LayerDigest{}, err
	}
	summarise(&out, ids, cards, shown, in.Full)
	fitDigest(&out)
	return out, nil
}

// fitDigest keeps a full digest within digestMaxBytes: the deepest actors (listed last) go first.
func fitDigest(out *LayerDigest) {
	b, err := json.Marshal(out)
	if err != nil || len(b) <= digestMaxBytes {
		return
	}
	size, n := len(b), len(out.Actors)
	for n > 0 && size > digestMaxBytes {
		n--
		a, _ := json.Marshal(out.Actors[n])
		size -= len(a) + 1
	}
	out.Notes = append(out.Notes, fmt.Sprintf("%d actors listed of %d: the digest keeps within %d KiB (the top levels first)", n, len(out.Actors), digestMaxBytes>>10))
	out.Actors = out.Actors[:n]
}

// digestEdgeTypes is the hierarchy type and the money types to read neighbors along: the pairs the
// layer shows first.
func (c *Client) digestEdgeTypes(ctx context.Context, shown map[string]bool) ([]int64, string) {
	digest, err := call[map[string]any, struct {
		EdgeTypes []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"edgeTypes"`
	}](ctx, c, "ontology/get", map[string]any{"accId": c.workspace(), "fields": []string{"edgeTypes"}, "with": []string{"accountEdgeTypes"}, "limit": 1})
	if err != nil {
		return nil, "edge types not read (" + err.Error() + "): no hierarchy levels or transfers"
	}
	var hierarchy []int64
	var first, rest []int64
	for _, t := range digest.EdgeTypes {
		switch {
		case t.Role == "hierarchy":
			hierarchy = append(hierarchy, t.ID)
		case t.Role == "transactions" && strings.HasPrefix(t.Name, transfersPrefix):
			if shown[strings.TrimPrefix(t.Name, transfersPrefix)] {
				first = append(first, t.ID)
			} else {
				rest = append(rest, t.ID)
			}
		}
	}
	money := append(first, rest...)
	note := ""
	if len(money) > digestMoneyTypes {
		note = fmt.Sprintf("transfers are read on %d of the workspace's %d money edge types (the layer's own accounts first)", digestMoneyTypes, len(money))
		money = money[:digestMoneyTypes]
	}
	return append(hierarchy[:min(len(hierarchy), 1)], money...), note
}

const transfersPrefix = "transfersAmount_"

// digestCards reads the actors' cards in batches of 100: data, accounts and the neighbors along the
// given types. An actor whose card is refused is left out of the details.
func (c *Client) digestCards(ctx context.Context, ids []string, types []int64) (map[string]digestCard, error) {
	params := make([]any, len(ids))
	for i, id := range ids {
		p := map[string]any{"accId": c.workspace(), "actorId": id, "fields": []string{"actor", "data", "accounts"}, "perType": 50}
		if len(types) > 0 {
			p["fields"], p["edgeTypeIds"] = []string{"actor", "data", "accounts", "neighbors"}, types
		}
		params[i] = p
	}
	found, err := c.eachItems(ctx, "actor/context/get", params)
	if err != nil {
		return nil, fmt.Errorf("actor cards: %w", err)
	}
	out := map[string]digestCard{}
	for i, f := range found {
		var card digestCard
		if f.err == nil && json.Unmarshal(f.raw, &card) == nil {
			out[ids[i]] = card
		}
	}
	return out, nil
}

func ratOf(n json.Number) *big.Rat {
	r, ok := new(big.Rat).SetString(n.String())
	if !ok {
		return new(big.Rat)
	}
	return r
}

func ratString(r *big.Rat) string {
	s := r.FloatString(10)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// summarise fills the digest from the cards: levels from the hierarchy parents on the layer,
// balances (the layer's own pairs when it names some, else every non-zero one), totals and transfers.
func summarise(out *LayerDigest, ids []string, cards map[string]digestCard, shown map[string]bool, full bool) {
	onLayer := map[string]bool{}
	for _, id := range ids {
		onLayer[id] = true
	}
	parent := map[string]string{}
	for _, id := range ids {
		for _, g := range cards[id].Neighbors {
			if g.Role == "hierarchy" && g.Direction == "in" {
				for _, n := range g.Neighbors {
					if onLayer[n.ActorID] && parent[id] == "" && n.ActorID != id {
						parent[id] = n.ActorID
					}
				}
			}
		}
	}
	level := func(id string) int {
		depth := 0
		for p, seen := parent[id], map[string]bool{id: true}; p != "" && !seen[p]; p = parent[p] {
			seen[p], depth = true, depth+1
		}
		return depth
	}
	forms, levels := map[int64]*DigestForm{}, map[int]int{}
	partial := 0 // actors whose money edges were more than read
	totals := map[string]*DigestBalance{}
	moves := map[string]*DigestTransfer{}
	names := map[string][2]string{} // nameId_currencyId -> account, currency
	for _, id := range ids {
		card, ok := cards[id]
		if !ok {
			out.Actors = append(out.Actors, DigestActor{ID: id, Level: level(id)})
			continue
		}
		f := forms[card.Actor.FormID]
		if f == nil {
			f = &DigestForm{FormID: card.Actor.FormID, Title: card.Actor.FormTitle}
			forms[card.Actor.FormID] = f
		}
		f.Count++
		a := DigestActor{ID: id, Title: card.Actor.Title, Form: card.Actor.FormTitle, Level: level(id), ParentID: parent[id]}
		levels[a.Level]++
		for _, d := range card.Data {
			if d.Value == nil || d.Value == "" || !full && len(a.Data) == digestFields {
				continue
			}
			v := d.Value
			if s, ok := v.(string); ok && !full {
				if r := []rune(s); len(r) > digestValueRunes {
					v = string(r[:digestValueRunes]) + "…"
				}
			}
			a.Data = append(a.Data, DigestField{Title: d.Title, Value: v})
		}
		for _, acc := range card.Accounts {
			pair := acc.NameID + "_" + acc.CurrencyID.String()
			names[pair] = [2]string{acc.Name, acc.Currency}
			bal := new(big.Rat).Sub(ratOf(acc.Credit), ratOf(acc.Debit))
			if acc.Type != "" && acc.Type != "fact" || len(shown) > 0 && !shown[pair] && !full || bal.Sign() == 0 {
				continue
			}
			a.Balances = append(a.Balances, DigestBalance{Account: acc.Name, Currency: acc.Currency, Balance: ratString(bal)})
			t := totals[pair]
			if t == nil {
				t = &DigestBalance{Account: acc.Name, Currency: acc.Currency, sum: new(big.Rat)}
				totals[pair] = t
			}
			t.sum.Add(t.sum, bal)
			t.Actors++
		}
		more := false
		for _, g := range card.Neighbors {
			if g.Role != "transactions" || g.Direction != "out" {
				continue
			}
			for _, n := range g.Neighbors {
				w, ok := new(big.Rat).SetString(n.Weight)
				if !onLayer[n.ActorID] || !ok {
					continue
				}
				key := id + "|" + n.ActorID + "|" + g.EdgeType
				m := moves[key]
				if m == nil {
					pair := strings.TrimPrefix(g.EdgeType, transfersPrefix)
					m = &DigestTransfer{From: id, To: n.ActorID, Account: pair, sum: new(big.Rat)}
					moves[key] = m
				}
				m.sum.Add(m.sum, w)
				m.Edges++
			}
			more = more || g.HasMore
		}
		if more {
			partial++
		}
		out.Actors = append(out.Actors, a)
	}
	titles := map[string]string{}
	for _, a := range out.Actors {
		titles[a.ID] = a.Title
	}
	for _, f := range forms {
		out.Forms = append(out.Forms, *f)
	}
	slices.SortFunc(out.Forms, func(a, b DigestForm) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Title, b.Title)
	})
	for l, n := range levels { // a hierarchy cycle on the layer leaves no level 0
		out.Levels = append(out.Levels, DigestLevel{Level: l, Count: n})
	}
	slices.SortFunc(out.Levels, func(a, b DigestLevel) int { return a.Level - b.Level })
	for _, t := range totals {
		t.Balance = ratString(t.sum)
		out.Totals = append(out.Totals, *t)
	}
	slices.SortFunc(out.Totals, func(a, b DigestBalance) int { return strings.Compare(a.Account+a.Currency, b.Account+b.Currency) })
	for _, m := range moves {
		m.Amount, m.FromTitle, m.ToTitle = ratString(m.sum), titles[m.From], titles[m.To]
		if n, ok := names[m.Account]; ok {
			m.Account, m.Currency = n[0], n[1]
		}
		out.Transfers = append(out.Transfers, *m)
	}
	slices.SortFunc(out.Transfers, func(a, b DigestTransfer) int {
		if c := b.sum.Cmp(a.sum); c != 0 {
			return c
		}
		return strings.Compare(a.From+a.To+a.Account, b.From+b.To+b.Account)
	})
	if partial > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d actors paid more than read (50 edges per money type): their transfers here are partial", partial))
	}
	if !full && len(out.Transfers) > digestTransfers {
		out.Notes = append(out.Notes, fmt.Sprintf("%d transfer lines listed of %d (the largest first); full: true lists all", digestTransfers, len(out.Transfers)))
		out.Transfers = out.Transfers[:digestTransfers]
	}
	slices.SortStableFunc(out.Actors, func(a, b DigestActor) int { return a.Level - b.Level })
	if !full && len(out.Actors) > digestActors {
		out.Notes = append(out.Notes, fmt.Sprintf("%d actors listed of %d (the top levels first); full: true lists all", digestActors, len(out.Actors)))
		out.Actors = out.Actors[:digestActors]
	}
	if len(shown) > 0 && !full {
		out.Notes = append(out.Notes, "balances are the accounts the layer shows (its settings); full: true adds every other non-zero one")
	}
}
