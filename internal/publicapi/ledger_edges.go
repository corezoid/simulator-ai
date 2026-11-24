package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// The graph's money edges (transfersAmount_<nameId>_<currencyId>, role transactions) and form-field
// transfer edges are drawn by the platform from a transfer: their weight is the amount moved and
// their linked actor the transfer. An edge drawn by hand, re-pointed or relinked lies about money
// (eval write-10: asked to "draw a transfersAmount edge directly", the model did, with edge_create).
// Every edge write refuses them; ontology_apply already does for its edges.

// ledgerEdgeNote is what the refusal tells the model to do instead.
const ledgerEdgeNote = "is drawn by the platform from a transfer, never by hand: create the transfer (transfer_create, or ontology_apply transfers) and the edge appears by itself"

// LedgerEdgeError refuses an edge type a transfer draws, named as the caller named it.
func LedgerEdgeError(typeName string) error {
	if !ledgerDrawnEdge(strings.TrimSpace(typeName)) {
		return nil
	}
	return fmt.Errorf("edge type %s %s", typeName, ledgerEdgeNote)
}

// ledgerEdgeTypeByID answers whether an edge type id is one a transfer draws: its name from
// ontology/get with the per-account types (role transactions, or a transfersAmount/
// transfersFormField name).
func (c *Client) ledgerEdgeTypeByID(ctx context.Context, id int64) (string, bool, error) {
	digest, err := call[map[string]any, struct {
		EdgeTypes []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"edgeTypes"`
	}](ctx, c, "ontology/get", map[string]any{"accId": c.workspace(), "fields": []string{"edgeTypes"}, "with": []string{"accountEdgeTypes"}, "limit": 1})
	if err != nil {
		return "", false, err
	}
	for _, t := range digest.EdgeTypes {
		if t.ID == id {
			return t.Name, t.Role == "transactions" || ledgerDrawnEdge(t.Name), nil
		}
	}
	return "", false, nil
}

// CheckEdgeRewrite refuses re-pointing an existing money edge (edge_endpoints_update) or changing
// its linked transfer (edge_update linkedActorId / hole): the edge would then name other parties or
// another transfer than the ledger holds. Labels and looks stay editable.
func (c *Client) CheckEdgeRewrite(ctx context.Context, edgeID string, args map[string]json.RawMessage, endpoints bool) error {
	if !endpoints && args["linkedActorId"] == nil && args["hole"] == nil {
		return nil
	}
	edge, err := call[map[string]any, struct {
		EdgeTypeID int64 `json:"edgeTypeId"`
	}](ctx, c, "edge/get", map[string]any{"accId": c.workspace(), "edgeId": edgeID})
	if err != nil || edge.EdgeTypeID < 1 {
		return nil // the write itself answers a missing edge
	}
	name, money, err := c.ledgerEdgeTypeByID(ctx, edge.EdgeTypeID)
	if err != nil || !money {
		return nil // without ontology/get the backend's own rules stay
	}
	what := "its linked transfer"
	if endpoints {
		what = "its source and target"
	}
	return fmt.Errorf("edge %s is a %s edge: %s cannot change. It %s; to correct a payment post a reversing transfer", edgeID, name, what, ledgerEdgeNote)
}
