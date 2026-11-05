package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/drawing"
)

type drawBackend struct {
	steps   []string
	batch   map[string]any
	png     []byte
	defForm string
}

func (b *drawBackend) serve(t *testing.T) *Client {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/rest/upload" {
			_ = r.ParseMultipartForm(4 << 20)
			f, h, _ := r.FormFile("file")
			b.png, _ = io.ReadAll(f)
			b.steps = append(b.steps, "upload "+h.Filename+" actorId="+r.FormValue("actorId"))
			fmt.Fprint(w, `{"id":9,"title":"drawing.png","type":"image/png","size":10,"fileName":"2026/11/drawing-k1.png"}`)
			return
		}
		var q struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		b.steps = append(b.steps, q.Method)
		switch q.Method {
		case "actor/get":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":"L","data":{%s}}}`, q.ID, b.defForm)
		case "batch":
			b.batch = q.Params
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"isAtomic":true,"list":[{"id":"0","result":{"id":"p1"}},{"id":"1","result":{"nodes":[{"laId":41,"actorId":"p1"}],"edges":[]}},{"id":"2","result":{}}]}}`, q.ID)
		}
	}))
	t.Cleanup(api.Close)
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	return c
}

// The strokes become what a confirmed Pencil session makes: the PNG uploaded, then one atomic batch
// creates the picture actor of the layer's default form, places it at the ink's centre with the
// image cue, and attaches the file.
func TestDrawOnLayer(t *testing.T) {
	b := &drawBackend{defForm: `"defForm":[{"title":"Notes","value":12,"type":"form"}]`}
	c := b.serve(t)
	out, err := c.DrawOnLayer(context.Background(), LayerDrawInput{LayerID: "L", Cell: "B, 2", Title: "Sketch",
		Strokes: []drawing.Stroke{{Points: []drawing.Point{{X: -20, Y: 0}, {X: 20, Y: 0}}, Width: 4, ArrowEnd: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(b.steps, ",") != "actor/get,upload drawing.png actorId=,batch" {
		t.Fatal(b.steps)
	}
	// Cell "B, 2" is centred on (50, 50); the line is drawn about it.
	if out.ActorID != "p1" || out.LaID != 41 || out.Picture != "2026/11/drawing-k1.png" || out.Position != (GraphPoint{X: 50, Y: 50}) || out.Cell != "B, 2" {
		t.Fatalf("%+v", out)
	}
	img, err := png.Decode(bytes.NewReader(b.png))
	if err != nil || img.Bounds().Dx() != int(out.Width*drawing.Scale+0.999) {
		t.Fatal(err, img.Bounds(), out.Width)
	}
	sent, _ := json.Marshal(b.batch)
	for _, want := range []string{
		`"method":"actor/create","params":{"accId":"ws","contextLayerId":"L","data":{},"fields":["id"],"formId":12,"picture":"2026/11/drawing-k1.png","pictureObject":{"height":`,
		`"img":"2026/11/drawing-k1.png","type":"image"`, `"title":"Sketch"`,
		`"method":"graph/layer/actors/manage","params":{"accId":"ws","actorId":"L","items":[{"action":"create","data":{"id":"$0.id","layerSettings":{"height":`,
		fmt.Sprintf(`"image":{"naturalHeight":%d,"naturalWidth":%d}`, img.Bounds().Dy(), img.Bounds().Dx()),
		`"position":{"x":50,"y":50},"type":"node"}}]},"useRefs":true`,
		`"method":"attachment/attach","params":{"accId":"ws","actorId":"$0.id","attachmentIds":[9]},"useRefs":true`,
	} {
		if !strings.Contains(string(sent), want) {
			t.Errorf("%s lacks %s", sent, want)
		}
	}
}

func TestDrawOnLayerNeedsAForm(t *testing.T) {
	b := &drawBackend{}
	c := b.serve(t)
	strokes := []drawing.Stroke{{Points: []drawing.Point{{X: 0, Y: 0}, {X: 10, Y: 10}}}}
	if _, err := c.DrawOnLayer(context.Background(), LayerDrawInput{LayerID: "L", Strokes: strokes}); err == nil || !strings.Contains(err.Error(), "pass formId") || strings.Join(b.steps, ",") != "actor/get" {
		t.Fatal(err, b.steps)
	}
	b.steps = nil
	if _, err := c.DrawOnLayer(context.Background(), LayerDrawInput{LayerID: "L", FormID: 3, Strokes: strokes}); err != nil || strings.Join(b.steps, ",") != "upload drawing.png actorId=,batch" {
		t.Fatal(err, b.steps)
	}
	// A stroke the pen cannot draw reaches nothing.
	b.steps = nil
	if _, err := c.DrawOnLayer(context.Background(), LayerDrawInput{LayerID: "L", FormID: 3, Strokes: []drawing.Stroke{{Points: strokes[0].Points, Width: 30}}}); err == nil || len(b.steps) != 0 {
		t.Fatal(err, b.steps)
	}
}
