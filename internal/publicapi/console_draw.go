package publicapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/corezoid/simulator-ai/internal/drawing"
)

// A Graph Editor Pencil session is confirmed as one picture actor (simulator-ui-kit graph-layer
// app/layerEditor/useDrawingSession.ts → commands/textCommands.ts placePictureFrom): the flattened
// PNG uploaded and attached to a new actor of the layer's default form, that actor's picture and
// legacy pictureObject {img, width, height, type: "image"} naming the file, and one placement
// centred on the ink with layerSettings {width, height, image {naturalWidth, naturalHeight}} (the
// image cue that makes it a free, gridless picture). layer_draw builds the same from strokes.

type LayerDrawInput struct {
	LayerID string           `json:"layerId"`
	Strokes []drawing.Stroke `json:"strokes"`
	// Cell, when set, is the origin of every coordinate: the centre of that ruler cell.
	Cell string `json:"cell,omitempty"`
	// FormID overrides the layer's default form (data.defForm); Title names the actor.
	FormID int64  `json:"formId,omitempty"`
	Title  string `json:"title,omitempty"`
}

type LayerDrawn struct {
	ActorID  string     `json:"actorId"`
	LaID     int64      `json:"laId"`
	Picture  string     `json:"picture"`
	Position GraphPoint `json:"position"`
	Cell     string     `json:"cell"`
	Width    float64    `json:"width"`
	Height   float64    `json:"height"`
}

type layerDefForm struct {
	Data struct {
		DefForm []struct {
			Value json.Number `json:"value"`
		} `json:"defForm"`
	} `json:"data"`
}

// DrawOnLayer rasterizes the strokes and places the picture on the layer as Pencil does.
func (c *Client) DrawOnLayer(ctx context.Context, in LayerDrawInput) (LayerDrawn, error) {
	if e := consoleOnly(c); e != nil {
		return LayerDrawn{}, e
	}
	if !validID(in.LayerID) || !validTitle(in.Title, true) {
		return LayerDrawn{}, errors.New("layerId required (title at most 1000 characters)")
	}
	strokes := in.Strokes
	if in.Cell != "" {
		origin, err := CellCenter(in.Cell)
		if err != nil {
			return LayerDrawn{}, err
		}
		strokes = shifted(strokes, origin)
	}
	pic, err := drawing.Render(strokes)
	if err != nil {
		return LayerDrawn{}, err
	}
	if len(pic.PNG) > MaxUploadBytes {
		return LayerDrawn{}, fmt.Errorf("the drawing's PNG is %d KiB, over %d KiB: draw it smaller or in parts", len(pic.PNG)>>10, MaxUploadBytes>>10)
	}
	ws := c.config.Workspace
	formID := in.FormID
	if formID == 0 {
		if formID, err = c.layerDefaultForm(ctx, in.LayerID); err != nil {
			return LayerDrawn{}, err
		}
	}
	// Uploaded first and unattached: a refused create below leaves only an unattached file, never an
	// empty picture on the layer. The same batch then attaches it, as the editor's attached upload.
	f, err := c.UploadAttachment(ctx, AttachmentUploadInput{FileName: "drawing.png", Content: base64.StdEncoding.EncodeToString(pic.PNG), ContentType: "image/png"})
	if err != nil {
		return LayerDrawn{}, fmt.Errorf("upload: %w", err)
	}
	// sim-api validates width/height with gte=1, so a sub-unit box is floored (store/commands.ts placeImage).
	w, h := math.Max(pic.Box.W, 1), math.Max(pic.Box.H, 1)
	create := map[string]any{"accId": ws, "formId": formID, "data": map[string]any{}, "contextLayerId": in.LayerID, "picture": f.FileName,
		"pictureObject": map[string]any{"img": f.FileName, "width": pic.Box.W, "height": pic.Box.H, "type": "image"}, "fields": []string{"id"}}
	if in.Title != "" {
		create["title"] = in.Title
	}
	position := GraphPoint{X: pic.Box.X, Y: pic.Box.Y}
	place := map[string]any{"accId": ws, "actorId": in.LayerID, "items": []any{map[string]any{"action": "create", "data": map[string]any{
		"id": "$0.id", "type": "node", "position": position,
		"layerSettings": map[string]any{"width": w, "height": h, "image": map[string]any{"naturalWidth": pic.Pixels.X, "naturalHeight": pic.Pixels.Y}},
	}}}}
	results, err := c.atomicItems(ctx, []atomicItem{
		{Method: "actor/create", Params: create},
		{Method: "graph/layer/actors/manage", Params: place, UseRefs: true},
		{Method: "attachment/attach", Params: map[string]any{"accId": ws, "actorId": "$0.id", "attachmentIds": []int64{f.ID}}, UseRefs: true},
	})
	if err != nil {
		return LayerDrawn{}, fmt.Errorf("%w (the PNG stays uploaded, unattached, as %s)", err, f.FileName)
	}
	var actor named
	var placed GraphCreatedPlacements
	_ = json.Unmarshal(results[1], &placed)
	if json.Unmarshal(results[0], &actor) != nil || fmt.Sprint(actor.ID) == "" {
		return LayerDrawn{}, errors.New("batch committed but returned no picture actor; read layer_get before retrying")
	}
	out := LayerDrawn{ActorID: fmt.Sprint(actor.ID), Picture: f.FileName, Position: position, Cell: CellOf(position), Width: w, Height: h}
	if len(placed.Nodes) == 1 {
		out.LaID = placed.Nodes[0].LaID
	}
	return out, nil
}

// layerDefaultForm is the template a new canvas object of the layer gets: its data.defForm. The
// Graph Editor falls back to the user's own default template (form/default/get), which is not a
// public method, so without defForm the model is asked for formId.
func (c *Client) layerDefaultForm(ctx context.Context, layerID string) (int64, error) {
	layer, err := call[map[string]any, layerDefForm](ctx, c, "actor/get", map[string]any{"accId": c.config.Workspace, "actorId": layerID, "fields": []string{"id", "data.defForm"}})
	if err != nil {
		return 0, fmt.Errorf("actor/get layer: %w", err)
	}
	if len(layer.Data.DefForm) > 0 {
		if id, err := layer.Data.DefForm[0].Value.Int64(); err == nil && id > 0 {
			return id, nil
		}
	}
	return 0, errors.New("the layer has no default form (its settings' Default actors form): pass formId, a template from forms_list the user agrees to, or set the layer's default form first")
}

// shifted moves every coordinate by the origin, leaving the caller's strokes untouched.
func shifted(strokes []drawing.Stroke, origin GraphPoint) []drawing.Stroke {
	out := make([]drawing.Stroke, len(strokes))
	for i, s := range strokes {
		s.Points = append([]drawing.Point(nil), s.Points...)
		for j := range s.Points {
			s.Points[j].X += origin.X
			s.Points[j].Y += origin.Y
		}
		if s.Center != nil {
			c := drawing.Point{X: s.Center.X + origin.X, Y: s.Center.Y + origin.Y}
			s.Center = &c
		}
		out[i] = s
	}
	return out
}
