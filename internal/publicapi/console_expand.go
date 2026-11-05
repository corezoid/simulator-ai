package publicapi

import (
	"context"
	"errors"
	"fmt"
)

// NodeExpandInput opens a node placement in place, closes it, or resizes an open one.
type NodeExpandInput struct {
	LayerID string `json:"layerId"`
	LaID    int64  `json:"laId"`
	// Expand false closes the node; absent or true opens it.
	Expand *bool `json:"expand,omitempty"`
	// Width and Height are the open box in cells (both or neither); on an open node they resize it.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// NodeExpanded reports the node's open state after the call.
type NodeExpanded struct {
	LaID     int64  `json:"laId"`
	ActorID  string `json:"actorId"`
	Expanded bool   `json:"expanded"`
	// Changed is false when the node already was as asked.
	Changed    bool   `json:"changed"`
	ExpandType string `json:"expandType,omitempty"`
	// Area names the open box by its corner cells, "C, 3 - O, 11".
	Area string `json:"area,omitempty"`
	// Pushed is whether this open state makes room (the layer's anisotropy when it opened).
	Pushed bool `json:"pushed,omitempty"`
	// Moved is how many placements the make-room moved.
	Moved int `json:"moved,omitempty"`
	// Covers names placements the open box lies over (no make-room: anisotropy off).
	Covers string `json:"covers,omitempty"`
}

type elementsListRequest struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	Nodes   []int64  `json:"nodes"`
	Fields  []string `json:"fields,omitempty"`
}

type actorSettingsPatch struct {
	AccID         string    `json:"accId"`
	ActorID       string    `json:"actorId"`
	LaActorID     int64     `json:"laActorId"`
	LayerSettings openPatch `json:"layerSettings"`
}

type offsetRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
	OffsetDirective
}

// layerNode reads one node placement of the layer by laId.
func (c *Client) layerNode(ctx context.Context, layerID string, laID int64) (layerNodeFull, error) {
	r, err := call[elementsListRequest, struct {
		Nodes []layerNodeFull `json:"nodes"`
	}](ctx, c, "graph/layer/elements/list", elementsListRequest{AccID: c.config.Workspace, ActorID: layerID, Nodes: []int64{laID},
		Fields: []string{"id", "title", "formId", "pictureObject"}})
	if err != nil {
		return layerNodeFull{}, err
	}
	if len(r.Nodes) != 1 || r.Nodes[0].LaID != laID {
		return layerNodeFull{}, fmt.Errorf("no node placement %d on layer %s (layer_get lists them)", laID, layerID)
	}
	return r.Nodes[0], nil
}

// nestedForms reads the system templates whose actors open as a chart or a Smart Form.
func (c *Client) nestedForms(ctx context.Context) (nestedSystemForms, error) {
	scripts, err := c.systemFormID(ctx, "Scripts")
	if err != nil {
		return nestedSystemForms{}, err
	}
	dashboards, err := c.systemFormID(ctx, "Dashboards")
	return nestedSystemForms{Scripts: scripts, Dashboards: dashboards}, err
}

func (in NodeExpandInput) frame() (*NestedFrame, error) {
	switch {
	case in.Width == 0 && in.Height == 0:
		return nil, nil
	case in.Expand != nil && !*in.Expand:
		return nil, errors.New("width and height size an open node: leave them out to close it")
	case in.Width < nestedMinWidth || in.Height < nestedMinHeight || in.Width > nestedMaxCells || in.Height > nestedMaxCells:
		return nil, fmt.Errorf("width %d..%d and height %d..%d cells, both or neither", nestedMinWidth, nestedMaxCells, nestedMinHeight, nestedMaxCells)
	}
	f := centredFrame(in.Width, in.Height)
	return &f, nil
}

// ExpandNode opens a node in place as the Graph Editor does (its own layer, a chart, a Smart Form),
// closes it, or resizes an open one; on a layer with anisotropy the neighbours make room, and a close
// returns them. One atomic batch: actor_settings/save, then actors/offset.
func (c *Client) ExpandNode(ctx context.Context, in NodeExpandInput) (NodeExpanded, error) {
	if e := consoleOnly(c); e != nil {
		return NodeExpanded{}, e
	}
	if !validID(in.LayerID) || in.LaID < 1 {
		return NodeExpanded{}, errors.New("layerId and laId required")
	}
	frame, err := in.frame()
	if err != nil {
		return NodeExpanded{}, err
	}
	node, err := c.layerNode(ctx, in.LayerID, in.LaID)
	if err != nil {
		return NodeExpanded{}, err
	}
	if node.AccessDenied {
		return NodeExpanded{}, fmt.Errorf("placement %d: no access to its actor", in.LaID)
	}
	if k := node.kind(); k != "actor" {
		return NodeExpanded{}, fmt.Errorf("placement %d is a %s: only an actor opens in place", in.LaID, k)
	}
	if _, _, ok := node.center(); !ok {
		return NodeExpanded{}, fmt.Errorf("placement %d has no position", in.LaID)
	}
	room, err := c.readLayerRoom(ctx, in.LayerID)
	if err != nil {
		return NodeExpanded{}, err
	}
	n := node.asOpenNode()
	out := NodeExpanded{LaID: in.LaID, ActorID: node.ID, Expanded: n.Settings.Expand}
	var patch openPatch
	var dir *OffsetDirective
	switch open := in.Expand == nil || *in.Expand; {
	case open && !n.Settings.Expand:
		forms, err := c.nestedForms(ctx)
		if err != nil {
			return NodeExpanded{}, err
		}
		var formID int64
		if node.Actor != nil && node.Actor.FormID != nil {
			formID = *node.Actor.FormID
		}
		kind := nestedKindOf(formID, n.Settings, forms)
		if kind == nestedCard {
			return NodeExpanded{}, fmt.Errorf("placement %d shows an actor card, which the Graph Editor does not open in place", in.LaID)
		}
		patch, dir = expandPlan(n, kind, frame, room.Anisotropy)
	case open && frame != nil && (!n.Settings.frameStored(*frame)):
		patch, dir = resizePlan(n, *frame)
	case !open && n.Settings.Expand:
		patch, dir = collapsePlan(n)
	default:
		return describeOpen(out, n, room), nil
	}
	items := []atomicItem{{Method: "graph/layer/actor_settings/save",
		Params: actorSettingsPatch{AccID: c.config.Workspace, ActorID: in.LayerID, LaActorID: in.LaID, LayerSettings: patch}}}
	if dir != nil {
		items = append(items, atomicItem{Method: "graph/layer/actors/offset", Params: offsetRequest{AccID: c.config.Workspace, ActorID: in.LayerID, OffsetDirective: *dir}})
	}
	if _, err := c.atomicItems(ctx, items); err != nil {
		return NodeExpanded{}, err
	}
	out.Changed = true
	if dir != nil {
		out.Moved = room.moved(dir, in.LaID)
	}
	n.Settings = n.Settings.with(patch)
	out.Expanded = n.Settings.Expand
	return describeOpen(out, n, room.after(dir, in.LaID)), nil
}

// describeOpen fills the open state's type, box and what the box lies over.
func describeOpen(out NodeExpanded, n openNode, room layerRoom) NodeExpanded {
	if !n.Settings.Expand {
		return out
	}
	box := n.box()
	out.ExpandType, out.Area, out.Pushed = n.Settings.ExpandType, areaCells(box), n.Settings.pushed()
	if covered := room.conflicts([]graphRect{box}, out.LaID); len(covered) > 0 {
		out.Covers = cellsOf(covered)
	}
	return out
}

// frameStored reports whether the stored frame already is f.
func (s openSettings) frameStored(f NestedFrame) bool {
	stored, ok := s.storedFrame()
	return ok && stored == f
}

// with is the bag after the patch is merged into it.
func (s openSettings) with(p openPatch) openSettings {
	if p.Expand != nil {
		s.Expand = *p.Expand
	}
	if p.ExpandType != "" {
		s.ExpandType = p.ExpandType
	}
	if p.Profile != nil {
		s.Profile = *p.Profile
	}
	if p.ExpandPushed != nil {
		s.ExpandPushed = p.ExpandPushed
	}
	if f := p.NestedFrame; f != nil {
		l, t, w, h := float64(f.Left), float64(f.Top), float64(f.Width), float64(f.Height)
		s.NestedFrame = &nestedFrameWire{Left: &l, Top: &t, Width: &w, Height: &h}
	}
	return s
}
