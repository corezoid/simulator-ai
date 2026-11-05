package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestEdgeTypesLegacyNormalizedFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/parity/getEdgeTypes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ID             string `json:"id"`
		LegacyRevision string `json:"legacyRevision"`
		Cases          []struct {
			Name   string `json:"name"`
			Filter string `json:"filter"`
			Legacy struct {
				Data []map[string]json.RawMessage `json:"data"`
			} `json:"legacy"`
			Public   json.RawMessage `json:"public"`
			Expected struct {
				List []map[string]json.RawMessage `json:"list"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ID != "parity/getEdgeTypes" || fixture.LegacyRevision != "f11283155e3471091798abc640de7a0d0553c4a5" || len(fixture.Cases) != 5 {
		t.Fatal("legacy oracle drift")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q struct {
					ID string `json:"id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
					return
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, tc.Public)
			}))
			defer backend.Close()
			c, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "graph-read"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.ListGraphEdgeTypes(context.Background(), GraphEdgeTypesInput{Filter: tc.Filter})
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				List []map[string]json.RawMessage `json:"list"`
			}
			if err = json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.List, tc.Expected.List) || !reflect.DeepEqual(got.List, tc.Legacy.Data) {
				t.Fatalf("legacy projection/null differs: %s", b)
			}
		})
	}
}
