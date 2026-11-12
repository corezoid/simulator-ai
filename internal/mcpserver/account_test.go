package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/corezoid/simulator-ai/internal/auth"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func invoke(t *testing.T, s *server.MCPServer, name string, args map[string]any) (string, bool) {
	t.Helper()
	tool := s.GetTool(name)
	if tool == nil {
		t.Fatalf("%s not registered", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = name, args
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	return string(b), res.IsError
}

func accountServer(t *testing.T, vars map[string]string) (*server.MCPServer, string) {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"list":[{"id":"ws-1","name":"Sales"}]}}`, q.ID)
	}))
	t.Cleanup(api.Close)
	dir := t.TempDir()
	vars["SIMULATOR_AI_ORIGIN"], vars["SIMULATOR_AI_CONFIG_DIR"] = api.URL, dir
	cfg, err := publicapi.LoadConfig(func(k string) string { return vars[k] })
	if err != nil {
		t.Fatal(err)
	}
	c, err := publicapi.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return New(c), dir
}

func TestAccountToolsInLoginMode(t *testing.T) {
	s, dir := accountServer(t, map[string]string{})
	out, isErr := invoke(t, s, "whoami", map[string]any{})
	if isErr || !strings.Contains(out, `"credential":"login"`) || !strings.Contains(out, `"next":"call login"`) {
		t.Fatalf("whoami unsigned: %s", out)
	}
	if out, isErr = invoke(t, s, "actor_get", map[string]any{"actorId": "a"}); !isErr || !strings.Contains(out, "call the login tool") {
		t.Fatalf("domain tool before login: %s", out)
	}
	enc := base64.RawURLEncoding.EncodeToString
	jwt := enc([]byte("{}")) + "." + enc([]byte(`{"sub":"42","nick":"Ada"}`)) + ".token-canary"
	out, _ = invoke(t, s, "whoami", map[string]any{})
	var who Identity
	_ = json.Unmarshal([]byte(structured(t, out)), &who)
	// Stand in for a completed browser sign-in.
	_ = auth.NewStore(dir).Update(who.Origin, func(e *auth.Entry) { e.AccessToken, e.ExpiresAt = jwt, time.Now().Add(time.Hour) })
	if out, isErr = invoke(t, s, "workspaces_list", map[string]any{}); isErr || !strings.Contains(out, "Sales") {
		t.Fatalf("workspaces_list: %s", out)
	}
	if out, isErr = invoke(t, s, "workspace_set", map[string]any{"workspaceId": "ws-1"}); isErr || !strings.Contains(out, "ws-1") {
		t.Fatalf("workspace_set: %s", out)
	}
	out, _ = invoke(t, s, "whoami", map[string]any{})
	if !strings.Contains(out, `"workspaceSource":"selected"`) || !strings.Contains(out, `"nick":"Ada"`) || strings.Contains(out, "token-canary") {
		t.Fatalf("whoami signed: %s", out)
	}
	if out, isErr = invoke(t, s, "logout", map[string]any{}); isErr || !strings.Contains(out, `"removed":true`) {
		t.Fatalf("logout: %s", out)
	}
	if _, isErr = invoke(t, s, "login", map[string]any{"extra": 1}); !isErr {
		t.Fatal("login accepted arguments")
	}
}

func structured(t *testing.T, out string) string {
	t.Helper()
	var r struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if json.Unmarshal([]byte(out), &r) != nil {
		t.Fatal(out)
	}
	return string(r.StructuredContent)
}

func TestAccountToolsWithHostCredentials(t *testing.T) {
	s, _ := accountServer(t, map[string]string{"SIMULATOR_AI_API_KEY": "host-key", "SIMULATOR_AI_WORKSPACE": "ws-env"})
	if out, isErr := invoke(t, s, "login", map[string]any{}); !isErr || !strings.Contains(out, "SIMULATOR_AI_API_KEY") {
		t.Fatalf("login with an API key: %s", out)
	}
	if out, isErr := invoke(t, s, "workspace_set", map[string]any{"workspaceId": "ws-1"}); !isErr || !strings.Contains(out, "host configuration") {
		t.Fatalf("workspace_set with an API key: %s", out)
	}
	if out, _ := invoke(t, s, "whoami", map[string]any{}); !strings.Contains(out, `"credential":"apiKey"`) || !strings.Contains(out, `"workspaceSource":"env"`) || strings.Contains(out, "host-key") {
		t.Fatalf("whoami with an API key: %s", out)
	}
	// A host that hands over a complete Authorization (the Hermes relay) keeps today's catalog.
	hosted, _ := accountServer(t, map[string]string{"SIMULATOR_AI_AUTHORIZATION": "Simulator a.b.c", "SIMULATOR_AI_WORKSPACE": "ws", "SIMULATOR_AI_PROFILE": "console"})
	for _, name := range []string{"login", "logout", "whoami", "workspace_set"} {
		if hosted.GetTool(name) != nil {
			t.Fatalf("%s registered for a host Authorization", name)
		}
	}
	// console-full keeps its own parity workspaces_list.
	full, _ := accountServer(t, map[string]string{"SIMULATOR_AI_API_KEY": "k", "SIMULATOR_AI_WORKSPACE": "ws", "SIMULATOR_AI_PROFILE": "console-full"})
	if !strings.Contains(full.GetTool("workspaces_list").Tool.Description, "host workspace only") || full.GetTool("whoami") == nil {
		t.Fatal("console-full account tools")
	}
}
