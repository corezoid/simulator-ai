package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReactionStatsPreservesCountersAndRejectsMalformedResults(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		invalid      bool
	}{
		{"counts", `{"comment":2,"rating":1,"view":0}`, false},
		{"empty", `{}`, false},
		{"dynamic type", `{"customReaction":4}`, false},
		{"null result", `null`, true},
		{"null count", `{"comment":null}`, true},
		{"negative count", `{"comment":-1}`, true},
		{"fraction", `{"comment":1.5}`, true},
		{"numeric string", `{"comment":"2"}`, true},
		{"nested data", `{"comment":{"count":2}}`, true},
		{"array", `[]`, true},
		{"empty key", `{"":1}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var q request[reactionStatsRequest]
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
				}
				if q.Method != "reaction/stats/get" || q.Params.AccID != "workspace" || q.Params.RootActorID != "actor" || r.Header.Get("Authorization") != "ApiKey fixture" || r.URL.Path != "/api/2.0/rpc" {
					t.Error("request boundary mismatch")
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, tc.result)
			}))
			defer backend.Close()
			client, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "reactions-read"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.GetReactionStats(context.Background(), "actor")
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v err=%v", tc.invalid, err)
			}
			if tc.invalid && result != nil {
				t.Fatal("partial malformed result leaked")
			}
			if !tc.invalid {
				expected := ReactionStats{}
				if err = json.Unmarshal([]byte(tc.result), &expected); err != nil {
					t.Fatal(err)
				}
				a, _ := json.Marshal(expected)
				b, _ := json.Marshal(result)
				if string(a) != string(b) {
					t.Fatalf("counter shape changed: %s", b)
				}
			}
			if calls != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}

func TestReactionStatsFreshReadAndProfileBoundary(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q request[reactionStatsRequest]
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if calls == 1 {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"comment":2}}`, q.ID)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":1,"message":"private-canary","data":{"reason":"accessDenied"}}}`, q.ID)
	}))
	defer backend.Close()
	client, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "reactions-read"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetReactionStats(context.Background(), "actor"); err != nil {
		t.Fatal(err)
	}
	result, err := client.GetReactionStats(context.Background(), "actor")
	if err == nil || result != nil || err.Error() != "public API request failed (code=1 reason=accessDenied http=0)" {
		t.Fatal("revoke was cached or error leaked")
	}
	for _, profile := range []string{"read", "graph-read", "graph-create", "smart-forms"} {
		c, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: profile})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.GetReactionStats(context.Background(), "actor"); err == nil {
			t.Fatal("stats allowed outside profile")
		}
	}
	for _, id := range []string{"", " "} {
		if _, err = client.GetReactionStats(context.Background(), id); err == nil {
			t.Fatal("empty actor allowed")
		}
	}
	if calls != 2 {
		t.Fatal("invalid call reached backend")
	}
}

func TestReactionStatsHostActorBinding(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q request[reactionStatsRequest]
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if q.Params.RootActorID != "host-actor" {
			t.Error("host binding escaped")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{}}`, q.ID)
	}))
	defer backend.Close()
	client, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "reactions-read", ActorID: "host-actor"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetReactionStats(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{"host-actor", "other"} {
		if _, err = client.GetReactionStats(context.Background(), actor); err == nil {
			t.Fatal("model actor argument accepted in bound mode")
		}
	}
	if calls != 1 {
		t.Fatal("rejected binding reached API")
	}
	for _, profile := range []string{"read", "graph-read", "graph-create", "smart-forms", "delegated-read"} {
		if _, err = New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: profile, ActorID: "host-actor"}); err == nil {
			t.Fatal("actor binding silently ignored by another profile")
		}
	}
}
