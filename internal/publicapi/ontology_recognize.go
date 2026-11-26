package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// itemResult is one item of a non-atomic batch: its result or its error.
type itemResult struct {
	raw json.RawMessage
	err *APIError
}

// eachItems runs one method per item in non-atomic public batches of up to 100 (reads: one item's
// refusal leaves the others), in order.
func (c *Client) eachItems(ctx context.Context, method string, params []any) ([]itemResult, error) {
	out := make([]itemResult, 0, len(params))
	for start := 0; start < len(params); start += 100 {
		chunk := params[start:min(start+100, len(params))]
		req := atomicRequest{List: make([]atomicItem, len(chunk))}
		for i, p := range chunk {
			req.List[i] = atomicItem{ID: strconv.Itoa(i), Method: method, Params: p}
		}
		wire, err := call[atomicRequest, atomicWire](ctx, c, "batch", req)
		if err != nil {
			return nil, err
		}
		if len(wire.List) != len(chunk) {
			return nil, errors.New("batch returned a different number of items")
		}
		for _, item := range wire.List {
			r := itemResult{raw: item.Result}
			if item.Error != nil {
				r.raw, r.err = nil, c.apiError(item.Error)
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// recognize looks for each new actor among what exists, before it is created: the actor an earlier
// call of this operationId created (its ref), then actors of its template with the same title or
// with one of its names among their aliases. The first is the patch's own (exists); the others are
// candidates the person picks from (existing) or overrides (new).
func (p *ontologyPlan) recognize(ctx context.Context, c *Client) error {
	var fresh []*planActor
	for _, a := range p.actors {
		if a.in.Existing == "" && a.formID > 0 {
			fresh = append(fresh, a)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	refs, titles, aliases := make([]any, len(fresh)), make([]any, len(fresh)), make([]any, len(fresh))
	fields := []string{"id", "title", "formId"}
	for i, a := range fresh {
		refs[i] = map[string]any{"accId": c.workspace(), "formId": a.formID, "ref": BuildRef(p.in.OperationID, a.in.Key), "fields": fields}
		titles[i] = map[string]any{"accId": c.workspace(), "formId": a.formID, "query": a.title, "searchType": "text", "limit": 20, "fields": fields}
		var ors []map[string]any
		for _, name := range append([]string{a.title}, a.in.Aliases...) {
			ors = append(ors, map[string]any{aliasesKey: map[string]any{"$all": []string{strings.TrimSpace(name)}}})
		}
		aliases[i] = map[string]any{"accId": c.workspace(), "formId": a.formID, "queryData": map[string]any{"$or": ors}, "limit": 10, "fields": append(fields, "data."+aliasesKey)}
	}
	byRef, err := c.eachItems(ctx, "actor/by-ref/get", refs)
	if err != nil {
		return fmt.Errorf("recognition: %w", err)
	}
	byTitle, err := c.eachItems(ctx, "actor/search/list", titles)
	if err != nil {
		return fmt.Errorf("recognition: %w", err)
	}
	byAlias, err := c.eachItems(ctx, "actor/filter/list", aliases)
	if err != nil {
		return fmt.Errorf("recognition: %w", err)
	}
	for i, a := range fresh {
		var own actorBrief
		if byRef[i].err == nil && json.Unmarshal(byRef[i].raw, &own) == nil && own.ID != "" {
			a.id, a.status = own.ID, "exists"
			continue
		}
		if e := byRef[i].err; e != nil && e.Reason != "notFound" && e.Reason != "noFoundOrNoAccess" {
			return fmt.Errorf("recognition of %q: %w", a.in.Key, e)
		}
		a.status = "create"
		// A search that failed found no candidates either way: say so rather than let the apply
		// create what may be a duplicate unannounced.
		for _, r := range []struct {
			what string
			err  *APIError
		}{{"title", byTitle[i].err}, {"alias", byAlias[i].err}} {
			if r.err != nil {
				p.warnings = append(p.warnings, fmt.Sprintf("actor %q: the search for same-%s actors failed (%v): check it does not exist already (actors_search)", a.in.Key, r.what, r.err))
			}
		}
		seen := map[string]bool{}
		add := func(b actorBrief, reason string) {
			if b.ID != "" && !seen[b.ID] && len(a.candidates) < 5 {
				seen[b.ID] = true
				a.candidates = append(a.candidates, OntologyCandidate{ActorID: b.ID, Title: b.Title, FormID: b.FormID, Reason: reason})
			}
		}
		for _, b := range listOf(byTitle[i]) {
			if sameName(b.Title, a.title) {
				add(b.actorBrief, "same title")
			}
		}
		for _, b := range listOf(byAlias[i]) {
			add(b.actorBrief, "alias: "+matchedAlias(b.Data.Aliases, a))
		}
	}
	return nil
}

type aliasRow struct {
	actorBrief
	Data struct {
		Aliases []string `json:"aliases"`
	} `json:"data"`
}

func listOf(r itemResult) []aliasRow {
	var page struct {
		List []aliasRow `json:"list"`
	}
	if r.err != nil || json.Unmarshal(r.raw, &page) != nil {
		return nil
	}
	return page.List
}

func sameName(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

// matchedAlias names which of the actor's names a candidate holds among its aliases.
func matchedAlias(held []string, a *planActor) string {
	for _, name := range append([]string{a.title}, a.in.Aliases...) {
		for _, h := range held {
			if h == strings.TrimSpace(name) {
				return h
			}
		}
	}
	return a.title
}
