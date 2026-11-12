package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ActorsCreateInput creates 1..100 actors, of one or several templates, in one call. With
// OperationID a repeated call answers the actors the first one created instead of new ones.
type ActorsCreateInput struct {
	Actors      []ActorCreateInput `json:"actors"`
	OperationID string             `json:"operationId,omitempty"`
}

// EdgesCreateInput creates 1..100 edges in one call.
type EdgesCreateInput struct {
	Edges []EdgeCreateInput `json:"edges"`
}

// CreatedActor is a created actor's metadata with its colour, which layer_placements_create
// paints flowchart blocks with.
type CreatedActor struct {
	Actor
	Color string `json:"color,omitempty"`
	Hole  bool   `json:"hole,omitempty"`
}

type CreatedActors struct {
	Actors []CreatedActor `json:"actors"`
}
type CreatedEdges struct {
	Edges []GraphCreatedEdge `json:"edges"`
}

type atomicItem struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
	// UseRefs resolves "$<index>.id" in params to an earlier item's result id (sim-api rpc_batch refs).
	UseRefs bool `json:"useRefs,omitempty"`
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
	items := make([]atomicItem, len(params))
	for i, p := range params {
		items[i] = atomicItem{Method: method, Params: p}
	}
	return c.atomicItems(ctx, items)
}

// atomicItems runs the items, of any methods, in one atomic public batch, in order.
func (c *Client) atomicItems(ctx context.Context, items []atomicItem) ([]json.RawMessage, error) {
	if len(items) < 1 || len(items) > 100 {
		return nil, errors.New("1..100 items required")
	}
	req := atomicRequest{IsAtomic: true, List: make([]atomicItem, len(items))}
	for i, item := range items {
		item.ID = strconv.Itoa(i)
		req.List[i] = item
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
	if len(wire.List) != len(items) {
		return nil, errors.New("batch returned a different number of items; read the items back before retrying")
	}
	out := make([]json.RawMessage, len(items))
	for i, item := range wire.List {
		if item.Error != nil {
			return nil, &atomicItemError{Index: i, Cause: c.apiError(item.Error)}
		}
		out[i] = item.Result
	}
	if wire.RolledBack {
		return nil, errRolledBack
	}
	return out, nil
}

// createdActorFields is what a bulk creation answers per actor.
var createdActorFields = append(actorFields[:len(actorFields):len(actorFields)], "color", "hole")

// CreateActors creates the actors atomically and returns them in input order. They stay out of the
// caller's recent list. With an operationId each actor carries the ref "gb:<operationId>:<index>"
// (unique within its template), and a call that finds the first one answers the earlier call's
// actors: a retry after a lost answer creates nothing twice.
func (c *Client) CreateActors(ctx context.Context, in ActorsCreateInput) (CreatedActors, error) {
	if e := consoleOnly(c); e != nil {
		return CreatedActors{}, e
	}
	if in.OperationID != "" && !operationPattern.MatchString(in.OperationID) {
		return CreatedActors{}, errOperationID
	}
	params := make([]any, len(in.Actors))
	refs := make([]actorRef, len(in.Actors))
	for i, a := range in.Actors {
		if a.FormID < 1 || !validTitle(a.Title, false) {
			return CreatedActors{}, fmt.Errorf("actor %d: positive formId and a 1..1000 character title required", i)
		}
		data, err := objectData(a.Data, false)
		if err != nil {
			return CreatedActors{}, fmt.Errorf("actor %d: %w", i, err)
		}
		color := a.Color
		if color == "" {
			color = RandomActorColor()
		}
		req := actorCreateRequest{AccID: c.workspace(), FormID: a.FormID, Title: a.Title, Description: a.Description, Color: color, Data: data, Hole: a.Hole, AddToRecent: notRecent, Fields: createdActorFields}
		if in.OperationID != "" {
			req.Ref = BuildRef(in.OperationID, strconv.Itoa(i))
			refs[i] = actorRef{FormID: a.FormID, Ref: req.Ref}
		}
		params[i] = req
	}
	if in.OperationID != "" && len(refs) > 0 {
		earlier, found, err := c.actorsByRef(ctx, refs)
		if err == nil && found && !sameTitles(earlier, in.Actors) {
			return CreatedActors{}, fmt.Errorf("operationId %q already created other actors (the first is %q): use a new operationId", in.OperationID, deref(earlier.Actors[0].Title))
		}
		if err != nil || found {
			return earlier, err
		}
	}
	results, err := c.atomic(ctx, "actor/create", params)
	if err != nil {
		if item := (*atomicItemError)(nil); in.OperationID != "" && errors.As(err, &item) && refTaken(item.Cause) {
			return CreatedActors{}, fmt.Errorf("operationId %q is taken (an earlier call, changed since): use a new operationId: %w", in.OperationID, err)
		}
		return CreatedActors{}, err
	}
	out := CreatedActors{Actors: make([]CreatedActor, len(results))}
	for i, r := range results {
		if json.Unmarshal(r, &out.Actors[i]) != nil || out.Actors[i].ID == nil || *out.Actors[i].ID == "" {
			return CreatedActors{}, fmt.Errorf("actor %d: batch committed but actor/create returned no actor id; read the actors back before retrying", i)
		}
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// sameTitles says the actors found under an operation's refs are the ones this call sends: at least
// one title matches. A retry matches them all; an operationId reused for other actors matches none,
// and answering those as created would hide that nothing was.
func sameTitles(found CreatedActors, sent []ActorCreateInput) bool {
	for i, a := range found.Actors {
		if i < len(sent) && strings.TrimSpace(deref(a.Title)) == strings.TrimSpace(sent[i].Title) {
			return true
		}
	}
	return false
}

// actorRef names an actor by its template and ref.
type actorRef struct {
	FormID int64  `json:"formId"`
	Ref    string `json:"ref"`
}

type actorRefRequest struct {
	AccID string `json:"accId"`
	actorRef
	Fields []string `json:"fields"`
}

// actorsByRef answers the actors an earlier call created under these refs, found false when the
// first is not there (the creation is atomic: all of them exist or none). An operation that is
// there only in part was changed since (actors deleted, or other input): it is refused.
func (c *Client) actorsByRef(ctx context.Context, refs []actorRef) (CreatedActors, bool, error) {
	get := func(r actorRef) actorRefRequest {
		return actorRefRequest{AccID: c.workspace(), actorRef: r, Fields: createdActorFields}
	}
	first, err := call[actorRefRequest, CreatedActor](ctx, c, "actor/by-ref/get", get(refs[0]))
	if err != nil {
		if api := (*APIError)(nil); errors.As(err, &api) && (api.Reason == "notFound" || api.Reason == "noFoundOrNoAccess") {
			return CreatedActors{}, false, nil
		}
		return CreatedActors{}, false, fmt.Errorf("actor/by-ref/get: %w", err)
	}
	out := CreatedActors{Actors: make([]CreatedActor, len(refs))}
	out.Actors[0] = first
	if len(refs) > 1 {
		items := make([]atomicItem, len(refs)-1)
		for i, r := range refs[1:] {
			items[i] = atomicItem{Method: "actor/by-ref/get", Params: get(r)}
		}
		results, err := c.atomicItems(ctx, items)
		if err != nil {
			return CreatedActors{}, false, fmt.Errorf("this operationId was used by a call with other actors, or some of its actors were deleted; use a new operationId: %w", err)
		}
		for i, r := range results {
			if json.Unmarshal(r, &out.Actors[i+1]) != nil {
				return CreatedActors{}, false, fmt.Errorf("actor %d: actor/by-ref/get result shape mismatch", i+1)
			}
		}
	}
	for i, a := range out.Actors {
		if a.ID == nil || *a.ID == "" {
			return CreatedActors{}, false, fmt.Errorf("actor %d: actor/by-ref/get returned no actor id", i)
		}
	}
	return out, true, nil
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
			t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.workspace(), Name: name})
			if err != nil {
				return CreatedEdges{}, fmt.Errorf("edge type %q: %w", name, err)
			}
			id, types[name] = t.ID, t.ID
		}
		params[i] = edgeCreateRequest{AccID: c.workspace(), Source: e.Source, Target: e.Target, EdgeTypeID: id, Name: e.Name, CurveStyle: e.CurveStyle, Hole: e.Hole, ForceDirection: true}
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
