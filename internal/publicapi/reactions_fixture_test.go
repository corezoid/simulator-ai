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

func TestReactionStatsLegacyNormalizedFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/parity/getReactionsStats.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ID             string `json:"id"`
		LegacyRevision string `json:"legacyRevision"`
		Cases          []struct {
			Name   string `json:"name"`
			Legacy struct {
				Data ReactionStats `json:"data"`
			} `json:"legacy"`
			Expected ReactionStats `json:"expected"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ID != "parity/getReactionsStats" || fixture.LegacyRevision != "f11283155e3471091798abc640de7a0d0553c4a5" || len(fixture.Cases) != 4 {
		t.Fatal("legacy oracle drift")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q request[reactionStatsRequest]
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
				}
				b, err := json.Marshal(tc.Legacy.Data)
				if err != nil {
					t.Error(err)
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, b)
			}))
			defer backend.Close()
			client, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "reactions-read"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetReactionStats(context.Background(), "actor")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tc.Expected, got) {
				t.Fatalf("legacy-normalized counters differ: %+v", got)
			}
		})
	}
}
