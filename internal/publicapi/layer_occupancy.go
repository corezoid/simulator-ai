package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// What a layer's placements occupy, so the plugin neither puts a node over another nor lets an open
// node's box swallow its neighbours. The Graph Editor refuses a new actor on a cell another one holds
// (placementCommands cellOccupied), and sim-api refuses a move onto one, but graph/layer/actors/manage
// does not check: the plugin checks before it places. It counts, as the editor does, only what lives
// on the grid: a plain actor, a hole or a text node holds its cell, an open node its whole box, a
// flowchart block its own box. Pictures, drawings and markup (polygons) are marks over the grid and
// hold nothing.

// maxRoomPages bounds the read of a layer's placements (1000 a page).
const maxRoomPages = 10

// placementSettingsWire is the part of a stored placement bag the plugin reads.
type placementSettingsWire struct {
	openSettings
	BlockID    string          `json:"blockId"`
	SizeGrade  string          `json:"sizeGrade"`
	Width      *float64        `json:"width"`
	Height     *float64        `json:"height"`
	IsTextNode bool            `json:"isTextNode"`
	Image      json.RawMessage `json:"image"`
	Drawing    json.RawMessage `json:"drawing"`
}

// layerNodeFull is a node row of graph/layer/actors/list with the geometry occupancy needs.
type layerNodeFull struct {
	LaID          int64                  `json:"laId"`
	ID            string                 `json:"id"`
	Position      *Position              `json:"position"`
	AccessDenied  bool                   `json:"accessDenied"`
	Hole          bool                   `json:"hole"`
	Polygon       [][]float64            `json:"polygon"`
	AreaPicture   json.RawMessage        `json:"areaPicture"`
	LayerSettings *placementSettingsWire `json:"layerSettings"`
	Actor         *struct {
		FormID        *int64             `json:"formId"`
		Title         *string            `json:"title"`
		PictureObject *pictureObjectWire `json:"pictureObject"`
	} `json:"actor"`
}

// pictureObjectWire is an actor's picture object. Only a legacy bitmap, a type image or napkin
// with a source, makes the placement a picture (normalize.ts legacyImageObject); an avatar does not.
type pictureObjectWire struct {
	Type string `json:"type"`
	Img  string `json:"img"`
}

func (p *pictureObjectWire) bitmap() bool {
	return p != nil && (p.Type == "image" || p.Type == "napkin") && p.Img != ""
}

func present(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

// settings is the stored bag, empty when none.
func (n layerNodeFull) settings() placementSettingsWire {
	if n.LayerSettings == nil {
		return placementSettingsWire{}
	}
	return *n.LayerSettings
}

// kind classifies the placement as the Graph Editor does (normalize.ts placementKindOf).
func (n layerNodeFull) kind() string {
	s := n.settings()
	switch {
	case n.Hole:
		return "hole"
	case s.BlockID != "":
		return "flowchart"
	case present(s.Drawing):
		return "drawing"
	case present(s.Image) || present(n.AreaPicture) || n.Actor != nil && n.Actor.PictureObject.bitmap():
		return "image"
	case len(n.Polygon) > 2:
		return "markup"
	case s.IsTextNode:
		return "text"
	}
	return "actor"
}

// center is the stored position, false when the row has none.
func (n layerNodeFull) center() (float64, float64, bool) {
	if n.Position == nil || n.Position.X == nil || n.Position.Y == nil {
		return 0, 0, false
	}
	return *n.Position.X, *n.Position.Y, true
}

// asOpenNode is the placement as opening reads it.
func (n layerNodeFull) asOpenNode() openNode {
	x, y, _ := n.center()
	s := n.settings()
	return openNode{X: x, Y: y, SizeGrade: s.SizeGrade, Settings: s.openSettings}
}

// cellOf is the box of the grid cell a centre falls in (cellIndexAt).
func cellOf(x, y float64) graphRect {
	return cellBox(jsRound(x/gridCell)*gridCell, jsRound(y/gridCell)*gridCell)
}

// footprint is what the placement holds on the grid, false for a mark that holds nothing.
func (n layerNodeFull) footprint() (graphRect, bool) {
	x, y, ok := n.center()
	if !ok {
		return graphRect{}, false
	}
	s := n.settings()
	switch n.kind() {
	case "actor":
		if s.Expand {
			return n.asOpenNode().box().union(cellOf(x, y)), true
		}
		return cellOf(x, y), true
	case "hole", "text":
		return cellOf(x, y), true
	case "flowchart":
		w, h := gridCell, gridCell
		if s.Width != nil && *s.Width > 0 {
			w = *s.Width
		}
		if s.Height != nil && *s.Height > 0 {
			h = *s.Height
		}
		return graphRect{X: x - w/2, Y: y - h/2, W: w, H: h}, true
	}
	return graphRect{}, false
}

// occupant is one placement that holds part of the grid.
type occupant struct {
	LaID int64
	X, Y float64
	Box  graphRect
}

// layerRoom is a layer as placing reads it: its anisotropy and what its placements hold.
type layerRoom struct {
	Anisotropy bool
	Occupants  []occupant
	// Partial is set when the layer has more placements than were read.
	Partial bool
}

type layerAnisotropyWire struct {
	ID   string `json:"id"`
	Data struct {
		Anisotropy json.RawMessage `json:"anisotropy"`
	} `json:"data"`
}

// layerAnisotropy reads the layer's anisotropy setting: its data.anisotropy, on when absent or not
// a boolean (LAYER_VIEW_SETTINGS_DEFAULTS, legacy LAYER_SETTINGS_DEFAULTS).
func (c *Client) layerAnisotropy(ctx context.Context, layerID string) (bool, error) {
	layer, err := call[map[string]any, layerAnisotropyWire](ctx, c, "actor/get",
		map[string]any{"accId": c.config.Workspace, "actorId": layerID, "fields": []string{"id", "data.anisotropy"}})
	if err != nil {
		return false, fmt.Errorf("actor/get layer: %w", err)
	}
	return string(bytes.TrimSpace(layer.Data.Anisotropy)) != "false", nil
}

// layerNodesPage reads one page of a layer's node placements with the fields occupancy needs.
func (c *Client) layerNodesPage(ctx context.Context, layerID string, limit, offset int64, fields []string) ([]layerNodeFull, error) {
	page, err := call[layerPageRequest, struct {
		List []layerNodeFull `json:"list"`
	}](ctx, c, "graph/layer/actors/list", layerPageRequest{AccID: c.config.Workspace, ActorID: layerID, Limit: limit, Offset: offset, Fields: fields})
	if err != nil {
		return nil, err
	}
	for _, n := range page.List {
		if n.LaID < 1 || strings.TrimSpace(n.ID) == "" {
			return nil, errors.New("invalid node placement identity")
		}
	}
	return page.List, nil
}

// readLayerRoom reads the layer's anisotropy and every placement's footprint (at most
// maxRoomPages pages; past that Partial is set and the server's own checks decide the rest).
func (c *Client) readLayerRoom(ctx context.Context, layerID string) (layerRoom, error) {
	aniso, err := c.layerAnisotropy(ctx, layerID)
	if err != nil {
		return layerRoom{}, err
	}
	room := layerRoom{Anisotropy: aniso}
	const limit = 1000
	for page := int64(0); page < maxRoomPages; page++ {
		nodes, err := c.layerNodesPage(ctx, layerID, limit, page*limit, []string{"id", "formId", "pictureObject"})
		if err != nil {
			return layerRoom{}, fmt.Errorf("graph/layer/actors/list: %w", err)
		}
		for _, n := range nodes {
			if box, ok := n.footprint(); ok {
				x, y, _ := n.center()
				room.Occupants = append(room.Occupants, occupant{LaID: n.LaID, X: x, Y: y, Box: box})
			}
		}
		if len(nodes) < limit {
			return room, nil
		}
	}
	room.Partial = true
	return room, nil
}

// conflicts lists the occupants (but skip) whose box overlaps any of the boxes.
func (r layerRoom) conflicts(boxes []graphRect, skip int64) []occupant {
	var out []occupant
	for _, o := range r.Occupants {
		if o.LaID == skip {
			continue
		}
		for _, b := range boxes {
			if o.Box.overlaps(b) {
				out = append(out, o)
				break
			}
		}
	}
	return out
}

// after is the layer once the directive has moved its placements (skip, the grown node, stays).
func (r layerRoom) after(d *OffsetDirective, skip int64) layerRoom {
	if d == nil {
		return r
	}
	out := layerRoom{Anisotropy: r.Anisotropy, Partial: r.Partial, Occupants: make([]occupant, len(r.Occupants))}
	for i, o := range r.Occupants {
		if o.LaID != skip {
			if x, y, moved := d.moves(o.X, o.Y); moved {
				o.Box.X, o.Box.Y = o.Box.X+x-o.X, o.Box.Y+y-o.Y
				o.X, o.Y = x, y
			}
		}
		out.Occupants[i] = o
	}
	return out
}

// movedTo counts the placements whose position differs in the later layer (after's order is the
// receiver's), each once however many directives moved it.
func (r layerRoom) movedTo(later layerRoom) int {
	n := 0
	for i, o := range r.Occupants {
		if i < len(later.Occupants) && (later.Occupants[i].X != o.X || later.Occupants[i].Y != o.Y) {
			n++
		}
	}
	return n
}

// cellsOf names the occupants' cells as the rulers do, sorted, at most 10.
func cellsOf(occ []occupant) string {
	seen := map[string]bool{}
	var cells []string
	for _, o := range occ {
		cell := fmt.Sprintf("%s (laId %d)", CellOf(GraphPoint{X: o.X, Y: o.Y}), o.LaID)
		if !seen[cell] {
			seen[cell] = true
			cells = append(cells, cell)
		}
	}
	sort.Strings(cells)
	if len(cells) > 10 {
		cells = append(cells[:10], fmt.Sprintf("and %d more", len(cells)-10))
	}
	return strings.Join(cells, ", ")
}

// areaCells names a box by its corner cells, "C, 3 - M, 11".
func areaCells(b graphRect) string {
	first := CellOf(GraphPoint{X: b.X + gridCell/2, Y: b.Y + gridCell/2})
	last := CellOf(GraphPoint{X: b.right() - gridCell/2, Y: b.bottom() - gridCell/2})
	return first + " - " + last
}

// finite reports whether every coordinate is a usable number.
func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e9 {
			return false
		}
	}
	return true
}
