package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDelegatedConfigurationIsExplicitAndRedacted(t *testing.T) {
	valid := Config{Origin: "https://example.invalid", Workspace: "workspace-a", Profile: "delegated-read", DelegatedToken: strings.Repeat("A", 43)}
	if _, err := New(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.APIKey = "broad-api-key" },
		func(c *Config) { c.Profile = "read" },
		func(c *Config) { c.Profile = "smart-forms" },
		func(c *Config) { c.Profile = "" },
		func(c *Config) { c.DelegatedToken = "" },
		func(c *Config) { c.DelegatedToken += "=" },
		func(c *Config) { c.DelegatedToken = strings.Repeat("!", 43) },
	} {
		bad := valid
		mutate(&bad)
		if _, err := New(bad); err == nil {
			t.Fatal("unsafe auth combination accepted")
		}
	}
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("fixture", "config", valid)
	for _, output := range []string{string(encoded), fmt.Sprintf("%v %+v %#v", valid, valid, valid), logs.String()} {
		if strings.Contains(output, valid.DelegatedToken) {
			t.Fatal("configuration leaked credential")
		}
	}
}

func TestDelegatedPublicReadsDenyWithoutFallback(t *testing.T) {
	token := strings.Repeat("A", 43)
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Delegated "+token {
			t.Error("wrong auth scheme")
		}
		var req received
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Params.AccID != "workspace-a" {
			t.Error("workspace widened")
		}
		if calls == 1 {
			if req.Method != "form/get" {
				t.Error("wrong public method")
			}
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":7,"title":"Form"}}`, req.ID)
		} else {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":1,"message":%q,"data":{"reason":"accessDenied"}}}`, req.ID, token)
		}
	}))
	defer api.Close()
	c, err := New(Config{Origin: api.URL, Workspace: "workspace-a", Profile: "delegated-read", DelegatedToken: token})
	if err != nil {
		t.Fatal(err)
	}
	if c.SmartFormsEnabled() {
		t.Fatal("writes enabled")
	}
	if _, err := c.GetForm(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetActor(context.Background(), "actor-a"); err == nil || strings.Contains(err.Error(), token) {
		t.Fatal("denial missing or leaked secret")
	}
	if calls != 2 {
		t.Fatal("unexpected retry")
	}
	if _, err := c.ListLayerActors(context.Background(), "layer-a", 1, 0); err == nil {
		t.Fatal("layer read accepted")
	}
	if _, err := call[ActorRequest, Actor](context.Background(), c, "actor/update", ActorRequest{}); err == nil {
		t.Fatal("write accepted")
	}
	if calls != 2 {
		t.Fatal("forbidden method reached network")
	}
}
