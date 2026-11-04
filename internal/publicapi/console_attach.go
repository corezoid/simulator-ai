package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// What an actor shows in its view besides reactions and files, as ui-kit's ActorView stores it: one
// card of another actor (actors.card_actor_id), one Smart Form (app_id), and any number of form
// cards, each an actor of an account template pinned to it by a hierarchy edge (pinned true).

type ActorAttachInput struct {
	ActorID string `json:"actorId"`
	Kind    string `json:"kind"` // actor, smartForm or form
	// TargetID is the card actor (actor), the appId (smartForm) or an existing form actor (form).
	TargetID string `json:"targetId,omitempty"`
	// FormID, Title and Data create a new form actor of that template to pin (form only).
	FormID int64           `json:"formId,omitempty"`
	Title  string          `json:"title,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type ActorAttached struct {
	ActorID  string `json:"actorId"`
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
	EdgeID   string `json:"edgeId,omitempty"`
}

// AttachToActor sets the actor's card or Smart Form (replacing the one it had), or pins a form card.
func (c *Client) AttachToActor(ctx context.Context, in ActorAttachInput) (ActorAttached, error) {
	if e := consoleOnly(c); e != nil {
		return ActorAttached{}, e
	}
	if !validID(in.ActorID) {
		return ActorAttached{}, errors.New("actorId required")
	}
	newForm := in.FormID != 0 || in.Title != "" || len(in.Data) > 0
	if in.Kind != "form" && newForm {
		return ActorAttached{}, errors.New("formId, title and data create a form card: kind form only")
	}
	ws := c.config.Workspace
	switch in.Kind {
	case "actor", "smartForm":
		if !validID(in.TargetID) || in.TargetID == in.ActorID {
			return ActorAttached{}, errors.New("targetId required: the card actor id, or the Smart Form's appId")
		}
		field := map[string]string{"actor": "cardActorId", "smartForm": "appId"}[in.Kind]
		if _, err := call[map[string]any, named](ctx, c, "actor/update", map[string]any{"accId": ws, "actorId": in.ActorID, field: in.TargetID, "fields": []string{"id"}}); err != nil {
			return ActorAttached{}, fmt.Errorf("actor/update: %w", err)
		}
		return ActorAttached{ActorID: in.ActorID, Kind: in.Kind, TargetID: in.TargetID}, nil
	case "form":
	default:
		return ActorAttached{}, errors.New("kind is actor, smartForm or form")
	}
	if newForm == (in.TargetID != "") {
		return ActorAttached{}, errors.New("a form card is a new actor (formId, title, data) or an existing one (targetId)")
	}
	hierarchy, err := c.HierarchyEdgeType(ctx)
	if err != nil {
		return ActorAttached{}, err
	}
	edge := map[string]any{"accId": ws, "source": in.ActorID, "target": in.TargetID, "edgeTypeId": hierarchy, "forceDirection": true, "pinned": true}
	if !newForm {
		if !validID(in.TargetID) || in.TargetID == in.ActorID {
			return ActorAttached{}, errors.New("targetId: another actor's id")
		}
		e, err := call[map[string]any, named](ctx, c, "edge/create", edge)
		if err != nil {
			return ActorAttached{}, fmt.Errorf("edge/create: %w", err)
		}
		return ActorAttached{ActorID: in.ActorID, Kind: "form", TargetID: in.TargetID, EdgeID: fmt.Sprint(e.ID)}, nil
	}
	if in.FormID < 1 || !validTitle(in.Title, false) {
		return ActorAttached{}, errors.New("a positive formId and a 1..1000 character title required")
	}
	data, err := objectData(in.Data, false)
	if err != nil {
		return ActorAttached{}, err
	}
	edge["target"] = "$0.id"
	results, err := c.atomicItems(ctx, []atomicItem{
		{Method: "actor/create", Params: map[string]any{"accId": ws, "formId": in.FormID, "title": in.Title, "data": data, "color": RandomActorColor(), "fields": []string{"id"}}},
		{Method: "edge/create", Params: edge, UseRefs: true},
	})
	if err != nil {
		return ActorAttached{}, err
	}
	var actor, created named
	if json.Unmarshal(results[0], &actor) != nil || json.Unmarshal(results[1], &created) != nil || fmt.Sprint(actor.ID) == "" {
		return ActorAttached{}, errors.New("batch committed but returned no form actor or edge; read graph_get before retrying")
	}
	return ActorAttached{ActorID: in.ActorID, Kind: "form", TargetID: fmt.Sprint(actor.ID), EdgeID: fmt.Sprint(created.ID)}, nil
}
