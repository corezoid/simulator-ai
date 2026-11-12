package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// A hole is a placeholder on a layer for an actor or a link that is expected but not known yet: the
// hole flag of an actor or edge row. Closing one is layer-level (sim-api actor/hole/merge,
// edge/hole/merge, ports of legacy holeActor.js mergeHoleActorReq and holeEdge.js): the closer takes
// the hole's placement, the hole itself stays and a revert puts it back. Creating an actor over a
// hole is not closing it.

// HoleCloseInput closes an actor hole on a layer with an existing actor or a new one.
type HoleCloseInput struct {
	LayerID     string `json:"layerId"`
	HoleActorID string `json:"holeActorId"`
	// ActorID is an existing closer; NewActor creates one first (exactly one of the two).
	ActorID  string        `json:"actorId,omitempty"`
	NewActor *HoleNewActor `json:"newActor,omitempty"`
	// HoleLaID closes only this placement of the hole; by default every one on the layer.
	HoleLaID   int64 `json:"holeLaId,omitempty"`
	ForceMerge bool  `json:"forceMerge,omitempty"`
}

// HoleNewActor is the closer to create. formId defaults to the hole's template and data starts
// from the hole's own data (its closing conditions), so a new closer meets them unless data
// overrides a key.
type HoleNewActor struct {
	Title       string          `json:"title"`
	FormID      int64           `json:"formId,omitempty"`
	Description string          `json:"description,omitempty"`
	Color       string          `json:"color,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// HoleClosed is actor/hole/merge's answer, with the closer when this call created it.
type HoleClosed struct {
	HoleActorID string       `json:"holeActorId"`
	ActorID     string       `json:"actorId"`
	Created     bool         `json:"created"`
	Changes     []HoleChange `json:"changes"`
	// Progress, Conditions and ConditionErrors are set by a forceMerge close.
	Progress        *HoleProgress    `json:"closingProgress,omitempty"`
	Conditions      []map[string]any `json:"conditions,omitempty"`
	ConditionErrors []string         `json:"conditionErrors,omitempty"`
}

// HoleChange is one hole placement swapped for the closer's (sim-api actorHoleChangeItem).
type HoleChange struct {
	LayerID     string `json:"layerId"`
	RemovedLaID int64  `json:"removedLaId"`
	AddedLaID   int64  `json:"addedLaId"`
}

// HoleProgress is matched of total non-empty hole conditions (sim-api e_hole.Progress).
type HoleProgress struct {
	Matched int `json:"matched"`
	Total   int `json:"total"`
}

// EdgeHoleChange is one hole edge placement swapped for the closer's (sim-api edgeHoleChangeItem).
type EdgeHoleChange struct {
	LayerID     string `json:"layerId"`
	RemovedLeID int64  `json:"removedLeId"`
	AddedLeID   int64  `json:"addedLeId"`
}

// EdgeHoleCloseInput closes an edge hole on a layer with an existing edge or a new one between the
// hole's own endpoints.
type EdgeHoleCloseInput struct {
	LayerID    string `json:"layerId"`
	HoleEdgeID string `json:"holeEdgeId"`
	EdgeID     string `json:"edgeId,omitempty"`
	// Name labels a new closing edge; without edgeId it is created with the hole's type.
	Name string `json:"name,omitempty"`
}

type EdgeHoleClosed struct {
	HoleEdgeID string           `json:"holeEdgeId"`
	EdgeID     string           `json:"edgeId"`
	Created    bool             `json:"created"`
	Changes    []EdgeHoleChange `json:"changes"`
}

type holeActorWire struct {
	ID     string          `json:"id"`
	FormID int64           `json:"formId"`
	Hole   bool            `json:"hole"`
	Data   json.RawMessage `json:"data"`
}

type holeMergeWire struct {
	HoleActorID     string           `json:"holeActorId"`
	Changes         []HoleChange     `json:"changes"`
	ClosingProgress *HoleProgress    `json:"closingProgress"`
	Conditions      []map[string]any `json:"conditions"`
	ConditionErrors []string         `json:"conditionErrors"`
}

// CloseHole closes the hole: with an existing actor in one actor/hole/merge, with a new one in an
// atomic batch of actor/create and the merge, so a refused close leaves no stray actor.
func (c *Client) CloseHole(ctx context.Context, in HoleCloseInput) (HoleClosed, error) {
	if e := consoleOnly(c); e != nil {
		return HoleClosed{}, e
	}
	if !validID(in.LayerID) || !validID(in.HoleActorID) {
		return HoleClosed{}, errors.New("layerId and holeActorId required")
	}
	if (in.ActorID == "") == (in.NewActor == nil) {
		return HoleClosed{}, errors.New("close with an existing actor (actorId) or a new one (newActor), not both")
	}
	ws := c.workspace()
	merge := map[string]any{"accId": ws, "layerId": in.LayerID, "holeActorId": in.HoleActorID}
	if in.HoleLaID > 0 {
		merge["holeLaId"] = in.HoleLaID
	}
	if in.ForceMerge {
		merge["forceMerge"] = true
	}
	if in.NewActor == nil {
		if !validID(in.ActorID) || in.ActorID == in.HoleActorID {
			return HoleClosed{}, errors.New("actorId: another, real actor")
		}
		merge["actorId"] = in.ActorID
		w, err := call[map[string]any, holeMergeWire](ctx, c, "actor/hole/merge", merge)
		if err != nil {
			return HoleClosed{}, fmt.Errorf("actor/hole/merge: %w", err)
		}
		return holeClosed(w, in.ActorID, false), nil
	}
	n := in.NewActor
	if !validTitle(n.Title, false) || len([]rune(n.Description)) > 10000 {
		return HoleClosed{}, errors.New("newActor: a 1..1000 character title required")
	}
	hole, err := call[map[string]any, holeActorWire](ctx, c, "actor/get", map[string]any{"accId": ws, "actorId": in.HoleActorID, "fields": []string{"id", "formId", "hole", "data"}})
	if err != nil {
		return HoleClosed{}, fmt.Errorf("actor/get hole: %w", err)
	}
	if !hole.Hole {
		return HoleClosed{}, errors.New("holeActorId is not a hole: layer_get marks hole placements with hole true")
	}
	data, err := closerData(hole.Data, n.Data)
	if err != nil {
		return HoleClosed{}, fmt.Errorf("newActor: %w", err)
	}
	formID := n.FormID
	if formID == 0 {
		formID = hole.FormID
	}
	color := n.Color
	if color == "" {
		color = RandomActorColor()
	}
	merge["actorId"] = "$0.id"
	results, err := c.atomicItems(ctx, []atomicItem{
		{Method: "actor/create", Params: actorCreateRequest{AccID: ws, FormID: formID, Title: n.Title, Description: n.Description, Color: color, Data: data, Fields: []string{"id"}}},
		{Method: "actor/hole/merge", Params: merge, UseRefs: true},
	})
	if err != nil {
		return HoleClosed{}, err
	}
	var created named
	var w holeMergeWire
	if json.Unmarshal(results[0], &created) != nil || json.Unmarshal(results[1], &w) != nil || fmt.Sprint(created.ID) == "" {
		return HoleClosed{}, errors.New("batch committed but returned no closing actor; read layer_get before retrying")
	}
	return holeClosed(w, fmt.Sprint(created.ID), true), nil
}

func holeClosed(w holeMergeWire, actorID string, created bool) HoleClosed {
	return HoleClosed{HoleActorID: w.HoleActorID, ActorID: actorID, Created: created, Changes: w.Changes,
		Progress: w.ClosingProgress, Conditions: w.Conditions, ConditionErrors: w.ConditionErrors}
}

func nullless(v json.RawMessage) json.RawMessage {
	if len(v) == 0 || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return nil
	}
	return v
}

// closerData is the hole's data overlaid with the caller's keys: the hole's non-empty fields are
// the conditions a closer is checked against.
func closerData(hole, given json.RawMessage) (json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(nullless(hole)) > 0 && json.Unmarshal(hole, &out) != nil {
		out = map[string]json.RawMessage{}
	}
	if len(given) > 0 {
		data, err := objectData(given, false)
		if err != nil {
			return nil, err
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, err
		}
		for k, v := range keys {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

type holeEdgeWire struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	EdgeTypeID int64  `json:"edgeTypeId"`
	Hole       bool   `json:"hole"`
}

type edgeHoleMergeWire struct {
	Changes []EdgeHoleChange `json:"changes"`
}

// CloseEdgeHole closes a placeholder link: the closer must join the hole's two actors in its
// direction, so without edgeId it is created from the hole's endpoints and type in the same atomic
// batch as edge/hole/merge (as the Graph Editor does).
func (c *Client) CloseEdgeHole(ctx context.Context, in EdgeHoleCloseInput) (EdgeHoleClosed, error) {
	if e := consoleOnly(c); e != nil {
		return EdgeHoleClosed{}, e
	}
	if !validID(in.LayerID) || !validID(in.HoleEdgeID) || len([]rune(in.Name)) > 1000 {
		return EdgeHoleClosed{}, errors.New("layerId and holeEdgeId required")
	}
	ws := c.workspace()
	merge := map[string]any{"accId": ws, "layerId": in.LayerID, "holeEdgeId": in.HoleEdgeID}
	if in.EdgeID != "" {
		if in.Name != "" {
			return EdgeHoleClosed{}, errors.New("name labels a new closing edge: send it without edgeId")
		}
		merge["edgeId"] = in.EdgeID
		w, err := call[map[string]any, edgeHoleMergeWire](ctx, c, "edge/hole/merge", merge)
		if err != nil {
			return EdgeHoleClosed{}, fmt.Errorf("edge/hole/merge: %w", err)
		}
		return EdgeHoleClosed{HoleEdgeID: in.HoleEdgeID, EdgeID: in.EdgeID, Changes: w.Changes}, nil
	}
	hole, err := call[map[string]any, holeEdgeWire](ctx, c, "edge/get", map[string]any{"accId": ws, "edgeId": in.HoleEdgeID})
	if err != nil {
		return EdgeHoleClosed{}, fmt.Errorf("edge/get hole: %w", err)
	}
	if !hole.Hole || hole.Source == "" || hole.Target == "" || hole.EdgeTypeID < 1 {
		return EdgeHoleClosed{}, errors.New("holeEdgeId is not a hole edge: layer_get marks hole edge placements with hole true")
	}
	merge["edgeId"] = "$0.id"
	results, err := c.atomicItems(ctx, []atomicItem{
		// forceDirection: the closer must keep the hole's orientation; a hierarchy create would
		// otherwise reuse a reverse edge, which edge/hole/merge refuses.
		{Method: "edge/create", Params: edgeCreateRequest{AccID: ws, Source: hole.Source, Target: hole.Target, EdgeTypeID: hole.EdgeTypeID, Name: in.Name, ForceDirection: true}},
		{Method: "edge/hole/merge", Params: merge, UseRefs: true},
	})
	if err != nil {
		return EdgeHoleClosed{}, err
	}
	var created named
	var w edgeHoleMergeWire
	if json.Unmarshal(results[0], &created) != nil || json.Unmarshal(results[1], &w) != nil || fmt.Sprint(created.ID) == "" {
		return EdgeHoleClosed{}, errors.New("batch committed but returned no closing edge; read layer_get before retrying")
	}
	return EdgeHoleClosed{HoleEdgeID: in.HoleEdgeID, EdgeID: fmt.Sprint(created.ID), Created: true, Changes: w.Changes}, nil
}
