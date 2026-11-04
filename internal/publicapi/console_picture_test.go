package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The image is uploaded attached to nothing, then its fileName becomes the actor's picture.
func TestUploadActorPicture(t *testing.T) {
	var steps []string
	var update map[string]any
	fail := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/rest/upload" {
			_ = r.ParseMultipartForm(1 << 20)
			steps = append(steps, "upload actorId="+r.FormValue("actorId"))
			fmt.Fprint(w, `{"id":7,"title":"face.png","type":"image/png","size":3,"fileName":"2026/11/face-k1.png"}`)
			return
		}
		var q struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		steps, update = append(steps, q.Method), q.Params
		if fail {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":1,"message":"no modify","data":{"reason":"accessDenied"}}}`, q.ID)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":"a1"}}`, q.ID)
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	p, err := c.UploadActorPicture(context.Background(), ActorPictureInput{ActorID: "a1", FileName: "face.png", Content: "AQID"})
	if err != nil || p != (ActorPicture{ActorID: "a1", Picture: "2026/11/face-k1.png", FileID: 7}) {
		t.Fatal(p, err)
	}
	if strings.Join(steps, ",") != "upload actorId=,actor/update" || update["picture"] != "2026/11/face-k1.png" || update["actorId"] != "a1" {
		t.Fatal(steps, update)
	}
	fail = true
	if _, err = c.UploadActorPicture(context.Background(), ActorPictureInput{ActorID: "a1", FileName: "face.png", Content: "AQID"}); err == nil || !strings.Contains(err.Error(), "uploaded as 2026/11/face-k1.png, file 7, and is not set") {
		t.Fatal("a refused update must name the uploaded file:", err)
	}
	steps = nil
	if _, err = c.UploadActorPicture(context.Background(), ActorPictureInput{ActorID: "a1", FileName: "notes.txt", Content: "AQID"}); err == nil || len(steps) != 0 {
		t.Fatal("a non-image reached the API:", steps, err)
	}
}
