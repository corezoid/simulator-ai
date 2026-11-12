package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// The simulation snapshot reads a layer with its actors' data and account balances (legacy
// simulationSnapshot read /graph_layers and /accounts). Numbers stay json.Number so balances keep
// every decimal digit.

// SimulationNode is one node placement with the actor fields a simulation needs.
type SimulationNode struct {
	ID           string `json:"id"`
	AccessDenied bool   `json:"accessDenied"`
	Position     *struct {
		X json.Number `json:"x"`
		Y json.Number `json:"y"`
	} `json:"position"`
	Actor *struct {
		Title     string          `json:"title"`
		FormID    json.Number     `json:"formId"`
		FormTitle string          `json:"formTitle"`
		Data      json.RawMessage `json:"data"`
	} `json:"actor"`
}

// SimulationEdge is one edge placement with its edge.
type SimulationEdge struct {
	ID   string `json:"id"`
	Edge *struct {
		Source        string `json:"source"`
		Target        string `json:"target"`
		EdgeTypeID    int64  `json:"edgeTypeId"`
		LinkedActorID string `json:"linkedActorId"`
	} `json:"edge"`
}

// SimulationAccount is one account row of an actor (one income side of a pair).
type SimulationAccount struct {
	NameID     string      `json:"nameId"`
	CurrencyID json.Number `json:"currencyId"`
	Amount     json.Number `json:"amount"`
	IncomeType string      `json:"incomeType"`
	Type       string      `json:"type"`
	Formula    string      `json:"formula"`
	Meta       struct {
		AccountName  string `json:"accountName"`
		CurrencyName string `json:"currencyName"`
		IsSystem     bool   `json:"isSystem"`
	} `json:"meta"`
}

// SimulationNodes reads every node placement of a layer with each actor's data and template title.
func (c *Client) SimulationNodes(ctx context.Context, layerID string) ([]SimulationNode, error) {
	q := layerPageRequest{AccID: c.workspace(), ActorID: layerID,
		Fields: []string{"id", "title", "formId", "data"}, With: []string{"formTitle"}}
	return allLayerPages[SimulationNode](ctx, c, "graph/layer/actors/list", "node", q)
}

// SimulationEdges reads every edge placement of a layer.
func (c *Client) SimulationEdges(ctx context.Context, layerID string) ([]SimulationEdge, error) {
	q := layerPageRequest{AccID: c.workspace(), ActorID: layerID}
	return allLayerPages[SimulationEdge](ctx, c, "graph/layer/edges/list", "edge", q)
}

type edgeTypesRequest struct {
	AccID  string `json:"accId"`
	Limit  int64  `json:"limit"`
	Offset int64  `json:"offset"`
}

// EdgeTypeNames maps the workspace's edge type ids to their names. ListGraphEdgeTypes reads the same
// rows but only in the graph-read profile, so this shares its wire type, not its gate.
func (c *Client) EdgeTypeNames(ctx context.Context) (map[int64]string, error) {
	if !c.ParityAllowed("edge_type/list") {
		return nil, errors.New("edge_type/list is not available in this profile")
	}
	out := map[int64]string{}
	q := edgeTypesRequest{AccID: c.workspace(), Limit: 1000}
	for page := 0; page < maxLayerPages; page++ {
		types, err := call[edgeTypesRequest, graphEdgeTypesWire](ctx, c, "edge_type/list", q)
		if err != nil {
			return nil, err
		}
		for _, t := range types.List {
			out[t.ID] = t.Name
		}
		if int64(len(types.List)) < q.Limit {
			return out, nil
		}
		q.Offset += q.Limit
	}
	return nil, errors.New("too many edge types")
}

type simulationAccountsRequest struct {
	AccID       string   `json:"accId"`
	ActorID     string   `json:"actorId"`
	AccountType string   `json:"accountType"`
	With        []string `json:"with"`
	From        int64    `json:"from,omitempty"`
	To          int64    `json:"to,omitempty"`
	Limit       int64    `json:"limit"`
	Offset      int64    `json:"offset"`
}

// SimulationAccounts reads every fact account row of an actor with its name and currency. With from
// and to (Unix ms) the amounts are the turnover of that window instead of the balance.
func (c *Client) SimulationAccounts(ctx context.Context, actorID string, from, to int64) ([]SimulationAccount, error) {
	if !c.ParityAllowed("account_actor/list") {
		return nil, errors.New("account_actor/list is not available in this profile")
	}
	if !validID(actorID) {
		return nil, errors.New("actorId required")
	}
	out := []SimulationAccount{}
	q := simulationAccountsRequest{AccID: c.workspace(), ActorID: actorID, AccountType: "fact",
		With: []string{"meta"}, From: from, To: to, Limit: 100}
	for page := 0; page < maxLayerPages; page++ {
		rows, err := call[simulationAccountsRequest, listWire[SimulationAccount]](ctx, c, "account_actor/list", q)
		if err != nil {
			return nil, err
		}
		out = append(out, rows.List...)
		if int64(len(rows.List)) < q.Limit {
			return out, nil
		}
		q.Offset += q.Limit
	}
	return nil, fmt.Errorf("actor has more than %d account rows", maxLayerPages*q.Limit)
}

// Workspace is the configured workspace (accId) every call works in.
func (c *Client) Workspace() string { return c.workspace() }
