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

const edgeTypeFixture = `{"id":1,"name":"custom","accId":"workspace","userId":null,"isSystem":false,"createdAt":0,"isTree":false}`

func TestEdgeTypeProjectionAndFilterBoundary(t *testing.T) {
	for _, tc := range []struct{ filter, want string }{
		{"", edgeTypeFixture},
		{"id,name", `{"id":1,"name":"custom"}`},
		{"id, name", `{"id":1}`},
		{"unknown", `{}`},
		{"isSystem,isTree,createdAt,userId", `{"isSystem":false,"isTree":false,"createdAt":0,"userId":null}`},
		{"id,id", `{"id":1}`},
	} {
		filter := tc.filter
		t.Run(filter, func(t *testing.T) {
			calls := 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var q struct {
					ID     string `json:"id"`
					Method string `json:"method"`
					Params struct {
						AccID    string `json:"accId"`
						Limit    int64  `json:"limit"`
						Offset   int64  `json:"offset"`
						IsSystem *bool  `json:"isSystem"`
						Filter   string `json:"filter"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
				}
				if q.Method != "edge_type/list" || q.Params.AccID != "workspace" || q.Params.IsSystem == nil || *q.Params.IsSystem || q.Params.Filter != "" || q.Params.Limit != 50 {
					t.Error("host/filter boundary escaped")
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"list":[%s]}}`, q.ID, edgeTypeFixture)
			}))
			defer backend.Close()
			c, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "graph-read"})
			if err != nil {
				t.Fatal(err)
			}
			custom := false
			got, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{IsSystem: &custom, Filter: filter})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				List []map[string]json.RawMessage `json:"list"`
			}
			if err = json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.List) != 1 || calls != 1 {
				t.Fatal("projection changed membership")
			}
			var want map[string]json.RawMessage
			if err = json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if len(result.List[0]) != len(want) {
				t.Fatalf("unexpected projection: %s", raw)
			}
			for key, value := range want {
				if string(result.List[0][key]) != string(value) {
					t.Fatalf("lost selected zero/null/field %s: %s", key, raw)
				}
			}
		})
	}
}

func TestEdgeTypeRejectsMalformedIdentityOrderAndFilter(t *testing.T) {
	for _, row := range []string{
		strings.Replace(edgeTypeFixture, `"accId":"workspace"`, `"accId":"other"`, 1),
		strings.Replace(edgeTypeFixture, `"createdAt":0,`, "", 1),
		strings.Replace(edgeTypeFixture, `"createdAt":0`, `"createdAt":null`, 1),
		strings.Replace(edgeTypeFixture, `"id":1`, `"id":0`, 1),
		strings.Replace(edgeTypeFixture, `"isTree":false`, `"isTree":null`, 1),
		strings.Replace(edgeTypeFixture, `"isTree":false`, `"isTree":"false"`, 1),
		strings.Replace(edgeTypeFixture, `"userId":null`, `"userId":"1"`, 1),
		strings.Replace(edgeTypeFixture, `"id":1`, `"id":1.5`, 1),
		strings.Replace(edgeTypeFixture, `"name":"custom"`, `"name":" "`, 1),
		strings.Replace(edgeTypeFixture, `"isSystem":false`, `"isSystem":true`, 1),
		edgeTypeFixture + "," + edgeTypeFixture,
		strings.Replace(edgeTypeFixture, `"id":1`, `"id":2`, 1) + "," + edgeTypeFixture,
	} {
		response := strings.ReplaceAll(`{"list":[`+row+`]}`, `"workspace"`, `"workspace-a"`)
		calls := 0
		c := graphClient(t, &response, &calls)
		custom := false
		result, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{IsSystem: &custom, Filter: "unknown"})
		if err == nil || result.List != nil {
			t.Fatalf("malformed row leaked through empty projection: %+v", result)
		}
	}
}

func TestEdgeTypesFreshReadDoesNotCacheOrRetryDenial(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		calls++
		if calls == 1 {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"list":[%s]}}`, q.ID, edgeTypeFixture)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":1,"message":"denied","data":{"reason":"notAuthorized"}}}`, q.ID)
	}))
	defer backend.Close()
	c, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "graph-read"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{}); err != nil {
		t.Fatal(err)
	}
	result, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{})
	if err == nil || result.List != nil || calls != 2 {
		t.Fatalf("denied fresh read returned cached rows or retried: %+v, calls %d", result, calls)
	}
}

func TestEdgeTypesRejectsMissingNullOrOversizedPage(t *testing.T) {
	for _, response := range []string{`{}`, `{"list":null}`, `{"list":[` + edgeTypeFixture + `,` + strings.Replace(edgeTypeFixture, `"id":1`, `"id":2`, 1) + `]}`} {
		response = strings.ReplaceAll(response, `"workspace"`, `"workspace-a"`)
		calls := 0
		c := graphClient(t, &response, &calls)
		limit := int64(1)
		result, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{Limit: &limit})
		if err == nil || result.List != nil {
			t.Fatalf("invalid page accepted: %+v", result)
		}
	}
}
