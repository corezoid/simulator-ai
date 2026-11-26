package publicapi

import (
	"context"
	"encoding/json"
	"slices"
)

// AI Console dialogs are Events actors whose meta_info names the layer or Smart Form they were
// opened on (aiDialogParentId); the console's own messages are comments under them. They mention
// the actors a person talked about, so a search or a mention lookup is full of them. sim-api leaves
// them out itself (includeAiDialogs) of actor/search/list, actor/context/get, actor/backlinks/get
// and the walks; for the layer search and a dialog's own thread, which it does not filter, the
// plugin reads metaInfo and drops them after the page is read.

// aiDialogKey is the meta_info key that marks an Events actor as an AI Console dialog.
const aiDialogKey = "aiDialogParentId"

// MetaInfoField is the actor projection key that carries meta_info.
const MetaInfoField = "metaInfo"

// IsAIDialog says whether an actor object (any shape with metaInfo) is an AI Console dialog.
func IsAIDialog(actor json.RawMessage) bool {
	var a struct {
		MetaInfo map[string]json.RawMessage `json:"metaInfo"`
	}
	if json.Unmarshal(actor, &a) != nil {
		return false
	}
	var parent string
	return json.Unmarshal(a.MetaInfo[aiDialogKey], &parent) == nil && parent != ""
}

// WithMetaInfo adds metaInfo to a projection that names fields; an empty projection already
// returns every column, metaInfo among them. It answers whether the caller asked for metaInfo itself
// (named it, or sent no projection), so the filter leaves it in the rows.
func WithMetaInfo(fields []string) ([]string, bool) {
	if len(fields) == 0 || slices.Contains(fields, MetaInfoField) {
		return fields, true
	}
	return append(slices.Clone(fields), MetaInfoField), false
}

// DropAIDialogs removes the AI Console dialogs from the rows of list (a row is the actor, or a
// placement row whose actor is under "actor") and, unless keepMeta, the metaInfo the filter read.
// It answers the rows and how many were dropped.
func DropAIDialogs(list []json.RawMessage, keepMeta bool) ([]json.RawMessage, int) {
	out := make([]json.RawMessage, 0, len(list))
	for _, row := range list {
		var doc map[string]json.RawMessage
		if json.Unmarshal(row, &doc) != nil || doc == nil {
			out = append(out, row)
			continue
		}
		actor, nested := doc["actor"]
		if !nested {
			actor = row
		}
		if IsAIDialog(actor) {
			continue
		}
		if !keepMeta {
			if nested {
				var a map[string]json.RawMessage
				if json.Unmarshal(actor, &a) == nil && a != nil {
					delete(a, MetaInfoField)
					doc["actor"], _ = json.Marshal(a)
				}
			}
			delete(doc, MetaInfoField)
			row, _ = json.Marshal(doc)
		}
		out = append(out, row)
	}
	return out, len(list) - len(out)
}

// AIDialogs answers which of these actors are AI Console dialogs, in one batch per 100. An actor
// the caller cannot read is not one (its row stays as the backend gave it).
func (c *Client) AIDialogs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	params := make([]any, len(ids))
	for i, id := range ids {
		params[i] = map[string]any{"accId": c.workspace(), "actorId": id, "fields": []string{"id", MetaInfoField}}
	}
	found, err := c.eachItems(ctx, "actor/get", params)
	if err != nil {
		return nil, err
	}
	for i, f := range found {
		if f.err == nil && IsAIDialog(f.raw) {
			out[ids[i]] = true
		}
	}
	return out, nil
}

// DropAIDialogPage leaves the AI Console dialogs out of a {list: [...]} page, keeping its other
// keys, and counts them in aiDialogsHidden.
func DropAIDialogPage(page json.RawMessage, keepMeta bool) json.RawMessage {
	var doc map[string]json.RawMessage
	var list []json.RawMessage
	if json.Unmarshal(page, &doc) != nil || json.Unmarshal(doc["list"], &list) != nil {
		return page
	}
	list, n := DropAIDialogs(list, keepMeta)
	doc["list"], _ = json.Marshal(list)
	if n > 0 {
		doc["aiDialogsHidden"], _ = json.Marshal(n)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return page
	}
	return out
}
