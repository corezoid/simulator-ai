package publicapi

import (
	"context"
	"errors"
	"math"
)

// ViewportInput moves the caller's own open view of a layer (public graph/viewport/command): the
// person whose token the run carries sees the Graph Editor pan, zoom, fit, focus an actor, switch
// 2D/3D or open another layer. Cell is the ruler address ("C, 7"); sim-api takes the cell index.
type ViewportInput struct {
	LayerID       string   `json:"layerId"`
	Action        string   `json:"action"` // pan, zoom, fit, focus, mode2d, mode3d, openLayer
	Cell          string   `json:"cell,omitempty"`
	X             *float64 `json:"x,omitempty"`
	Y             *float64 `json:"y,omitempty"`
	Zoom          *float64 `json:"zoom,omitempty"`
	ActorID       string   `json:"actorId,omitempty"`
	TargetLayerID string   `json:"targetLayerId,omitempty"`
	TransitionMs  int      `json:"transitionMs,omitempty"`
}

// ViewportMoved says how many of the caller's tabs have the layer open: 0 means nothing moved.
type ViewportMoved struct {
	CommandID string `json:"commandId"`
	OpenTabs  int    `json:"openTabs"`
}

// gridIndex is sim-api's cell: the index of a ruler cell, centred at (50·x, 50·y).
type gridIndex struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type viewportRequest struct {
	AccID string `json:"accId"`
	ViewportInput
	Cell *gridIndex `json:"cell,omitempty"`
}

// MoveViewport sends the command; which fields each action takes is sim-api's to check, and its
// refusal names the field.
func (c *Client) MoveViewport(ctx context.Context, in ViewportInput) (ViewportMoved, error) {
	if e := consoleOnly(c); e != nil {
		return ViewportMoved{}, e
	}
	if !validID(in.LayerID) {
		return ViewportMoved{}, errors.New("layerId required: the current layer of the request's context")
	}
	req := viewportRequest{AccID: c.config.Workspace, ViewportInput: in}
	if in.Cell != "" {
		at, err := CellCenter(in.Cell)
		if err != nil {
			return ViewportMoved{}, err
		}
		req.Cell = &gridIndex{X: int(math.Round(at.X / gridCell)), Y: int(math.Round(at.Y / gridCell))}
	}
	return call[viewportRequest, ViewportMoved](ctx, c, "graph/viewport/command", req)
}
