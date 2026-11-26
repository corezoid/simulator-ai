package mcpserver

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// "Which payment went through contract №22, to whom and from whom" (eval transfers-14): a contract
// is seldom a party to its payment; the payment names it in its comment ("Оплата за договором №22").
// The card's payments are the transfers of the last 366 days the actor is a party to, and those whose
// comment holds a number token of its title (№22): one batch of transfer/filter. sim-api's full-text
// comment search matches the token, not the inflected title.

const (
	paymentsSection = "payments"
	paymentsLimit   = 10  // transfers per read
	paymentTokens   = 2   // number tokens of the title searched in comments
	paymentComment  = 300 // a payment's comment is cut here (runes)
)

// mentionsToken says whether a comment holds the token as a whole word: sim-api's full-text query
// also matches parts of it (№2 finds "№21" and "етап 2"), so the comment is checked again here.
func mentionsToken(comment, token string) bool {
	alnum := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	low, tok := []rune(strings.ToLower(comment)), []rune(strings.ToLower(token))
	for i := 0; i+len(tok) <= len(low); i++ {
		if string(low[i:i+len(tok)]) != string(tok) {
			continue
		}
		before := i == 0 || !alnum(low[i-1])
		after := i+len(tok) == len(low) || !alnum(low[i+len(tok)])
		if before && after {
			return true
		}
	}
	return false
}

type paymentLeg struct {
	ActorID string `json:"actorId,omitempty"`
	Title   string `json:"title,omitempty"`
	Amount  string `json:"amount,omitempty"`
}

type cardPayment struct {
	ID        string       `json:"id"`
	CreatedAt int64        `json:"createdAt"`
	Type      string       `json:"type,omitempty"`
	Comment   string       `json:"comment,omitempty"`
	Account   string       `json:"account,omitempty"`
	Currency  string       `json:"currency,omitempty"`
	From      []paymentLeg `json:"from"`
	To        []paymentLeg `json:"to"`
	Via       string       `json:"via"` // party: the actor is a leg; comment: its number is in the comment
}

// wantsPayments reads and removes the plugin's own payments section from the card's fields: no
// fields reads every section, payments included.
func wantsPayments(args map[string]json.RawMessage) bool {
	var fields []string
	if len(args["fields"]) == 0 || json.Unmarshal(args["fields"], &fields) != nil || len(fields) == 0 {
		return true
	}
	i := slices.Index(fields, paymentsSection)
	if i < 0 {
		return false
	}
	fields = slices.Delete(fields, i, i+1)
	if len(fields) == 0 {
		fields = []string{"actor"}
	}
	args["fields"], _ = json.Marshal(fields)
	return true
}

// numberTokens are the words of a title that hold a digit (№22, 22/2026, INV-7).
func numberTokens(title string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(title, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(",;:()«»\"'", r) }) {
		if strings.IndexFunc(w, unicode.IsDigit) >= 0 && !slices.Contains(out, w) && len(out) < paymentTokens {
			out = append(out, w)
		}
	}
	return out
}

// withPayments adds the card's payments; a failed read leaves the card as it is.
func withPayments(ctx context.Context, c *publicapi.Client, actorID string, card json.RawMessage) json.RawMessage {
	var doc jsonObj
	var head struct {
		Actor struct {
			Title string `json:"title"`
		} `json:"actor"`
	}
	if json.Unmarshal(card, &doc) != nil || doc == nil || json.Unmarshal(card, &head) != nil {
		return card
	}
	now := time.Now().UnixMilli()
	window := map[string]json.RawMessage{"from": jsonInt(now - maxWindowMs), "to": jsonInt(now), "limit": jsonInt(paymentsLimit)}
	item := func(k string, v any) map[string]json.RawMessage {
		out := map[string]json.RawMessage{}
		for wk, wv := range window {
			out[wk] = wv
		}
		out[k], _ = json.Marshal(v)
		return out
	}
	items := []map[string]json.RawMessage{item("actorIds", []string{actorID})}
	vias, tokens := []string{"party"}, []string{""}
	for _, token := range numberTokens(head.Actor.Title) {
		items, vias, tokens = append(items, item("query", token)), append(vias, "comment"), append(tokens, token)
	}
	results, err := c.PassEach(ctx, "transfer/filter", items)
	if err != nil {
		return card
	}
	var list []cardPayment
	seen := map[string]bool{}
	hidden := 0
	for _, r := range results {
		var page struct {
			List []struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				Comment   string `json:"comment"`
				CreatedAt int64  `json:"createdAt"`
				From      []struct {
					ActorID      string          `json:"actorId"`
					ActorTitle   string          `json:"actorTitle"`
					Amount       json.RawMessage `json:"amount"`
					AccountName  string          `json:"accountName"`
					CurrencyName string          `json:"currencyName"`
				} `json:"from"`
				To []struct {
					ActorID    string          `json:"actorId"`
					ActorTitle string          `json:"actorTitle"`
					Amount     json.RawMessage `json:"amount"`
				} `json:"to"`
			} `json:"list"`
		}
		if r.Error != "" || r.Index < 0 || r.Index >= len(vias) || json.Unmarshal(r.Result, &page) != nil {
			continue
		}
		for _, t := range page.List {
			if seen[t.ID] || tokens[r.Index] != "" && !mentionsToken(t.Comment, tokens[r.Index]) {
				continue
			}
			seen[t.ID] = true
			comment := t.Comment
			if rs := []rune(comment); len(rs) > paymentComment {
				comment = string(rs[:paymentComment]) + "…"
			}
			p := cardPayment{ID: t.ID, CreatedAt: t.CreatedAt, Type: t.Type, Comment: comment, Via: vias[r.Index], From: []paymentLeg{}, To: []paymentLeg{}}
			for _, l := range t.From {
				p.From = append(p.From, paymentLeg{l.ActorID, l.ActorTitle, strings.Trim(string(l.Amount), `"`)})
				p.Account, p.Currency = l.AccountName, l.CurrencyName
			}
			visible := false
			for _, l := range t.To {
				p.To = append(p.To, paymentLeg{l.ActorID, l.ActorTitle, strings.Trim(string(l.Amount), `"`)})
			}
			for _, l := range append(slices.Clone(p.From), p.To...) {
				visible = visible || l.Title != ""
			}
			if !visible { // between actors you cannot see (or deleted ones): counted, not listed
				hidden++
				continue
			}
			list = append(list, p)
		}
	}
	if hidden > 0 {
		doc["paymentsHidden"], _ = json.Marshal(hidden)
	}
	if len(list) == 0 && hidden == 0 {
		return card
	}
	slices.SortFunc(list, func(a, b cardPayment) int { return int(b.CreatedAt - a.CreatedAt) })
	doc[paymentsSection], _ = json.Marshal(list)
	out, err := json.Marshal(doc)
	if err != nil {
		return card
	}
	return out
}
