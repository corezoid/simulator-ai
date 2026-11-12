package publicapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/corezoid/simulator-ai/internal/auth"
)

func signedJWT(exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	payload, _ := json.Marshal(map[string]any{"sub": "42", "nick": "Ada", "exp": exp.Unix()})
	return enc([]byte(`{"alg":"HS256"}`)) + "." + enc(payload) + ".sig"
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// workspaceAPI answers workspace/list with two workspaces sharing a name and actor/get for any.
func workspaceAPI(t *testing.T, headers *[]string, accIDs *[]string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*headers = append(*headers, r.Header.Get("Authorization"))
		var q received
		_ = json.NewDecoder(r.Body).Decode(&q)
		*accIDs = append(*accIDs, q.Params.AccID)
		result := `{"id":"actor-a"}`
		if q.Method == "workspace/list" {
			result = `{"list":[{"id":"ws-1","name":"Sales","status":"active"},{"id":"ws-2","name":"Ops"},{"id":"ws-3","name":"ops"}]}`
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, result)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestLoadConfigModes(t *testing.T) {
	home := t.TempDir()
	c, err := LoadConfig(env(map[string]string{"HOME": home}))
	if err != nil || c.Origin != DefaultOrigin || c.Account == nil || c.Account.Origin() != DefaultOrigin || c.Account.AccountURL() != auth.DefaultAccountURL || c.Account.StorePath() != filepath.Join(home, ".config", "simulator-ai", "credentials.json") {
		t.Fatalf("login mode: %+v %v", c, err)
	}
	client, _ := New(c)
	if client.CredentialMode() != "login" || client.WorkspacePinned() {
		t.Fatal("login mode not selected")
	}
	if c, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_ORIGIN": "https://x.example", "SIMULATOR_AI_AUTHORIZATION": "Simulator a.b.c", "SIMULATOR_AI_WORKSPACE": "ws"})); err != nil || c.Account != nil {
		t.Fatal("a host Authorization must not get a browser login", err)
	}
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_PROFILE": "delegated-read"})); err == nil {
		t.Fatal("delegated-read without its token accepted")
	}
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_ACCOUNT_URL": "http://account.example"})); err == nil {
		t.Fatal("plain-HTTP account URL accepted")
	}
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_API_KEY": "k"})); err == nil {
		t.Fatal("API key without workspace accepted")
	}
	// Only a browser login defaults the origin; a host credential still needs one.
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_API_KEY": "k", "SIMULATOR_AI_WORKSPACE": "ws"})); err == nil {
		t.Fatal("API key sent to the default origin")
	}
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_AUTHORIZATION": "Simulator a.b.c", "SIMULATOR_AI_WORKSPACE": "ws"})); err == nil {
		t.Fatal("host Authorization sent to the default origin")
	}
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_PROFILE": "reactions-read", "SIMULATOR_AI_ACTOR": "a"})); err == nil {
		t.Fatal("actor-bound profile started on a browser login")
	}
	if _, err = LoadConfig(env(map[string]string{"HOME": home, "SIMULATOR_AI_ORIGIN": "https://x.example", "SIMULATOR_AI_API_KEY": "k", "SIMULATOR_AI_WORKSPACE": "ws", "SIMULATOR_AI_ACCOUNT_URL": "bad"})); err != nil {
		t.Fatal("login-only setting blocked API-key mode:", err)
	}
	if c, err = LoadConfig(env(map[string]string{"SIMULATOR_AI_CONFIG_DIR": home, "XDG_CONFIG_HOME": "/elsewhere"})); err != nil || !strings.HasPrefix(c.Account.StorePath(), home) {
		t.Fatal("SIMULATOR_AI_CONFIG_DIR ignored", err)
	}
	if c, err = LoadConfig(env(map[string]string{"XDG_CONFIG_HOME": home})); err != nil || c.Account.StorePath() != filepath.Join(home, "simulator-ai", "credentials.json") {
		t.Fatal("XDG_CONFIG_HOME ignored", err)
	}
}

func TestHostCredentialWinsOverStoredLogin(t *testing.T) {
	var headers, accIDs []string
	api := workspaceAPI(t, &headers, &accIDs)
	dir := t.TempDir()
	jwt := signedJWT(time.Now().Add(time.Hour))
	session := auth.NewSession(auth.Options{Origin: api.URL, ConfigDir: dir})
	if err := session.SetWorkspace("ws-stored"); err != nil {
		t.Fatal(err)
	}
	keyed, err := LoadConfig(env(map[string]string{"SIMULATOR_AI_ORIGIN": api.URL, "SIMULATOR_AI_CONFIG_DIR": dir, "SIMULATOR_AI_API_KEY": "host-key", "SIMULATOR_AI_WORKSPACE": "ws-env"}))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := New(keyed)
	if _, err = c.GetActor(context.Background(), "actor-a"); err != nil || headers[0] != "ApiKey host-key" || accIDs[0] != "ws-env" {
		t.Fatalf("API key mode: %v %v %v", err, headers, accIDs)
	}
	if _, err = c.SetWorkspace(context.Background(), "ws-1", ""); err == nil || !strings.Contains(err.Error(), "host configuration") {
		t.Fatal("API-key workspace changed", err)
	}
	// A login process reads the stored choice once; start this one without any.
	if err = session.SetWorkspace(""); err != nil {
		t.Fatal(err)
	}
	login, _ := LoadConfig(env(map[string]string{"SIMULATOR_AI_ORIGIN": api.URL, "SIMULATOR_AI_CONFIG_DIR": dir}))
	c, _ = New(login)
	// No sign-in yet: refused before any request, with the next step and no secret.
	if _, err = c.GetActor(context.Background(), "actor-a"); !errors.Is(err, auth.ErrLoginRequired) || !strings.Contains(err.Error(), "call the login tool") || len(headers) != 1 {
		t.Fatalf("unsigned login: %v", err)
	}
	if err = saveLogin(dir, api.URL, jwt); err != nil {
		t.Fatal(err)
	}
	if _, err = c.GetActor(context.Background(), "actor-a"); err == nil || !strings.Contains(err.Error(), "workspace_set") || len(headers) != 1 {
		t.Fatalf("no workspace: %v", err)
	}
	page, err := c.ListWorkspaces(context.Background(), "", 20, 0)
	if err != nil || len(page.List) != 3 || headers[1] != "Simulator "+jwt || accIDs[1] != "" {
		t.Fatalf("list without workspace: %v %v", err, accIDs)
	}
	for _, tc := range []struct{ id, name, want string }{{"ws-9", "", "not one of your"}, {"", "ops", "several"}, {"ws-1", "Sales", "exactly one"}} {
		if _, err = c.SetWorkspace(context.Background(), tc.id, tc.name); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	w, err := c.SetWorkspace(context.Background(), "", "sales")
	if err != nil || w.ID != "ws-1" || c.Workspace() != "ws-1" {
		t.Fatalf("by name: %+v %v", w, err)
	}
	n := len(headers)
	if _, err = c.GetActor(context.Background(), "actor-a"); err != nil || headers[n] != "Simulator "+jwt || accIDs[n] != "ws-1" {
		t.Fatalf("login request: %v %v", err, accIDs)
	}
	pinned, _ := LoadConfig(env(map[string]string{"SIMULATOR_AI_ORIGIN": api.URL, "SIMULATOR_AI_CONFIG_DIR": dir, "SIMULATOR_AI_WORKSPACE": "ws-env"}))
	c, _ = New(pinned)
	if c.Workspace() != "ws-env" {
		t.Fatal("SIMULATOR_AI_WORKSPACE must override the stored choice")
	}
	if _, err = c.SetWorkspace(context.Background(), "ws-1", ""); err == nil || !strings.Contains(err.Error(), "pins") {
		t.Fatal("pinned workspace changed", err)
	}
}

// saveLogin stores a sign-in the way a completed browser login does.
func saveLogin(dir, origin, jwt string) error {
	return auth.NewStore(dir).Update(origin, func(e *auth.Entry) {
		e.AccessToken, e.ExpiresAt = jwt, time.Now().Add(time.Hour)
	})
}

func TestExpiredLoginNeverReachesAPI(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(api.Close)
	dir := t.TempDir()
	_ = auth.NewStore(dir).Update(api.URL, func(e *auth.Entry) {
		e.AccessToken, e.ExpiresAt, e.Workspace = signedJWT(time.Now().Add(-time.Hour)), time.Now().Add(-time.Hour), "ws-1"
	})
	cfg, _ := LoadConfig(env(map[string]string{"SIMULATOR_AI_ORIGIN": api.URL, "SIMULATOR_AI_CONFIG_DIR": dir, "SIMULATOR_AI_PROFILE": "console"}))
	c, _ := New(cfg)
	if _, err := c.Pass(context.Background(), "actor/get", nil); !errors.Is(err, auth.ErrLoginRequired) || strings.Contains(err.Error(), ".sig") || calls.Load() != 0 {
		t.Fatalf("expired: %v", err)
	}
}

func TestLinkNeedsAWorkspaceInLoginMode(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(env(map[string]string{"SIMULATOR_AI_CONFIG_DIR": dir, "SIMULATOR_AI_PROFILE": "console"}))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := New(cfg)
	if _, err = c.BuildLink(context.Background(), LinkInput{Entity: "graph"}); !errors.Is(err, errNoWorkspace) {
		t.Fatalf("link without a workspace: %v", err)
	}
	_ = cfg.Account.SetWorkspace("ws-1")
	if l, err := c.BuildLink(context.Background(), LinkInput{Entity: "graph"}); err != nil || !strings.Contains(l.URL, "/actors_graph/") {
		t.Fatalf("link: %+v %v", l, err)
	}
}
