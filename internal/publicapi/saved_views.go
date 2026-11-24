package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// A saved view is an actor of the ActorFilters system template (sim-api docs/actor-filters.md): its
// data.filter, a JSON string, holds the template (formId), the predicate as the filter editor writes
// it (q: "field=value&other>=10||…"), statuses and the order. actor/filter/list with filterActorId
// runs it on the server, and the web app's actor list and dashboards open the same actor.

// ViewCondition is one term of a view: a template field, an operator and a value.
type ViewCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// SavedViewCreateInput makes a saved view: actors of a template, Where (OR of AND groups) on its
// fields, Status, and the order.
type SavedViewCreateInput struct {
	Title         string            `json:"title"`
	FormID        int64             `json:"formId"`
	Where         [][]ViewCondition `json:"where,omitempty"`
	Status        []string          `json:"status,omitempty"`
	OrderBy       string            `json:"orderBy,omitempty"`
	OrderValue    string            `json:"orderValue,omitempty"`
	OrderByKey    string            `json:"orderByKey,omitempty"`
	AccountNameID string            `json:"accountNameId,omitempty"`
	CurrencyID    int64             `json:"currencyId,omitempty"`
}

// SavedView is a saved view as found or made.
type SavedView struct {
	ViewID string `json:"viewId"`
	Title  string `json:"title"`
	FormID int64  `json:"formId,omitempty"`
	Q      string `json:"q,omitempty"`
}

var (
	viewOps      = []string{"=", "!=", "<", "<=", ">", ">="}
	viewStatuses = []string{"verified", "rejected", "pending", "null"}
	viewOrders   = []string{"createdAt", "updatedAt", "title", "owner", "balance"}
)

// viewQuery writes the conditions in the editor's q grammar; a value cannot hold & or | (they split
// the string before it is decoded, as in pong-server).
func viewQuery(t template, where [][]ViewCondition) (string, error) {
	groups := make([]string, 0, len(where))
	for _, group := range where {
		if len(group) == 0 {
			return "", errors.New("where: an empty group")
		}
		terms := make([]string, 0, len(group))
		for _, cond := range group {
			f, ok := t.Fields[cond.Field]
			switch {
			case !ok:
				return "", fmt.Errorf("where: field %q is not in template %q; %s", cond.Field, t.Title, t.describe())
			case !slices.Contains(viewOps, cond.Op):
				return "", fmt.Errorf("where %s: op is one of %s", cond.Field, strings.Join(viewOps, " "))
			case strings.ContainsAny(cond.Value, "&|") || strings.ContainsAny(asServerReads(cond.Value), "&|"):
				// The server decodes q twice before it splits it (pong-server's decodeURIComponent).
				return "", fmt.Errorf("where %s: a value cannot hold & or | (nor %%26, %%7C)", cond.Field)
			case (cond.Op == "<" || cond.Op == ">") && strings.HasPrefix(cond.Value, "="):
				// The first operator wins: "<" then "=5" reads as "<=" 5.
				return "", fmt.Errorf("where %s: a value after %s cannot start with =; use %s=", cond.Field, cond.Op, cond.Op)
			}
			value := cond.Value
			if len(f.Options) > 0 && !f.dynamic() && value != "" {
				for _, o := range f.Options {
					if strings.EqualFold(o.Title, value) {
						value = o.Value
					}
				}
			}
			terms = append(terms, url.PathEscape(cond.Field)+cond.Op+url.PathEscape(value))
		}
		groups = append(groups, strings.Join(terms, "&"))
	}
	return strings.Join(groups, "||"), nil
}

// asServerReads is a value as the server splits it: escaped once here (url.PathEscape) and
// decoded twice there, so one escape of its own survives as the character it names.
func asServerReads(value string) string {
	if v, err := url.PathUnescape(value); err == nil {
		return v
	}
	return value
}

// CreateSavedView stores a saved view after checking its fields against the template.
func (c *Client) CreateSavedView(ctx context.Context, in SavedViewCreateInput) (SavedView, error) {
	if e := consoleOnly(c); e != nil {
		return SavedView{}, e
	}
	switch {
	case !validTitle(in.Title, false) || in.FormID < 1:
		return SavedView{}, errors.New("a 1..1000 character title and the formId of the template the view lists required")
	case len(in.Where) > 20:
		return SavedView{}, errors.New("at most 20 where groups")
	case in.OrderBy != "" && !slices.Contains(viewOrders, in.OrderBy):
		return SavedView{}, fmt.Errorf("orderBy is one of %s", strings.Join(viewOrders, ", "))
	case in.OrderValue != "" && in.OrderValue != "ASC" && in.OrderValue != "DESC":
		return SavedView{}, errors.New("orderValue is ASC or DESC")
	case in.OrderBy == "balance" && (in.AccountNameID == "" || in.CurrencyID < 1):
		return SavedView{}, errors.New("orderBy balance needs accountNameId and currencyId")
	}
	for _, s := range in.Status {
		if !slices.Contains(viewStatuses, s) {
			return SavedView{}, fmt.Errorf("status is one of %s", strings.Join(viewStatuses, ", "))
		}
	}
	t, err := c.readTemplate(ctx, in.FormID)
	if err != nil {
		return SavedView{}, fmt.Errorf("template %d: %w", in.FormID, err)
	}
	if in.OrderByKey != "" {
		if _, ok := t.Fields[in.OrderByKey]; !ok {
			return SavedView{}, fmt.Errorf("orderByKey %q is not a field of template %q; %s", in.OrderByKey, t.Title, t.describe())
		}
	}
	q, err := viewQuery(t, in.Where)
	if err != nil {
		return SavedView{}, err
	}
	filter := map[string]any{"formId": in.FormID, "selectedForms": []int64{in.FormID}, "q": q}
	for k, v := range map[string]string{"orderBy": in.OrderBy, "orderValue": in.OrderValue, "orderByKey": in.OrderByKey, "accountNameId": in.AccountNameID} {
		if v != "" {
			filter[k] = v
		}
	}
	if in.CurrencyID > 0 {
		filter["currencyId"] = in.CurrencyID
	}
	if len(in.Status) > 0 {
		filter["status"] = in.Status
	}
	encoded, _ := plainJSON(filter)
	data, _ := plainJSON(map[string]any{"filter": string(encoded), "fields": "[]", "defaultFields": `["title","id","ref","owner","createdAt","updatedAt","balance"]`,
		"formFieldExportFormat": "idAndTitle", "cacheFilterResult": false})
	formID, err := c.systemFormID(ctx, "ActorFilters")
	if err != nil {
		return SavedView{}, err
	}
	a, err := c.CreateActor(ctx, ActorCreateInput{FormID: formID, Title: in.Title, Data: data})
	if err != nil {
		return SavedView{}, fmt.Errorf("create the ActorFilters actor: %w", err)
	}
	return SavedView{ViewID: deref(a.ID), Title: in.Title, FormID: in.FormID, Q: q}, nil
}

// SavedViewsFindInput finds saved views by their title, optionally of one template.
type SavedViewsFindInput struct {
	Query  string `json:"query,omitempty"`
	FormID int64  `json:"formId,omitempty"`
	Limit  int64  `json:"limit,omitempty"`
	Offset int64  `json:"offset,omitempty"`
}

// SavedViews is a page of saved views.
type SavedViews struct {
	List    []SavedView `json:"list"`
	HasMore bool        `json:"hasMore"`
}

// FindSavedViews lists the saved views you see: ActorFilters actors, newest first.
func (c *Client) FindSavedViews(ctx context.Context, in SavedViewsFindInput) (SavedViews, error) {
	if e := consoleOnly(c); e != nil {
		return SavedViews{}, e
	}
	limit, err := pageLimit(in.Limit)
	if err != nil || in.Offset < 0 {
		return SavedViews{}, errors.New("limit is 1..100 and offset 0 or more")
	}
	formID, err := c.systemFormID(ctx, "ActorFilters")
	if err != nil {
		return SavedViews{}, err
	}
	params := map[string]any{"accId": c.workspace(), "formId": formID, "limit": peekLimit(limit), "offset": in.Offset, "fields": []string{"id", "title", "data"}}
	if strings.TrimSpace(in.Query) != "" {
		params["search"] = in.Query
	}
	page, err := call[map[string]any, struct {
		List []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Data  struct {
				Filter json.RawMessage `json:"filter"`
			} `json:"data"`
		} `json:"list"`
	}](ctx, c, "actor/filter/list", params)
	if err != nil {
		return SavedViews{}, err
	}
	out := SavedViews{List: []SavedView{}, HasMore: morePages(len(page.List), limit)}
	for i, a := range page.List {
		if int64(i) == limit {
			break
		}
		var f struct {
			FormID flexInt `json:"formId"`
			Q      string  `json:"q"`
		}
		_ = json.Unmarshal(unwrapString(a.Data.Filter), &f)
		if in.FormID != 0 && int64(f.FormID) != in.FormID {
			continue
		}
		out.List = append(out.List, SavedView{ViewID: a.ID, Title: a.Title, FormID: int64(f.FormID), Q: f.Q})
	}
	return out, nil
}

// SavedViewRunInput runs a saved view: a page of the actors it selects, and their count with count.
type SavedViewRunInput struct {
	ViewID string   `json:"viewId"`
	Fields []string `json:"fields,omitempty"`
	Limit  int64    `json:"limit,omitempty"`
	Offset int64    `json:"offset,omitempty"`
	Count  bool     `json:"count,omitempty"`
}

// SavedViewRows is a page of a view's actors, as actor/filter/list answers them.
type SavedViewRows struct {
	List    json.RawMessage `json:"list"`
	HasMore bool            `json:"hasMore"`
	Total   *int64          `json:"total,omitempty"`
}

// RunSavedView reads a page of what a saved view selects; the server applies the stored filter.
func (c *Client) RunSavedView(ctx context.Context, in SavedViewRunInput) (SavedViewRows, error) {
	if e := consoleOnly(c); e != nil {
		return SavedViewRows{}, e
	}
	limit, err := pageLimit(in.Limit)
	if err != nil || !validID(in.ViewID) || in.Offset < 0 || len(in.Fields) > 50 {
		return SavedViewRows{}, errors.New("viewId (the saved view's actor id), limit 1..100, offset 0 or more and at most 50 fields required")
	}
	fields := in.Fields
	if len(fields) == 0 {
		fields = []string{"id", "title", "formId"}
	}
	page, err := call[map[string]any, struct {
		List []json.RawMessage `json:"list"`
	}](ctx, c, "actor/filter/list", map[string]any{"accId": c.workspace(), "filterActorId": in.ViewID, "limit": peekLimit(limit), "offset": in.Offset, "fields": fields})
	if err != nil {
		return SavedViewRows{}, err
	}
	out := SavedViewRows{HasMore: morePages(len(page.List), limit)}
	if int64(len(page.List)) > limit {
		page.List = page.List[:limit]
	}
	if page.List == nil {
		page.List = []json.RawMessage{}
	}
	out.List, _ = json.Marshal(page.List)
	if in.Count {
		n, err := call[map[string]any, struct {
			Count int64 `json:"count"`
		}](ctx, c, "actor/filter/count", map[string]any{"accId": c.workspace(), "filterActorId": in.ViewID})
		if err != nil {
			return SavedViewRows{}, fmt.Errorf("count: %w", err)
		}
		out.Total = &n.Count
	}
	return out, nil
}

// peekLimit asks one row more than the page to learn whether another follows, within the server's
// maximum of 100; a full page of 100 then says only that there may be more.
func peekLimit(limit int64) int64 { return min(limit+1, 100) }

func morePages(got int, limit int64) bool {
	return int64(got) > limit || limit == 100 && got == 100
}

// plainJSON encodes as the web app's JSON.stringify does: & < > stay as they are, so the stored q
// reads the same in the filter editor.
func plainJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
