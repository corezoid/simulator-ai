package publicapi

import (
	"context"
	"strings"
)

// history/list matches its field against the key a history row stores: a template field as
// data.<id> (data.amount), a column by its own name (title, description, links, account, ...);
// since sim-api a5d500b6 a bare field id matches data.<id> too, but a field's title and a column in
// another case still do not. A model asking for the field it sees on the card — its title («Сума») —
// got an empty list (eval asof-07). HistoryField maps it to the stored key from the actor's template.

// historyColumns are the actor columns history rows name by themselves, by their lower case: a
// template field titled "Status" or "Title" must not take the column's place, and "Title" is sent as
// the stored "title" (history/list matches exactly).
var historyColumns = func() map[string]string {
	out := map[string]string{}
	for _, k := range []string{"title", "description", "status", "ownerId", "access", "links", "account", "color", "picture",
		"ref", "startDate", "endDate", "appId", "cardActorId", "systemObjId"} {
		out[strings.ToLower(k)] = k
	}
	return out
}()

// HistoryField answers the stored key for the field an actor's history is narrowed to: a template
// field named by its id or title (any case) becomes data.<id>; anything else (a column, data.<key>,
// another object type) is sent as given.
func (c *Client) HistoryField(ctx context.Context, objType, objID, field string) (string, error) {
	field = strings.TrimSpace(field)
	if objType != "actor" || field == "" || strings.HasPrefix(field, "data.") {
		return field, nil
	}
	if column, ok := historyColumns[strings.ToLower(field)]; ok {
		return column, nil
	}
	actor, err := call[map[string]any, struct {
		FormID int64 `json:"formId"`
	}](ctx, c, "actor/get", map[string]any{"accId": c.workspace(), "actorId": objID, "fields": []string{"id", "formId"}})
	if err != nil || actor.FormID < 1 {
		return field, err
	}
	t, err := c.readTemplate(ctx, actor.FormID)
	if err != nil {
		return field, err
	}
	if _, ok := t.Fields[field]; ok {
		return "data." + field, nil
	}
	for _, id := range t.Order {
		if strings.EqualFold(strings.TrimSpace(t.Fields[id].Title), field) {
			return "data." + id, nil
		}
	}
	return field, nil
}
