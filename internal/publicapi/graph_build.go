package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// graph_build builds a large graph in one tool call: actors, their placements, edges and the edges'
// placements, in atomic batches the plugin cuts itself, so the model names every node once by a key
// of its own instead of echoing actor ids between four tools.
//
// Retry key. sim-api's Console actor/create has no operation id, but an actor's ref is unique within
// its template (actors_ref_form_id) and comes back with a layer's placements, so every actor of an
// operation carries the ref "gb:<operationId>:<key>". An actor is created and placed in one atomic
// batch: it exists exactly when it is on the layer. A repeated call reads the layer once, finds the
// keys that landed and builds only the rest; edge/create returns an existing edge unchanged, and an
// edge already drawn on the layer is not drawn twice. sim-api clears a deleted actor's ref, so an
// operation that was cleaned up (graph_build_delete) can be built again under the same operationId;
// a ref refused as taken means the operationId lives elsewhere (another layer or user) or the layer
// read was behind the write.

const (
	buildRefPrefix = "gb:"
	// buildLayerKey is the ref key of the layer an operation created; "~" is not a node key character.
	buildLayerKey = "~layer"
	// buildChunk actors or edges and the one graph/layer/actors/manage that places them fill a public
	// batch (at most 100 items, run in order in one transaction under sim-api's 25 s batch budget).
	buildChunk = 99
	// buildChunkBytes keeps one batch well under sim-api's 1 MiB request body.
	buildChunkBytes = 700 << 10
	// MaxBuildNodes and MaxBuildEdges bound one call: about 20 node and 40 edge batches, which fit
	// buildBudget; a model's 16k output tokens carry a few hundred of each anyway.
	MaxBuildNodes = 2000
	MaxBuildEdges = 4000
)

// buildBudget bounds one call's writes: past it the call answers what landed and the same call
// again builds the rest. It leaves room under a 120 s tool-call timeout for the batch in flight.
var buildBudget = 80 * time.Second

var (
	operationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	buildKeyPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	errOperationID   = errors.New("operationId is 1..64 characters of A-Z a-z 0-9 _ -")
)

// BuildRef is the ref of an operation's actor.
func BuildRef(operationID, key string) string { return buildRefPrefix + operationID + ":" + key }

// BuildNode is one node with its final place. LayerSettings and AreaPicture make it a flowchart block.
type BuildNode struct {
	Key           string
	FormID        int64
	Title         string
	Description   string
	Color         string
	Data          json.RawMessage
	Hole          bool
	Position      GraphPoint
	LayerSettings *GraphNativeSettings
	AreaPicture   *GraphAreaPicture
}

// BuildEdge joins two keys: nodes of this call, Existing keys or keys of the Use operations.
type BuildEdge struct {
	From, To, Name, EdgeType string
}

// GraphBuildInput is what one call builds on the session's layer.
type GraphBuildInput struct {
	Nodes []BuildNode
	Edges []BuildEdge
	// Existing names actors already on the layer by a key for the edges.
	Existing map[string]string
	// Use lists earlier operations whose keys the edges may name.
	Use []string
	// IDs lists the keys whose actor ids the answer carries: those the caller needs later.
	IDs []string
}

// BuildCount counts nodes or edges: Done were created or drawn by this call, Existing were there.
type BuildCount struct {
	Total    int `json:"total"`
	Done     int `json:"done"`
	Existing int `json:"existing,omitempty"`
}

// BuildChunks is a run of batches of one kind and outcome: ok, failed (nothing of it was written),
// unknown (no answer: it may have been written) or pending (not sent).
type BuildChunks struct {
	Kind   string `json:"kind"`
	Chunks int    `json:"chunks"`
	Items  int    `json:"items"`
	Status string `json:"status"`
}

// GraphBuildResult is the compact answer of a build: counts, never the input back. Chunks are there
// only when something is left to build.
type GraphBuildResult struct {
	Done    bool          `json:"done"`
	LayerID string        `json:"layerId"`
	Nodes   BuildCount    `json:"nodes"`
	Edges   BuildCount    `json:"edges"`
	Chunks  []BuildChunks `json:"chunks,omitempty"`
	// Area is the box the nodes cover by its corner cells.
	Area string `json:"area,omitempty"`
	// IDs maps the keys asked for to actor ids.
	IDs     map[string]string `json:"ids,omitempty"`
	Error   string            `json:"error,omitempty"`
	Resume  string            `json:"resume,omitempty"`
	Warning string            `json:"warning,omitempty"`
}

// GraphBuildTarget names the layer of a build: an existing one, or a new one by its title.
type GraphBuildTarget struct {
	LayerID     string
	NewLayer    string
	OperationID string
}

// buildPlaced is a node on the layer.
type buildPlaced struct {
	ActorID string
	LaID    int64
	At      GraphPoint
}

// GraphBuild is one call's view of its layer, read once: what holds the grid, which keys of which
// operations are placed, and each actor's placement.
type GraphBuild struct {
	c       *Client
	LayerID string
	op      string
	room    layerRoom
	refs    map[string]buildPlaced
	laIDs   map[string]int64
	// fresh: the layer had no placements, so nothing can be there twice.
	fresh bool
	// start is when the call began: the layer read counts against buildBudget too.
	start time.Time
}

// OpenGraphBuild resolves the layer (creating the new one once per operation) and reads it.
func (c *Client) OpenGraphBuild(ctx context.Context, t GraphBuildTarget) (*GraphBuild, error) {
	if e := consoleOnly(c); e != nil {
		return nil, e
	}
	if !operationPattern.MatchString(t.OperationID) {
		return nil, errOperationID
	}
	if (t.LayerID == "") == (t.NewLayer == "") || t.LayerID != "" && !validID(t.LayerID) || !validTitle(t.NewLayer, true) {
		return nil, errors.New("layerId of an existing layer or newLayer, the title of a new one, required")
	}
	b := &GraphBuild{c: c, LayerID: t.LayerID, op: t.OperationID, refs: map[string]buildPlaced{}, laIDs: map[string]int64{}, start: time.Now()}
	if t.NewLayer != "" {
		created, err := b.newLayer(ctx, t.NewLayer)
		if err != nil || created {
			b.fresh = true
			return b, err
		}
	}
	seen := 0
	partial, err := c.scanLayerNodes(ctx, b.LayerID, append(roomFields[:len(roomFields):len(roomFields)], "ref"), func(n layerNodeFull) {
		seen++
		b.room.occupy(n)
		b.laIDs[n.ID] = n.LaID
		if n.Actor != nil && n.Actor.Ref != nil && strings.HasPrefix(*n.Actor.Ref, buildRefPrefix) {
			x, y, _ := n.center()
			b.refs[*n.Actor.Ref] = buildPlaced{ActorID: n.ID, LaID: n.LaID, At: GraphPoint{X: x, Y: y}}
		}
	})
	b.room.Partial, b.fresh = partial, seen == 0
	return b, err
}

// newLayer finds the layer this operation created, or creates it; created says it is new and empty.
func (b *GraphBuild) newLayer(ctx context.Context, title string) (bool, error) {
	formID, err := b.c.systemFormID(ctx, "Layers")
	if err != nil {
		return false, err
	}
	ref := actorRef{FormID: formID, Ref: BuildRef(b.op, buildLayerKey)}
	earlier, found, err := b.c.actorsByRef(ctx, []actorRef{ref})
	if err != nil {
		return false, err
	}
	if found {
		b.LayerID = *earlier.Actors[0].ID
		return false, nil
	}
	layer, err := call[actorCreateRequest, Actor](ctx, b.c, "actor/create", actorCreateRequest{AccID: b.c.config.Workspace, FormID: formID,
		Title: title, Data: json.RawMessage("{}"), Ref: ref.Ref, Fields: actorFields})
	if err != nil {
		return false, fmt.Errorf("layer: %w", err)
	}
	if layer.ID == nil || *layer.ID == "" {
		return false, errors.New("actor/create returned no layer id")
	}
	b.LayerID = *layer.ID
	return true, nil
}

// Landed is where an earlier call of this operation placed the key.
func (b *GraphBuild) Landed(key string) (GraphPoint, bool) {
	p, ok := b.refs[BuildRef(b.op, key)]
	return p.At, ok
}

// lineAfter is the first grid line at or after v.
func lineAfter(v float64) float64 {
	return math.Ceil((v-gridCell/2)/gridCell)*gridCell + gridCell/2
}

// buildFlowCells is how wide sections built one after another fill a layer before the next goes
// under them: a layer of sections stays a page, not a strip.
const buildFlowCells = 80

// FreeOrigin is the top-left corner for a section of the given width where nothing is in the way:
// cell "A, 1" on an empty layer, else the highest free place within buildFlowCells columns of the
// leftmost occupied one, three free cells from what is beside and above it. So sections fill a row,
// then the next row starts under the lowest of them. One pass over the occupants and the columns:
// no more work or memory for a wider section.
func (b *GraphBuild) FreeOrigin(width float64) GraphPoint {
	if len(b.room.Occupants) == 0 {
		return GraphPoint{X: -gridCell / 2, Y: -gridCell / 2}
	}
	left, top := math.Inf(1), math.Inf(1)
	for _, o := range b.room.Occupants {
		left, top = math.Min(left, o.Box.X), math.Min(top, o.Box.Y)
	}
	lineBefore := func(v float64) float64 { return math.Floor((v-gridCell/2)/gridCell)*gridCell + gridCell/2 }
	left, top = lineBefore(left), lineBefore(top)
	const margin = 3
	if width >= buildFlowCells*gridCell {
		// As wide as the flow or wider (a caller's cells may span the whole plane): one place, under
		// everything it would be beside, with no column table to fill.
		reach := (math.Ceil(width/gridCell) + margin) * gridCell
		for _, o := range b.room.Occupants {
			if o.Box.X-left < reach {
				top = math.Max(top, lineAfter(o.Box.bottom())+margin*gridCell)
			}
		}
		return GraphPoint{X: left, Y: top}
	}
	need := max(1, int(math.Ceil(width/gridCell)))
	const columns = buildFlowCells
	// under[c] is the first line a section in column c may start at.
	under := make([]float64, columns)
	for c := range under {
		under[c] = top
	}
	for _, o := range b.room.Occupants {
		first := int(math.Floor((o.Box.X-left)/gridCell)) - margin
		last := int(math.Ceil((o.Box.right()-left)/gridCell)) - 1 + margin
		for c := max(first, 0); c <= min(last, columns-1); c++ {
			under[c] = math.Max(under[c], lineAfter(o.Box.bottom())+margin*gridCell)
		}
	}
	best, at := math.Inf(1), 0
	for c := 0; c+need <= columns; c++ {
		y := top
		for _, u := range under[c : c+need] {
			y = math.Max(y, u)
		}
		if y < best {
			best, at = y, c
		}
	}
	return GraphPoint{X: left + float64(at)*gridCell, Y: best}
}

// footprint is what the node will hold on the grid.
func (n BuildNode) footprint() graphRect {
	if s := n.LayerSettings; s != nil && s.Width > 0 && s.Height > 0 {
		return graphRect{X: n.Position.X - s.Width/2, Y: n.Position.Y - s.Height/2, W: s.Width, H: s.Height}
	}
	return cellOf(n.Position.X, n.Position.Y)
}

// cellRange is the grid cells a box touches, as index ranges.
func (r graphRect) cellRange() (x0, x1, y0, y1 int) {
	const eps = 1e-6
	first := func(v float64) int { return int(math.Floor((v+gridCell/2)/gridCell + eps)) }
	last := func(v float64) int { return int(math.Ceil((v+gridCell/2)/gridCell-eps)) - 1 }
	return first(r.X), last(r.right()), first(r.Y), last(r.bottom())
}

// checkRoom refuses nodes that overlap each other or what the layer holds; one pass over the cells
// the boxes touch.
func (b *GraphBuild) checkRoom(nodes []BuildNode) error {
	taken := map[[2]int]int{}
	boxes := make([]graphRect, len(nodes))
	area := graphRect{}
	for i, n := range nodes {
		if !finite(n.Position.X, n.Position.Y) {
			return fmt.Errorf("node %q: position out of range", n.Key)
		}
		box := n.footprint()
		boxes[i] = box
		if i == 0 {
			area = box
		}
		area = area.union(box)
		x0, x1, y0, y1 := box.cellRange()
		for x := x0; x <= x1; x++ {
			for y := y0; y <= y1; y++ {
				if j, dup := taken[[2]int{x, y}]; dup && boxes[j].overlaps(box) {
					return fmt.Errorf("nodes %q and %q overlap at %s: give each its own cells", nodes[j].Key, n.Key, CellOf(n.Position))
				}
				taken[[2]int{x, y}] = i
			}
		}
	}
	var hit []occupant
	for _, o := range b.room.Occupants {
		if !o.Box.overlaps(area) {
			continue
		}
		x0, x1, y0, y1 := o.Box.cellRange()
	cellsOfBox:
		for x := x0; x <= x1; x++ {
			for y := y0; y <= y1; y++ {
				if j, ok := taken[[2]int{x, y}]; ok && boxes[j].overlaps(o.Box) {
					hit = append(hit, o)
					break cellsOfBox
				}
			}
		}
	}
	if len(hit) > 0 {
		free := b.FreeOrigin(area.W)
		return fmt.Errorf("occupied: %s. Nothing was written: build on free cells, e.g. from %q (origin, or leave it out; layout given: other cells)", cellsOf(hit),
			CellOf(GraphPoint{X: free.X + gridCell/2, Y: free.Y + gridCell/2}))
	}
	return nil
}

// buildEnd is an edge's endpoint: its actor and placement, or the node of this call that will be.
type buildEnd struct {
	placed buildPlaced
	node   int // index into the call's nodes when not placed yet, else -1
}

// endpoints resolves every key an edge may name: this call's nodes first, then Existing, then the
// Use operations in order.
func (b *GraphBuild) endpoints(in GraphBuildInput) (func(string) (buildEnd, error), error) {
	nodes := map[string]int{}
	for i, n := range in.Nodes {
		nodes[n.Key] = i
	}
	existing := map[string]buildPlaced{}
	for key, actorID := range in.Existing {
		if _, dup := nodes[key]; dup || !buildKeyPattern.MatchString(key) || !validID(actorID) {
			return nil, fmt.Errorf("existing %q: a key no node of this call has, and an actor id, required", key)
		}
		laID, ok := b.laIDs[actorID]
		if !ok {
			return nil, fmt.Errorf("existing %q: actor %s is not placed on the layer; place it first (layer_placements_create)", key, actorID)
		}
		existing[key] = buildPlaced{ActorID: actorID, LaID: laID}
	}
	for _, op := range in.Use {
		if !operationPattern.MatchString(op) {
			return nil, fmt.Errorf("use %q: %w", op, errOperationID)
		}
	}
	return func(key string) (buildEnd, error) {
		if i, ok := nodes[key]; ok {
			if p, landed := b.refs[BuildRef(b.op, key)]; landed {
				return buildEnd{placed: p, node: -1}, nil
			}
			return buildEnd{node: i}, nil
		}
		if p, ok := existing[key]; ok {
			return buildEnd{placed: p, node: -1}, nil
		}
		for _, op := range append([]string{b.op}, in.Use...) {
			if p, ok := b.refs[BuildRef(op, key)]; ok {
				return buildEnd{placed: p, node: -1}, nil
			}
		}
		return buildEnd{}, fmt.Errorf("edge end %q is no node of this call, no existing key and no key of the operations in use on this layer", key)
	}, nil
}

// edgeTypeName is the edge type an edge names, hierarchy by default.
func edgeTypeName(name string) string {
	if name == "" {
		return "hierarchy"
	}
	return name
}

// Validate checks the input's own shape; callers run it before a new layer is created for it.
func (in GraphBuildInput) Validate() error {
	if len(in.Nodes)+len(in.Edges) < 1 || len(in.Nodes) > MaxBuildNodes || len(in.Edges) > MaxBuildEdges || len(in.Use) > 20 {
		return fmt.Errorf("1..%d nodes and up to %d edges per call (an edges-only call joins existing nodes); split a larger graph into sections", MaxBuildNodes, MaxBuildEdges)
	}
	keys := map[string]bool{}
	for _, n := range in.Nodes {
		if !buildKeyPattern.MatchString(n.Key) || keys[n.Key] {
			return fmt.Errorf("node key %q: unique keys of 1..64 characters A-Z a-z 0-9 _ . - required", n.Key)
		}
		keys[n.Key] = true
		if n.FormID < 1 || !validTitle(n.Title, false) || len([]rune(n.Description)) > 10000 {
			return fmt.Errorf("node %q: formId (or a block kind) and a 1..1000 character title required", n.Key)
		}
		if _, err := objectData(n.Data, false); err != nil {
			return fmt.Errorf("node %q: %w", n.Key, err)
		}
	}
	pairs := map[BuildEdge]bool{}
	for _, e := range in.Edges {
		pair := BuildEdge{From: e.From, To: e.To, EdgeType: edgeTypeName(e.EdgeType)}
		if e.From == "" || e.To == "" || e.From == e.To || pairs[pair] || len([]rune(e.Name)) > 1000 || len(e.EdgeType) > 255 {
			return fmt.Errorf("edge %s -> %s: two different keys, once per edge type, required", e.From, e.To)
		}
		pairs[pair] = true
	}
	return nil
}
