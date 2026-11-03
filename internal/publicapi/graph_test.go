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

const nativeBlock = `{"id":21,"parentId":20,"title":"Process","blockId":"process","updatedAt":1,"form":{"sections":[]}}`

func graphClient(t *testing.T, response *string, calls *int) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic-graph" {
			t.Error("unexpected transport")
		}
		q := readSmart(t, r)
		if q.Method != "form/flowchart-block/search" && q.Method != "form/system/get" && q.Method != "edge_type/list" {
			t.Error("nonpublic or write method")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, *response)
	}))
	t.Cleanup(server.Close)
	c, err := New(Config{Origin: server.URL, Workspace: "workspace-a", APIKey: "synthetic-graph", Profile: "graph-read"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGraphCatalogBindingAndMalformedResults(t *testing.T) {
	response := `{"list":[` + nativeBlock + `],"hasMore":false}`
	calls := 0
	c := graphClient(t, &response, &calls)
	input := GraphCatalogInput{BlockIDs: []string{"process", "process"}}
	r, err := c.SearchGraphBlocks(context.Background(), input)
	if err != nil || len(r.List) != 1 || r.List[0].ID != 21 {
		t.Fatalf("%+v %v", r, err)
	}
	for _, bad := range []string{
		`{"list":[],"hasMore":true}`, `{"list":[]}`, `{"list":null,"hasMore":false}`,
		`{"list":[` + nativeBlock + `,` + nativeBlock + `],"hasMore":false}`,
		strings.Replace(response, `"process"`, `"other"`, 1),
		strings.Replace(response, `"parentId":20`, `"parentId":0`, 1),
		strings.Replace(response, `"form":{"sections":[]}`, `"form":null`, 1),
	} {
		response = bad
		if _, err := c.SearchGraphBlocks(context.Background(), input); err == nil {
			t.Fatalf("accepted malformed catalog %s", bad)
		}
	}
	before := calls
	if _, err := c.SearchGraphBlocks(context.Background(), GraphCatalogInput{BlockIDs: []string{}}); err == nil {
		t.Fatal("empty explicit ids accepted")
	}
	for _, id := range []string{"", strings.Repeat("a", 129)} {
		if _, err := c.SearchGraphBlocks(context.Background(), GraphCatalogInput{BlockIDs: []string{id}}); err == nil {
			t.Fatal("invalid block id reached API")
		}
	}
	c.config.Profile = "read"
	if _, err := c.SearchGraphBlocks(context.Background(), GraphCatalogInput{}); err == nil {
		t.Fatal("profile fence bypassed")
	}
	if calls != before {
		t.Fatal("rejected input reached transport")
	}
}

func TestGraphSystemAndEdgeTypeProjections(t *testing.T) {
	response := `{"id":20,"title":"Layers","type":"system","form":{"sections":[]},"editToken":"auth-canary","ownerId":42}`
	calls := 0
	c := graphClient(t, &response, &calls)
	r, err := c.GetGraphSystemForm(context.Background(), GraphSystemFormInput{Title: "Layers"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "auth-canary") || strings.Contains(string(raw), "ownerId") {
		t.Fatal("unprojected fields leaked")
	}
	response = strings.Replace(response, `"Layers"`, `"Graphs"`, 1)
	if _, err := c.GetGraphSystemForm(context.Background(), GraphSystemFormInput{Title: "Layers"}); err == nil {
		t.Fatal("wrong system form accepted")
	}
	response = `{"list":[{"id":4,"name":"interaction","isTree":false,"isSystem":true,"userId":42}]}`
	edges, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{})
	if err != nil || len(edges.List) != 1 || *edges.List[0].IsTree {
		t.Fatalf("%+v %v", edges, err)
	}
	response = strings.Replace(response, `"isTree":false,`, "", 1)
	if _, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{}); err == nil {
		t.Fatal("missing tree constraint accepted")
	}
}
