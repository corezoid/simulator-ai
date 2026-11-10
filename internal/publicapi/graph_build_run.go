package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// atomicItemError is an atomic batch refused by one of its items: nothing of the batch was written.
type atomicItemError struct {
	Index int
	Cause *APIError
}

func (e *atomicItemError) Error() string {
	return fmt.Sprintf("item %d failed, nothing was created: %v", e.Index, e.Cause)
}
func (e *atomicItemError) Unwrap() error { return e.Cause }

// errRolledBack is an atomic batch the backend rolled back whole.
var errRolledBack = errors.New("batch rolled back, nothing was created")

// buildRun is the state of one call's writes.
type buildRun struct {
	b      *GraphBuild
	in     GraphBuildInput
	start  time.Time
	out    GraphBuildResult
	placed []buildPlaced // per node of the call, LaID 0 while not on the layer
	// stopped: a batch failed or the budget ran out; what is left is reported pending.
	stopped bool
}

// Run builds the nodes and edges that are not on the layer yet. Input errors come back as an error
// before anything is written; once writing started the answer says what landed, and the same call
// again builds the rest.
func (b *GraphBuild) Run(ctx context.Context, in GraphBuildInput) (GraphBuildResult, error) {
	if err := in.Validate(); err != nil {
		return GraphBuildResult{}, err
	}
	end, err := b.endpoints(in)
	if err != nil {
		return GraphBuildResult{}, err
	}
	r := &buildRun{b: b, in: in, start: b.start, placed: make([]buildPlaced, len(in.Nodes))}
	r.out = GraphBuildResult{LayerID: b.LayerID, Nodes: BuildCount{Total: len(in.Nodes)}, Edges: BuildCount{Total: len(in.Edges)}, Chunks: []BuildChunks{}}
	var pending []int
	var fresh []BuildNode
	for i, n := range in.Nodes {
		if p, ok := b.refs[BuildRef(b.op, n.Key)]; ok {
			r.placed[i] = p
			r.out.Nodes.Existing++
			continue
		}
		pending, fresh = append(pending, i), append(fresh, n)
	}
	if err = b.checkRoom(fresh); err != nil {
		return GraphBuildResult{}, err
	}
	edges := make([]buildEdgeEnds, len(in.Edges))
	types := map[string]int64{}
	for i, e := range in.Edges {
		if edges[i], err = r.resolveEdge(ctx, e, end, types); err != nil {
			return GraphBuildResult{}, err
		}
	}
	r.buildNodes(ctx, pending)
	r.buildEdges(ctx, edges)
	r.finish()
	return r.out, nil
}

// buildEdgeEnds is an edge with its type id and both ends.
type buildEdgeEnds struct {
	BuildEdge
	typeID   int64
	from, to buildEnd
}

func (r *buildRun) resolveEdge(ctx context.Context, e BuildEdge, end func(string) (buildEnd, error), types map[string]int64) (buildEdgeEnds, error) {
	out := buildEdgeEnds{BuildEdge: e}
	var err error
	if out.from, err = end(e.From); err != nil {
		return out, err
	}
	if out.to, err = end(e.To); err != nil {
		return out, err
	}
	name := edgeTypeName(e.EdgeType)
	id, ok := types[name]
	if !ok {
		c := r.b.c
		t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.config.Workspace, Name: name})
		if err != nil {
			return out, fmt.Errorf("edge type %q: %w", name, err)
		}
		id, types[name] = t.ID, t.ID
	}
	out.typeID = id
	return out, nil
}

// note adds a batch's outcome to the answer, merging runs of one kind and status.
func (r *buildRun) note(kind, status string, items int) {
	if n := len(r.out.Chunks); n > 0 && r.out.Chunks[n-1].Kind == kind && r.out.Chunks[n-1].Status == status {
		r.out.Chunks[n-1].Chunks++
		r.out.Chunks[n-1].Items += items
		return
	}
	r.out.Chunks = append(r.out.Chunks, BuildChunks{Kind: kind, Chunks: 1, Items: items, Status: status})
}

// refTaken says the server refused an actor because its (template, ref) pair is held already.
func refTaken(e *APIError) bool {
	return e != nil && e.Reason == "invalidParams" && strings.Contains(e.Message, "duplicate") && strings.Contains(e.Message, "ref")
}

// fail records a batch that did not land and stops the run. A refused batch wrote nothing; a failure
// without the server's answer (a cut connection) may have written it, which the next call finds out.
func (r *buildRun) fail(kind string, items int, err error, name func(int) string) {
	r.stopped = true
	var item *atomicItemError
	var api *APIError
	switch {
	case errors.As(err, &item):
		r.note(kind, "failed", items)
		r.out.Error = fmt.Sprintf("%s: %v", name(item.Index), item.Cause)
		r.out.Resume = "fix it and repeat the call: the same operationId keeps what landed"
		if refTaken(item.Cause) {
			// The actor exists but the layer read did not show it: a read behind this operation's own
			// write, or the operationId lives on another layer or with another user. A new id after a
			// lagging read would build the graph twice, so the answer names both cases.
			r.out.Resume = "if this operationId already built on this layer, wait a few seconds and repeat the same call; otherwise the id is taken (another layer or user): use a new operationId"
		}
	case errors.Is(err, errRolledBack):
		r.note(kind, "failed", items)
		r.out.Error = err.Error()
		r.out.Resume = "repeat the call: the same operationId keeps what landed"
	case errors.As(err, &api) && api.HTTPStatus == 0 && api.Reason != "timeout":
		// The server answered with a refusal: repeating the same call unchanged meets it again.
		r.note(kind, "failed", items)
		r.out.Error = err.Error()
		r.out.Resume = "the server refused it; fix the cause, then repeat the call: the same operationId builds only what is missing"
	default:
		r.note(kind, "unknown", items)
		r.out.Error = err.Error()
		r.out.Resume = "no answer from the server; repeat the call: the same operationId builds only what is missing"
	}
}

// overBudget stops the run when the call has written for too long.
func (r *buildRun) overBudget(ctx context.Context) bool {
	if !r.stopped && (time.Since(r.start) > buildBudget || ctx.Err() != nil) {
		r.stopped = true
		r.out.Resume = "time budget used; repeat the call: the same operationId builds the rest"
	}
	return r.stopped
}

type buildCreated struct {
	ID string `json:"id"`
}

// nodeChunks cuts the pending nodes into batches by count and request size.
func (r *buildRun) nodeChunks(pending []int) ([][]int, [][]atomicItem, []layerManageRequest) {
	c := r.b.c
	var chunks [][]int
	var creates [][]atomicItem
	var places []layerManageRequest
	size := 0
	for _, i := range pending {
		n := r.in.Nodes[i]
		data, _ := objectData(n.Data, false)
		color := n.Color
		if color == "" {
			color = RandomActorColor()
		}
		create := atomicItem{Method: "actor/create", Params: actorCreateRequest{AccID: c.config.Workspace, FormID: n.FormID, Title: n.Title, Description: n.Description,
			Color: color, Data: data, Hole: n.Hole, Ref: BuildRef(r.b.op, n.Key), AddToRecent: notRecent, Fields: []string{"id"}}}
		at := n.Position
		place := GraphPlacementItem{Action: "create", Data: GraphPlacementData{Type: "node", Position: &at, LayerSettings: n.LayerSettings, AreaPicture: n.AreaPicture}}
		a, _ := json.Marshal(create)
		p, _ := json.Marshal(place)
		k := len(chunks) - 1
		if k < 0 || len(chunks[k]) >= buildChunk || size+len(a)+len(p) > buildChunkBytes {
			chunks, creates = append(chunks, nil), append(creates, nil)
			places = append(places, layerManageRequest{AccID: c.config.Workspace, ActorID: r.b.LayerID})
			k, size = k+1, 0
		}
		place.Data.ID = "$" + strconv.Itoa(len(chunks[k])) + ".id"
		chunks[k], creates[k] = append(chunks[k], i), append(creates[k], create)
		places[k].Items = append(places[k].Items, place)
		size += len(a) + len(p)
	}
	return chunks, creates, places
}

// buildNodes creates and places the pending nodes, a batch at a time: each batch creates its actors
// and places them by reference ("$<index>.id") in one transaction.
func (r *buildRun) buildNodes(ctx context.Context, pending []int) {
	chunks, creates, places := r.nodeChunks(pending)
	for k, chunk := range chunks {
		if r.overBudget(ctx) {
			r.note("nodes", "pending", len(chunk))
			continue
		}
		items := append(creates[k], atomicItem{Method: "graph/layer/actors/manage", Params: places[k], UseRefs: true})
		results, err := r.b.c.atomicItems(ctx, items)
		var placed GraphCreatedPlacements
		if err == nil && (json.Unmarshal(results[len(chunk)], &placed) != nil || len(placed.Nodes) != len(chunk)) {
			err = errors.New("batch committed but graph/layer/actors/manage returned other placements; call again with the same operationId")
		}
		if err != nil {
			r.fail("nodes", len(chunk), err, func(i int) string {
				if i < len(chunk) {
					return fmt.Sprintf("node %q", r.in.Nodes[chunk[i]].Key)
				}
				return "placing the batch"
			})
			continue
		}
		for j, i := range chunk {
			var actor buildCreated
			_ = json.Unmarshal(results[j], &actor)
			r.placed[i] = buildPlaced{ActorID: actor.ID, LaID: placed.Nodes[j].LaID, At: r.in.Nodes[i].Position}
		}
		r.out.Nodes.Done += len(chunk)
		r.note("nodes", "ok", len(chunk))
	}
}

// drawnEdges is the ids of the edges the layer already shows, empty for a layer that was empty.
func (r *buildRun) drawnEdges(ctx context.Context) (map[string]bool, error) {
	drawn := map[string]bool{}
	if r.b.fresh {
		return drawn, nil
	}
	rows, err := allLayerPages[layerEdgeWire](ctx, r.b.c, "graph/layer/edges/list", "edge", layerPageRequest{AccID: r.b.c.config.Workspace, ActorID: r.b.LayerID})
	for _, e := range rows {
		drawn[e.ID] = true
	}
	return drawn, err
}

// buildEdges creates the edges and draws them, a batch at a time, once every node is on the layer.
// On a layer that showed no edge one transaction creates and draws a batch; otherwise the edges are
// created first (edge/create answers an existing edge unchanged) and only those not drawn are placed.
func (r *buildRun) buildEdges(ctx context.Context, edges []buildEdgeEnds) {
	if len(edges) == 0 {
		return
	}
	var drawn map[string]bool
	if !r.stopped {
		var err error
		if drawn, err = r.drawnEdges(ctx); err != nil {
			r.stopped, r.out.Error, r.out.Resume = true, err.Error(), "nodes are built; repeat the call to draw the edges"
		}
	}
	c := r.b.c
	for start := 0; start < len(edges); start += buildChunk {
		chunk := edges[start:min(start+buildChunk, len(edges))]
		if r.overBudget(ctx) {
			r.note("edges", "pending", len(chunk))
			continue
		}
		items := make([]atomicItem, len(chunk), len(chunk)+1)
		place := layerManageRequest{AccID: c.config.Workspace, ActorID: r.b.LayerID, Items: make([]GraphPlacementItem, len(chunk))}
		for j, e := range chunk {
			from, to := r.end(e.from), r.end(e.to)
			items[j] = atomicItem{Method: "edge/create", Params: edgeCreateRequest{AccID: c.config.Workspace, Source: from.ActorID, Target: to.ActorID, EdgeTypeID: e.typeID, Name: e.Name, ForceDirection: true}}
			src, dst := from.LaID, to.LaID
			place.Items[j] = GraphPlacementItem{Action: "create", Data: GraphPlacementData{ID: "$" + strconv.Itoa(j) + ".id", Type: "edge", LaIDSource: &src, LaIDTarget: &dst}}
		}
		existing, err := r.drawChunk(ctx, items, place, drawn)
		if err != nil {
			r.fail("edges", len(chunk), err, func(i int) string {
				if i < len(chunk) {
					return fmt.Sprintf("edge %s -> %s", chunk[i].From, chunk[i].To)
				}
				return "drawing the batch"
			})
			continue
		}
		r.out.Edges.Existing += existing
		r.out.Edges.Done += len(chunk) - existing
		r.note("edges", "ok", len(chunk))
	}
}

// end is an edge end's actor and placement, known by now for a node of this call.
func (r *buildRun) end(e buildEnd) buildPlaced {
	if e.node >= 0 {
		return r.placed[e.node]
	}
	return e.placed
}

// drawChunk creates a batch of edges and draws those the layer does not show; it answers how many
// were already drawn.
func (r *buildRun) drawChunk(ctx context.Context, creates []atomicItem, place layerManageRequest, drawn map[string]bool) (int, error) {
	c := r.b.c
	if len(drawn) == 0 {
		_, err := c.atomicItems(ctx, append(creates, atomicItem{Method: "graph/layer/actors/manage", Params: place, UseRefs: true}))
		return 0, err
	}
	results, err := c.atomicItems(ctx, creates)
	if err != nil {
		return 0, err
	}
	var missing []GraphPlacementItem
	for j, raw := range results {
		var edge buildCreated
		if json.Unmarshal(raw, &edge) != nil || edge.ID == "" {
			return 0, errors.New("edge/create returned no edge id")
		}
		if !drawn[edge.ID] {
			drawn[edge.ID] = true
			item := place.Items[j]
			item.Data.ID = edge.ID
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		place.Items = missing
		if _, err = call[layerManageRequest, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", place); err != nil {
			return 0, err
		}
	}
	return len(creates) - len(missing), nil
}

// finish sums the run up: whether everything is on the layer and the area.
func (r *buildRun) finish() {
	r.out.Done = !r.stopped
	if r.out.Done {
		r.out.Chunks = nil
	}
	if r.b.room.Partial {
		r.out.Warning = fmt.Sprintf("the layer has more than %d placements: free cells and what an earlier call built were checked on the first ones only", maxRoomPages*layerPage)
	}
	if len(r.in.Nodes) == 0 {
		return
	}
	area := r.in.Nodes[0].footprint()
	want := map[string]bool{}
	for _, key := range r.in.IDs {
		want[key] = true
	}
	ids := map[string]string{}
	for i, n := range r.in.Nodes {
		if p := r.placed[i]; p.LaID > 0 {
			n.Position = p.At
			if want[n.Key] {
				ids[n.Key] = p.ActorID
			}
		}
		area = area.union(n.footprint())
	}
	r.out.Area = areaCells(area)
	if len(ids) > 0 {
		r.out.IDs = ids
	}
}

// GraphBuildDeleteInput removes what an operation built on a layer.
type GraphBuildDeleteInput struct {
	LayerID     string `json:"layerId"`
	OperationID string `json:"operationId"`
	DryRun      bool   `json:"dryRun,omitempty"`
}

// GraphBuildDeleted counts the operation's actors found on the layer and deleted.
type GraphBuildDeleted struct {
	Found   int      `json:"found"`
	Deleted int      `json:"deleted"`
	DryRun  bool     `json:"dryRun,omitempty"`
	Errors  []string `json:"errors,omitempty"`
	Warning string   `json:"warning,omitempty"`
}

// DeleteGraphBuild deletes the actors an operation created, found by their refs on the layer; their
// placements and edges go with them. A layer the operation created stays.
func (c *Client) DeleteGraphBuild(ctx context.Context, in GraphBuildDeleteInput) (GraphBuildDeleted, error) {
	if e := consoleOnly(c); e != nil {
		return GraphBuildDeleted{}, e
	}
	if !validID(in.LayerID) || !operationPattern.MatchString(in.OperationID) {
		return GraphBuildDeleted{}, fmt.Errorf("layerId required; %w", errOperationID)
	}
	prefix, layerRef := BuildRef(in.OperationID, ""), BuildRef(in.OperationID, buildLayerKey)
	seen := map[string]bool{}
	var ids []string
	partial, err := c.scanLayerNodes(ctx, in.LayerID, []string{"id", "ref"}, func(n layerNodeFull) {
		if n.Actor != nil && n.Actor.Ref != nil && strings.HasPrefix(*n.Actor.Ref, prefix) && *n.Actor.Ref != layerRef && !seen[n.ID] {
			seen[n.ID] = true
			ids = append(ids, n.ID)
		}
	})
	if err != nil {
		return GraphBuildDeleted{}, err
	}
	out := GraphBuildDeleted{Found: len(ids), DryRun: in.DryRun}
	if partial {
		out.Warning = fmt.Sprintf("the layer has more than %d placements: only the first were searched; run again after this pass", maxRoomPages*layerPage)
	}
	if in.DryRun {
		return out, nil
	}
	for start := 0; start < len(ids); start += 100 {
		chunk := ids[start:min(start+100, len(ids))]
		items := make([]map[string]json.RawMessage, len(chunk))
		for i, id := range chunk {
			raw, _ := json.Marshal(id)
			items[i] = map[string]json.RawMessage{"actorId": raw}
		}
		results, err := c.PassEach(ctx, "actor/delete", items)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%d actors not deleted: %v", len(ids)-start, err))
			return out, nil
		}
		for _, r := range results {
			if r.Error == "" {
				out.Deleted++
			} else if len(out.Errors) < 5 {
				out.Errors = append(out.Errors, chunk[r.Index]+": "+r.Error)
			}
		}
	}
	return out, nil
}
