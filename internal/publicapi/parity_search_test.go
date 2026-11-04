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

func TestSearchAllKeepsServerTextToConsole(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"list":[{"id":"users","error":{"code":1,"message":"no viewUsers for key 42","data":{"reason":"internalDetail"}}},{"id":"actors","result":{"list":[]}}]}}`, q.ID)
	}))
	defer api.Close()
	for _, tc := range []struct {
		profile, auth string
		detail        bool
	}{{"read", "", false}, {"console", "Bearer user", true}} {
		cfg := Config{Origin: api.URL, Workspace: "ws", Profile: tc.profile, APIKey: "key"}
		if tc.auth != "" {
			cfg.APIKey, cfg.Authorization = "", tc.auth
		}
		c, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		out, err := c.SearchAll(context.Background(), SearchAllInput{Query: "acme", Filters: []string{"users", "actors"}})
		if err != nil || out.Actors == nil {
			t.Fatal(out, err)
		}
		msg := out.Errors["users"]
		if strings.Contains(msg, "viewUsers") != tc.detail || strings.Contains(msg, "internalDetail") || !strings.Contains(msg, "reason=remote_error") {
			t.Errorf("%s: users error %q", tc.profile, msg)
		}
	}
}
