package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// history_list answered an empty list to "how did the contract's sum change" (eval asof-07): the
// model narrowed it to the field's title («Сума») while history rows store data.amount, or sent the
// window in seconds. The Around maps the field to the stored key and reads second-precision bounds
// as seconds; the answer says what it sent (fieldKey, note).

// secondsBelow: a to under this is Unix seconds (as milliseconds it would be before March 1973).
const secondsBelow = int64(100_000_000_000)

var historyNote = map[string]string{
	"from":  "Window start, Unix MILLISECONDS (13 digits), exclusive; 0 for the whole history.",
	"to":    "Window end, Unix MILLISECONDS (13 digits), inclusive; now for the whole history.",
	"field": "One field: a template field's id or title (sent as the stored data.<id>), or a column (title, description, links, account, status).",
}

func historyAround(ctx context.Context, c *publicapi.Client, method string, args, _ map[string]json.RawMessage) (json.RawMessage, error) {
	extra := map[string]any{}
	var from, to int64
	if json.Unmarshal(args["from"], &from) == nil && json.Unmarshal(args["to"], &to) == nil && to > 0 && to < secondsBelow {
		args["from"], args["to"] = jsonInt(from*1000), jsonInt(to*1000)
		extra["note"] = "from and to looked like Unix seconds and were read as seconds (send milliseconds)"
	}
	var objType, objID, field string
	_ = json.Unmarshal(args["objType"], &objType)
	_ = json.Unmarshal(args["objId"], &objID)
	if json.Unmarshal(args["field"], &field) == nil && field != "" {
		// A failed lookup (a deleted actor, a template you may not read) sends the field as given:
		// history/list itself still answers.
		if key, err := c.HistoryField(ctx, objType, objID, field); err == nil && key != field {
			args["field"], _ = json.Marshal(key)
			extra["fieldKey"] = key
		}
	}
	v, err := c.Pass(ctx, method, args)
	if err != nil || len(extra) == 0 {
		return v, err
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(v, &doc) != nil || doc == nil {
		return v, nil
	}
	for k, x := range extra {
		doc[k], _ = json.Marshal(x)
	}
	return json.Marshal(doc)
}
