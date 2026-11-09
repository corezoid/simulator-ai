// Package simfake is an in-memory stand-in for the public sim-api JSON-RPC a graph is built through:
// actors with refs unique per template, edges with natural keys, a layer's node and edge placements,
// and batches (atomic, with "$<index>.<path>" result references) under the server's own limits. It
// lets tests build thousands of nodes, lose an answer mid-build and read back what really landed.
package simfake

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Server limits, as sim-api enforces them.
const (
	MaxBatchItems = 100
	MaxBodyBytes  = 1 << 20
	MaxLayerItems = 500
)

// Fault is what Hook does to a request.
type Fault int

const (
	// Answer handles the request normally.
	Answer Fault = iota
	// Drop cuts the connection before anything is applied.
	Drop
	// Lose applies the request and cuts the connection: the caller cannot know it landed.
	Lose
)

// Actor is a stored actor.
type Actor struct {
	ID, Title, Ref, Color string
	FormID                int64
	Recent                bool
}

// Node is a node placement of a layer.
type Node struct {
	LaID          int64
	Layer, Actor  string
	X, Y          float64
	LayerSettings json.RawMessage
	AreaPicture   json.RawMessage
}

// Edge is an edge; Placement an edge drawn on a layer.
type Edge struct {
	ID, Source, Target, Name string
	TypeID                   int64
}
type Placement struct {
	LaID     int64
	Layer    string
	Edge     string
	Src, Dst int64
}

// Call is one HTTP request the fake received.
type Call struct {
	Method string
	Items  int // list length of a batch, items of a layer write
	Bytes  int
}

type state struct {
	Actors     map[string]*Actor
	Refs       map[string]string // "<formId>|<ref>" -> actor id; kept after a delete, as the trash keeps it
	Nodes      []Node
	Edges      map[string]*Edge
	EdgeKeys   map[string]string
	Placements []Placement
	Seq        int64
}

func (s state) clone() state {
	out := state{Actors: make(map[string]*Actor, len(s.Actors)), Refs: make(map[string]string, len(s.Refs)), Edges: make(map[string]*Edge, len(s.Edges)),
		EdgeKeys: make(map[string]string, len(s.EdgeKeys)), Nodes: append([]Node(nil), s.Nodes...), Placements: append([]Placement(nil), s.Placements...), Seq: s.Seq}
	for k, v := range s.Actors {
		out.Actors[k] = v
	}
	for k, v := range s.Refs {
		out.Refs[k] = v
	}
	for k, v := range s.Edges {
		out.Edges[k] = v
	}
	for k, v := range s.EdgeKeys {
		out.EdgeKeys[k] = v
	}
	return out
}

// Server is the fake backend. Read its state through the methods; change the knobs with Set.
type Server struct {
	*httptest.Server
	mu sync.Mutex
	state
	calls []Call
	// Hook decides each request's fate by its ordinal (from 0) and method; nil answers all.
	Hook func(n int, method string) Fault
	// BadForm refuses actor/create of this template, as a template whose data does not validate.
	BadForm int64
	// Ghosts placements lead every node page and are dropped from the answer, as sim-api drops
	// deleted actors after paging: a page comes back short without being the last.
	Ghosts int
	// Blocks is the result of form/flowchart-block/search.
	Blocks string
}

// BlockCatalog is a one-kind flowchart catalog: "process", 200 x 100.
const BlockCatalog = `{"list":[{"id":6,"parentId":5,"title":"Process","blockId":"process","collection":"basic","updatedAt":10,"form":{"sections":[{"content":[{"id":"blockId","value":"process"},{"id":"collection","value":"basic"},{"id":"view","value":"{\"shape\":{\"type\":\"path\",\"d\":\"M0 0H1\"},\"size\":{\"w\":200,\"h\":100},\"textFrame\":{\"x\":0,\"y\":0,\"w\":200,\"h\":100}}"}]}]}}],"hasMore":false}`

// New starts a fake backend; close it with Close.
func New() *Server {
	s := &Server{Blocks: BlockCatalog, state: state{Actors: map[string]*Actor{}, Refs: map[string]string{}, Edges: map[string]*Edge{}, EdgeKeys: map[string]string{}}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Reason string `json:"reason"`
	} `json:"data"`
}

func fail(reason, message string) *rpcError {
	e := &rpcError{Code: -32602, Message: message}
	e.Data.Reason = reason
	return e
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if len(body) > MaxBodyBytes {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var q struct {
		ID     string          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	_ = json.Unmarshal(body, &q)
	s.mu.Lock()
	n := len(s.calls)
	s.calls = append(s.calls, Call{Method: q.Method, Bytes: len(body), Items: itemsOf(q.Params)})
	fault := Answer
	if s.Hook != nil {
		fault = s.Hook(n, q.Method)
	}
	if fault == Drop {
		s.mu.Unlock()
		panic(http.ErrAbortHandler)
	}
	result, err := s.run(q.Method, q.Params)
	s.mu.Unlock()
	if fault == Lose {
		panic(http.ErrAbortHandler)
	}
	out := map[string]any{"jsonrpc": "2.0", "id": q.ID}
	if err != nil {
		out["error"] = err
	} else {
		out["result"] = result
	}
	_ = json.NewEncoder(w).Encode(out)
}

func itemsOf(params json.RawMessage) int {
	var p struct {
		List  []json.RawMessage `json:"list"`
		Items []json.RawMessage `json:"items"`
	}
	_ = json.Unmarshal(params, &p)
	return len(p.List) + len(p.Items)
}

// Set changes the knobs (Hook, BadForm, Ghosts, Blocks) between requests.
func (s *Server) Set(change func(*Server)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(s)
}

// Calls is every request received so far.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// Count is how many requests of the method arrived.
func (s *Server) Count(method string) int {
	n := 0
	for _, c := range s.Calls() {
		if c.Method == method {
			n++
		}
	}
	return n
}

// Snapshot is the stored actors, a layer's node placements and its drawn edges.
func (s *Server) Snapshot(layer string) (actors []Actor, nodes []Node, edges []Placement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.Actors {
		actors = append(actors, *a)
	}
	for _, n := range s.Nodes {
		if n.Layer == layer {
			nodes = append(nodes, n)
		}
	}
	for _, p := range s.Placements {
		if p.Layer == layer {
			edges = append(edges, p)
		}
	}
	return actors, nodes, edges
}

type batchItem struct {
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	UseRefs bool            `json:"useRefs"`
}

func (s *Server) batch(params json.RawMessage) (any, *rpcError) {
	var p struct {
		IsAtomic bool        `json:"isAtomic"`
		List     []batchItem `json:"list"`
	}
	_ = json.Unmarshal(params, &p)
	if len(p.List) > MaxBatchItems {
		return nil, fail("notAllowed", fmt.Sprintf("maximum %d allowed in list", MaxBatchItems))
	}
	before := s.state.clone()
	type item struct {
		ID     string    `json:"id"`
		Result any       `json:"result,omitempty"`
		Error  *rpcError `json:"error,omitempty"`
	}
	out := struct {
		IsAtomic   bool   `json:"isAtomic"`
		RolledBack bool   `json:"rolledBack,omitempty"`
		List       []item `json:"list"`
	}{IsAtomic: p.IsAtomic, List: make([]item, len(p.List))}
	results := make([]any, len(p.List))
	failed := false
	for i, it := range p.List {
		args := it.Params
		var err *rpcError
		if it.UseRefs {
			args, err = resolveRefs(args, results[:i])
		}
		if err == nil {
			results[i], err = s.run(it.Method, args)
		}
		out.List[i] = item{ID: it.ID, Result: results[i], Error: err}
		failed = failed || err != nil
	}
	if p.IsAtomic && failed {
		s.state, out.RolledBack = before, true
	}
	return out, nil
}

var refPattern = regexp.MustCompile(`^\$(\d+)\.([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*)$`)

// resolveRefs substitutes "$<index>.<path>" strings from earlier results, keeping their JSON type.
func resolveRefs(params json.RawMessage, done []any) (json.RawMessage, *rpcError) {
	var tree any
	_ = json.Unmarshal(params, &tree)
	var bad *rpcError
	var walk func(any) any
	walk = func(v any) any {
		switch v := v.(type) {
		case string:
			m := refPattern.FindStringSubmatch(v)
			if m == nil {
				return v
			}
			i, _ := strconv.Atoi(m[1])
			if i >= len(done) || done[i] == nil {
				bad = fail("invalidParams", v+" references an item that has not run or failed")
				return v
			}
			raw, _ := json.Marshal(done[i])
			var cur any
			_ = json.Unmarshal(raw, &cur)
			for _, key := range strings.Split(m[2], ".") {
				switch c := cur.(type) {
				case map[string]any:
					cur = c[key]
				case []any:
					k, _ := strconv.Atoi(key)
					if k >= len(c) {
						bad = fail("invalidParams", v+": index out of range")
						return v
					}
					cur = c[k]
				}
			}
			return cur
		case map[string]any:
			for k, child := range v {
				v[k] = walk(child)
			}
		case []any:
			for k, child := range v {
				v[k] = walk(child)
			}
		}
		return v
	}
	tree = walk(tree)
	out, _ := json.Marshal(tree)
	return out, bad
}

func cellOf(v float64) float64 { return math.Round(v/50) * 50 }
