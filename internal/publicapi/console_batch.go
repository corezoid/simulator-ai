package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ActorsCreateInput creates 1..100 actors, of one or several templates, in one call.
type ActorsCreateInput struct {
	Actors []ActorCreateInput `json:"actors"`
}

// EdgesCreateInput creates 1..100 edges in one call.
type EdgesCreateInput struct {
	Edges []EdgeCreateInput `json:"edges"`
}

type CreatedActors struct {
	Actors []Actor `json:"actors"`
}
type CreatedEdges struct {
	Edges []GraphCreatedEdge `json:"edges"`
}

type atomicItem struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}
type atomicRequest struct {
	IsAtomic bool         `json:"isAtomic"`
	List     []atomicItem `json:"list"`
}
type atomicWire struct {
	IsAtomic   bool `json:"isAtomic"`
	RolledBack bool `json:"rolledBack"`
	List       []struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	} `json:"list"`
}

// atomic runs one method per item in an atomic public batch: every item is created, or none is and
// the error names the first failed item by index.
func (c *Client) atomic(ctx context.Context, method string, params []any) ([]json.RawMessage, error) {
	if len(params) < 1 || len(params) > 100 {
		return nil, errors.New("1..100 items required")
	}
	req := atomicRequest{IsAtomic: true, List: make([]atomicItem, len(params))}
	for i, p := range params {
		req.List[i] = atomicItem{ID: strconv.Itoa(i), Method: method, Params: p}
	}
	wire, err := call[atomicRequest, atomicWire](ctx, c, "batch", req)
	if err != nil {
		return nil, err
	}
	// A backend without atomic batches ignores isAtomic and keeps what succeeded: never report that
	// as "nothing was created".
	if !wire.IsAtomic {
		return nil, errors.New("the backend did not run the batch atomically: some items may be created; read them back before retrying")
	}
	if len(wire.List) != len(params) {
		return nil, errors.New("batch returned a different number of items; read the items back before retrying")
	}
	out := make([]json.RawMessage, len(params))
	for i, item := range wire.List {
		if item.Error != nil {
			cause := c.apiError(item.Error)
			return nil, fmt.Errorf("item %d failed, nothing was created: %w", i, cause)
		}
		out[i] = item.Result
	}
	if wire.RolledBack {
		return nil, errors.New("batch rolled back, nothing was created")
	}
	return out, nil
}

// CreateActors creates the actors atomically and returns them in input order.
func (c *Client) CreateActors(ctx context.Context, in ActorsCreateInput) (CreatedActors, error) {
	if e := consoleOnly(c); e != nil {
		return CreatedActors{}, e
	}
	params := make([]any, len(in.Actors))
	for i, a := range in.Actors {
		if a.FormID < 1 || !validTitle(a.Title, false) {
			return CreatedActors{}, fmt.Errorf("actor %d: positive formId and a 1..1000 character title required", i)
		}
		data, err := objectData(a.Data, false)
		if err != nil {
			return CreatedActors{}, fmt.Errorf("actor %d: %w", i, err)
		}
		params[i] = actorCreateRequest{AccID: c.config.Workspace, FormID: a.FormID, Title: a.Title, Description: a.Description, Color: a.Color, Data: data, Fields: actorFields}
	}
	results, err := c.atomic(ctx, "actor/create", params)
	if err != nil {
		return CreatedActors{}, err
	}
	out := CreatedActors{Actors: make([]Actor, len(results))}
	for i, r := range results {
		if json.Unmarshal(r, &out.Actors[i]) != nil || out.Actors[i].ID == nil || *out.Actors[i].ID == "" {
			return CreatedActors{}, fmt.Errorf("actor %d: batch committed but actor/create returned no actor id; read the actors back before retrying", i)
		}
	}
	return out, nil
}

// CreateEdges creates the edges atomically, each in its given direction, and returns them in order.
func (c *Client) CreateEdges(ctx context.Context, in EdgesCreateInput) (CreatedEdges, error) {
	if e := consoleOnly(c); e != nil {
		return CreatedEdges{}, e
	}
	types := map[string]int64{}
	params := make([]any, len(in.Edges))
	for i, e := range in.Edges {
		if !validID(e.Source) || !validID(e.Target) || len([]rune(e.Name)) > 1000 {
			return CreatedEdges{}, fmt.Errorf("edge %d: source and target actor ids required", i)
		}
		name := e.EdgeType
		if name == "" {
			name = "hierarchy"
		}
		id, ok := types[name]
		if !ok {
			t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.config.Workspace, Name: name})
			if err != nil {
				return CreatedEdges{}, fmt.Errorf("edge type %q: %w", name, err)
			}
			id, types[name] = t.ID, t.ID
		}
		params[i] = edgeCreateRequest{AccID: c.config.Workspace, Source: e.Source, Target: e.Target, EdgeTypeID: id, Name: e.Name, CurveStyle: e.CurveStyle, ForceDirection: true}
	}
	results, err := c.atomic(ctx, "edge/create", params)
	if err != nil {
		return CreatedEdges{}, err
	}
	out := CreatedEdges{Edges: make([]GraphCreatedEdge, len(results))}
	for i, r := range results {
		e := &out.Edges[i]
		if json.Unmarshal(r, e) != nil || validateGraphEdge(*e) != nil || e.Source != in.Edges[i].Source || e.Target != in.Edges[i].Target {
			return CreatedEdges{}, fmt.Errorf("edge %d: batch committed but edge/create returned an edge with other endpoints; read the edges back before retrying", i)
		}
	}
	return out, nil
}
