package publicapi

import (
	"encoding/json"
	"errors"
	"strings"
)

func validateGraphActor(r Actor) error {
	if r.ID == nil || strings.TrimSpace(*r.ID) == "" || r.FormID == nil || *r.FormID < 1 {
		return errors.New("invalid created actor identity")
	}
	return nil
}
func validateGraphEdge(r GraphCreatedEdge) error {
	if strings.TrimSpace(r.ID) == "" || r.EdgeTypeID < 1 || strings.TrimSpace(r.Source) == "" || strings.TrimSpace(r.Target) == "" {
		return errors.New("invalid created edge identity")
	}
	return nil
}
func sameEdgeEnds(p GraphEdgeInput, r GraphCreatedEdge) bool {
	return (r.Source == p.Source && r.Target == p.Target) || (!p.ForceDirection && r.Source == p.Target && r.Target == p.Source)
}
func graphStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// UnmarshalJSON refuses invented zero coordinates when the server omits x or y.
func (p *GraphPoint) UnmarshalJSON(raw []byte) error {
	var wire struct {
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	if wire.X == nil || wire.Y == nil {
		return errors.New("incomplete graph position")
	}
	p.X, p.Y = *wire.X, *wire.Y
	return nil
}
func validateGraphPlacements(r GraphCreatedPlacements) error {
	if r.Nodes == nil || r.Edges == nil || len(r.Nodes)+len(r.Edges) < 1 || len(r.Nodes)+len(r.Edges) > 100 {
		return errors.New("invalid placement result size")
	}
	seen := map[int64]bool{}
	for _, n := range r.Nodes {
		if n.LaID < 1 || strings.TrimSpace(n.ActorID) == "" || n.Position == nil || seen[n.LaID] {
			return errors.New("invalid node placement identity")
		}
		seen[n.LaID] = true
	}
	for _, e := range r.Edges {
		if e.LaID < 1 || strings.TrimSpace(e.EdgeID) == "" || e.LaIDSource == nil || *e.LaIDSource < 1 || e.LaIDTarget == nil || *e.LaIDTarget < 1 || seen[e.LaID] {
			return errors.New("invalid edge placement identity")
		}
		seen[e.LaID] = true
	}
	return nil
}

// Compare multisets, not response order: repeated actors on a layer are intentional.
// Positions are historical and may be snapped by the backend; never pretend input==stored.
func bindGraphPlacements(p GraphPlacementsInput, r GraphCreatedPlacements) error {
	nodes := map[string]int{}
	type edgeKey struct {
		id             string
		source, target int64
	}
	edges := map[edgeKey]int{}
	for _, item := range p.Items {
		if item.Data.Type == "node" {
			nodes[item.Data.ID]++
		} else {
			edges[edgeKey{item.Data.ID, *item.Data.LaIDSource, *item.Data.LaIDTarget}]++
		}
	}
	for _, n := range r.Nodes {
		nodes[n.ActorID]--
	}
	for _, e := range r.Edges {
		edges[edgeKey{e.EdgeID, *e.LaIDSource, *e.LaIDTarget}]--
	}
	for _, n := range nodes {
		if n != 0 {
			return errors.New("created node placement binding mismatch")
		}
	}
	for _, n := range edges {
		if n != 0 {
			return errors.New("created edge placement binding mismatch")
		}
	}
	return nil
}
