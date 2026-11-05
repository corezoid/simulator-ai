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

const elementResult = `{"nodes":[{"laId":11,"id":"a","position":{"x":1,"y":2},"layerSettings":{"blockId":"process","width":200,"height":100,"shape":{"type":"path","d":"M0 0 Z"},"textFrame":{"x":5,"y":5,"w":190,"h":90},"html":"secret-canary"},"actor":{"id":"a","title":"A","formId":21,"data":{"secret":"secret-canary"},"ownerId":42}}],"edges":[{"laId":11,"id":"e","laIdSource":11,"laIdTarget":12,"edge":{"id":"e","edgeTypeId":3,"source":"a","target":"b","name":"Так","userId":42}}]}`

func TestExactGraphReadbackProjectionAndFailures(t *testing.T) {
	response := elementResult
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic" {
			t.Error("transport changed")
		}
		q := readSmart(t, r)
		if q.Method != "graph/layer/elements/list" || string(q.Params["accId"]) != `"workspace-a"` || string(q.Params["fields"]) != `["id","title","description","formId","status","ref"]` {
			t.Error("unbound projection", q.Method)
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, response)
	}))
	defer server.Close()
	c, err := New(Config{Origin: server.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "graph-create"})
	if err != nil {
		t.Fatal(err)
	}
	input := GraphElementsInput{ActorID: "layer", Nodes: []int64{11}, Edges: []int64{11}}
	result, err := c.GetGraphElements(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "canary") || strings.Contains(string(raw), "ownerId") || strings.Contains(string(raw), "userId") || !strings.Contains(string(raw), "Так") {
		t.Fatal("invalid projection", string(raw))
	}
	for _, bad := range []string{
		`{"nodes":[],"edges":[]}`, `{"nodes":null,"edges":[]}`,
		strings.Replace(elementResult, `"position":{"x":1,"y":2}`, `"position":{}`, 1),
		strings.Replace(elementResult, `"textFrame":{"x":5,"y":5,"w":190,"h":90}`, `"textFrame":{}`, 1),
		strings.Replace(elementResult, `"textFrame":{"x":5,"y":5,"w":190,"h":90}`, `"textFrame":{"w":10}`, 1),
		strings.Replace(elementResult, `"textFrame":{"x":5,"y":5,"w":190,"h":90}`, `"textFrame":{"x":null,"y":5,"w":190,"h":90}`, 1),
		strings.Replace(elementResult, `"laId":11`, `"laId":99`, 1),
		strings.Replace(elementResult, `"actor":{`, `"accessDenied":true,"actor":{`, 1),
		strings.Replace(elementResult, `"actor":{"id":"a"`, `"actor":{"id":"other"`, 1),
		strings.Replace(elementResult, `"source":"a"`, `"source":"other"`, 1),
		strings.Replace(elementResult, `"laIdSource":11`, `"laIdSource":0`, 1),
	} {
		response = bad
		if _, err := c.GetGraphElements(context.Background(), input); err == nil {
			t.Fatal("malformed selection accepted")
		}
	}
	before := calls
	for _, bad := range []GraphElementsInput{{ActorID: "layer", Nodes: []int64{}, Edges: []int64{}}, {ActorID: "layer", Nodes: []int64{0}, Edges: []int64{}}, {ActorID: "layer", Nodes: []int64{11, 11}, Edges: []int64{}}, {ActorID: "layer", Nodes: nil, Edges: []int64{11}}} {
		if _, err := c.GetGraphElements(context.Background(), bad); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	c.config.Profile = "read"
	if _, err := c.GetGraphElements(context.Background(), input); err == nil {
		t.Fatal("profile fence missing")
	}
	if calls != before {
		t.Fatal("invalid input reached API")
	}
}
func TestExactGraphReadbackKeepsRepeatedActorsAndEdgeOnlySelection(t *testing.T) {
	response := `{"nodes":[{"laId":11,"id":"a","actor":{"id":"a"}},{"laId":12,"id":"a","actor":{"id":"a"}}],"edges":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := readSmart(t, r)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, response)
	}))
	defer server.Close()
	c, err := New(Config{Origin: server.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "graph-read"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetGraphElements(context.Background(), GraphElementsInput{ActorID: "layer", Nodes: []int64{11, 12}, Edges: []int64{}}); err != nil || len(got.Nodes) != 2 {
		t.Fatal("repeated actor lost", err)
	}
	response = `{"nodes":[],"edges":[{"laId":11,"id":"e","edge":{"id":"e","edgeTypeId":3,"source":"a","target":"b"}}]}`
	if got, err := c.GetGraphElements(context.Background(), GraphElementsInput{ActorID: "layer", Nodes: []int64{}, Edges: []int64{11}}); err != nil || len(got.Edges) != 1 {
		t.Fatal("edge-only selection rejected", err)
	}
}

func TestExactGraphPreservesLayerLabelAndStyleOverrides(t *testing.T) {
	response := strings.Replace(elementResult, `"laIdTarget":12,`, `"laIdTarget":12,"layerSettings":{"name":"Ні","curveStyle":"straight","linkedActorId":"secret-canary"},`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := readSmart(t, r)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, response)
	}))
	defer server.Close()
	c, err := New(Config{Origin: server.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "graph-read"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Ні", ""} {
		if name == "" {
			response = strings.Replace(response, `"name":"Ні"`, `"name":""`, 1)
		}
		result, err := c.GetGraphElements(context.Background(), GraphElementsInput{ActorID: "layer", Nodes: []int64{11}, Edges: []int64{11}})
		if err != nil {
			t.Fatal(err)
		}
		settings := result.Edges[0].LayerSettings
		if settings == nil || settings.Name == nil || *settings.Name != name || settings.CurveStyle == nil || *settings.CurveStyle != "straight" || *result.Edges[0].Edge.Name != "Так" {
			t.Fatal("layer override or clear was lost")
		}
		raw, _ := json.Marshal(result.Edges)
		if strings.Contains(string(raw), "canary") {
			t.Fatal("unprojected settings leaked")
		}
	}
}
