package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// The AI Console's own dialogs (Events actors with meta_info.aiDialogParentId, their messages
// comments under them) mention every actor a person talked about (eval 2026-11-24). sim-api leaves
// them out of the actor search, the card, the backlinks and the walks itself (includeAiDialogs, a
// contract param passed through). The layer search and a dialog's own thread it does not filter:
// there the plugin reads metaInfo and filters the page it got, so a page may come back shorter than
// its limit, and aiDialogsHidden says how many were left out.

// dialogsLocal is the includeAiDialogs switch of the search tools (a Local param, never sent).
var dialogsLocal = map[string]json.RawMessage{"includeAiDialogs": json.RawMessage(`{"type":"boolean"}`)}

// withFull adds the full switch to a tool's other Local params.
func withFull(local map[string]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range fullLocal {
		out[k] = v
	}
	for k, v := range local {
		out[k] = v
	}
	return out
}

func includeDialogs(local map[string]json.RawMessage) bool {
	return string(local["includeAiDialogs"]) == "true"
}

// withoutDialogRows is the Around of an actor search: metaInfo joins the projection and the
// dialogs leave the page (their metaInfo too, unless the caller asked for it).
func withoutDialogRows(ctx context.Context, c *publicapi.Client, method string, args, local map[string]json.RawMessage) (json.RawMessage, error) {
	if includeDialogs(local) {
		return c.Pass(ctx, method, args)
	}
	var fields []string
	if len(args["fields"]) > 0 && json.Unmarshal(args["fields"], &fields) != nil {
		return c.Pass(ctx, method, args) // the backend refuses a bad projection itself
	}
	fields, keepMeta := publicapi.WithMetaInfo(fields)
	if len(fields) > 0 {
		args["fields"], _ = json.Marshal(fields)
	}
	v, err := c.Pass(ctx, method, args)
	if err != nil {
		return nil, err
	}
	return publicapi.DropAIDialogPage(v, keepMeta), nil
}

// dialogThreadNote answers reactions_list under an AI Console dialog without includeAiDialogs.
const dialogThreadNote = "rootActorId is an AI Console dialog: its messages are the console's own conversation, not workspace knowledge. Send includeAiDialogs: true only when the person asks about that dialog."

// withoutDialogThread is the Around of reactions_list: the thread of an AI Console dialog is read
// only with includeAiDialogs (one more small read: the root's metaInfo).
func withoutDialogThread(ctx context.Context, c *publicapi.Client, method string, args, local map[string]json.RawMessage) (json.RawMessage, error) {
	var root string
	if !includeDialogs(local) && json.Unmarshal(args["rootActorId"], &root) == nil && root != "" {
		dialogs, err := c.AIDialogs(ctx, []string{root})
		if err != nil {
			return nil, err
		}
		if dialogs[root] {
			return json.Marshal(map[string]any{"list": []any{}, "aiDialog": true, "note": dialogThreadNote})
		}
	}
	return c.Pass(ctx, method, args)
}
