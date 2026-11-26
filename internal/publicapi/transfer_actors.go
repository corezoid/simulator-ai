package publicapi

import (
	"context"
	"encoding/json"
)

// TransferIDs answers the transfer id behind each Transfers actor. A money edge
// (transfersAmount_*) names its transfer through that actor (linkedActorId): control-tasks makes one
// per transfer with its own actor id and ref = the transfer id, so transfer_get needs the ref, not
// the edge's linkedActorId. One batch of actor/get per 100; an actor the caller cannot read (only
// the transfer's creator gets a rule on it) is left out.
func (c *Client) TransferIDs(ctx context.Context, actorIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(actorIDs) == 0 {
		return out, nil
	}
	params := make([]any, len(actorIDs))
	for i, id := range actorIDs {
		params[i] = map[string]any{"accId": c.workspace(), "actorId": id, "fields": []string{"id", "ref"}}
	}
	found, err := c.eachItems(ctx, "actor/get", params)
	if err != nil {
		return nil, err
	}
	for i, f := range found {
		var a struct {
			Ref string `json:"ref"`
		}
		if f.err == nil && json.Unmarshal(f.raw, &a) == nil && a.Ref != "" {
			out[actorIDs[i]] = a.Ref
		}
	}
	return out, nil
}
