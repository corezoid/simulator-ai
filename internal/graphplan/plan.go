// Package graphplan builds a deterministic, create-only native flowchart plan.
// It owns no credentials, execution journal, API writes or model calls.
package graphplan

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// Node uses a plan-local key, never a backend identity. BlockID comes from the public catalog.
type Node struct {
	Key     string `json:"key"`
	BlockID string `json:"blockId"`
	Title   string `json:"title"`
}

// Edge preserves directed semantic endpoints, including return links.
type Edge struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Target string `json:"target"`
	Name   string `json:"name,omitempty"`
}
type Input struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// PlannedNode contains public actor/create data and graph/layer/actors/manage geometry.
// FormID is the FlowchartBlock ROOT; ChildFormID is the data namespace source.
type PlannedNode struct {
	Key              string                        `json:"key"`
	Title            string                        `json:"title"`
	FormID           int64                         `json:"formId"`
	ChildFormID      int64                         `json:"childFormId"`
	CatalogUpdatedAt int64                         `json:"catalogUpdatedAt"`
	Data             json.RawMessage               `json:"data"`
	Position         publicapi.GraphPoint          `json:"position"`
	LayerSettings    publicapi.GraphNativeSettings `json:"layerSettings"`
}
type Plan struct {
	Algorithm   string        `json:"algorithm"`
	Nodes       []PlannedNode `json:"nodes"`
	Edges       []Edge        `json:"edges"`
	ReturnEdges []string      `json:"returnEdges"`
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// Validate runs before catalog reads; invalid plans never reach the network.
func Validate(p Input) error {
	if len(p.Nodes) < 1 || len(p.Nodes) > 100 || p.Edges == nil || len(p.Edges) > 100 {
		return errors.New("plan requires 1-100 nodes and 0-100 edges")
	}
	keys := map[string]bool{}
	for _, n := range p.Nodes {
		if !keyPattern.MatchString(n.Key) || keys[n.Key] || strings.TrimSpace(n.Title) == "" || utf8.RuneCountInString(n.Title) > 1000 || strings.TrimSpace(n.BlockID) == "" || utf8.RuneCountInString(n.BlockID) > 128 {
			return errors.New("invalid or duplicate plan node")
		}
		keys[n.Key] = true
	}
	edges := map[string]bool{}
	pairs := map[[2]string]bool{}
	for _, e := range p.Edges {
		pair := [2]string{e.Source, e.Target}
		if !keyPattern.MatchString(e.Key) || edges[e.Key] || !keys[e.Source] || !keys[e.Target] || e.Source == e.Target || pairs[pair] || utf8.RuneCountInString(e.Name) > 1000 {
			return errors.New("invalid, duplicate or unsupported self-loop plan edge")
		}
		edges[e.Key], pairs[pair] = true, true
	}
	return nil
}

// Build copies exact catalog values; missing or malformed kinds fail the whole plan.
// Geometry is versioned and deterministic for identical ordered input/catalog data.
func Build(p Input, catalog []publicapi.GraphBlock) (Plan, error) {
	if err := Validate(p); err != nil {
		return Plan{}, err
	}
	defs := map[string]publicapi.GraphBlock{}
	for _, d := range catalog {
		if _, exists := defs[d.BlockID]; exists {
			return Plan{}, errors.New("duplicate catalog definition")
		}
		defs[d.BlockID] = d
	}
	out := Plan{Algorithm: "native-flowchart-dag-v1", Nodes: make([]PlannedNode, 0, len(p.Nodes)), Edges: append([]Edge{}, p.Edges...), ReturnEdges: []string{}}
	for _, n := range p.Nodes {
		def, ok := defs[n.BlockID]
		if !ok {
			return Plan{}, errors.New("requested native block is unavailable")
		}
		data, settings, err := decodeDefinition(def)
		if err != nil {
			return Plan{}, err
		}
		out.Nodes = append(out.Nodes, PlannedNode{Key: n.Key, Title: n.Title, FormID: def.ParentID, ChildFormID: def.ID, CatalogUpdatedAt: def.UpdatedAt, Data: data, LayerSettings: settings})
	}
	layout(&out)
	return out, nil
}

// layout removes DFS back edges only for ranking. Semantic edges are untouched.
// The resulting DAG's longest paths place merges after every forward predecessor.
// Input order breaks ties; row packing uses actual native sizes and a 150-unit gap.
func layout(p *Plan) {
	index := map[string]int{}
	for i, n := range p.Nodes {
		index[n.Key] = i
	}
	adjacency := make([][]int, len(p.Nodes))
	for i, e := range p.Edges {
		adjacency[index[e.Source]] = append(adjacency[index[e.Source]], i)
	}
	state := make([]int, len(p.Nodes))
	back := make([]bool, len(p.Edges))
	var visit func(int)
	visit = func(n int) {
		state[n] = 1
		for _, i := range adjacency[n] {
			target := index[p.Edges[i].Target]
			if state[target] == 1 {
				back[i] = true
			} else if state[target] == 0 {
				visit(target)
			}
		}
		state[n] = 2
	}
	for i := range p.Nodes {
		if state[i] == 0 {
			visit(i)
		}
	}
	indegree := make([]int, len(p.Nodes))
	rank := make([]int, len(p.Nodes))
	for i, e := range p.Edges {
		if back[i] {
			p.ReturnEdges = append(p.ReturnEdges, e.Key)
		} else {
			indegree[index[e.Target]]++
		}
	}
	queue := []int{}
	for i := range p.Nodes {
		if indegree[i] == 0 {
			queue = append(queue, i)
		}
	}
	for head := 0; head < len(queue); head++ {
		n := queue[head]
		for _, i := range adjacency[n] {
			if back[i] {
				continue
			}
			target := index[p.Edges[i].Target]
			rank[target] = max(rank[target], rank[n]+1)
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	const gap = 150.0
	heights := make([]float64, len(p.Nodes))
	widths := make([]float64, len(p.Nodes))
	for i, n := range p.Nodes {
		heights[rank[i]] = max(heights[rank[i]], n.LayerSettings.Height)
		widths[rank[i]] += n.LayerSettings.Width + gap
	}
	y := 0.0
	for r, h := range heights {
		if h == 0 {
			continue
		}
		x := -(widths[r] - gap) / 2
		for i := range p.Nodes {
			if rank[i] == r {
				n := &p.Nodes[i]
				n.Position = publicapi.GraphPoint{X: x + n.LayerSettings.Width/2, Y: y + h/2}
				x += n.LayerSettings.Width + gap
			}
		}
		y += h + gap
	}
}
