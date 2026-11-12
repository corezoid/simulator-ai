package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/corezoid/simulator-ai/internal/auth"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// loginWait is how long the login tool waits for the browser before answering "pending": below
// Codex's default 60 s tool timeout. The sign-in keeps running and saves itself afterwards.
var loginWait = 45 * time.Second

// Identity is whoami's answer: where requests go and as whom, never the credential itself.
type Identity struct {
	Origin string `json:"origin"`
	// Credential: login (your browser sign-in), apiKey (SIMULATOR_AI_API_KEY).
	Credential      string       `json:"credential"`
	AccountURL      string       `json:"accountUrl,omitempty"`
	CredentialsFile string       `json:"credentialsFile,omitempty"`
	User            *auth.Status `json:"user,omitempty"`
	Workspace       string       `json:"workspace,omitempty"`
	// WorkspaceSource: env (SIMULATOR_AI_WORKSPACE), selected (workspace_set) or none.
	WorkspaceSource string `json:"workspaceSource"`
	Next            string `json:"next,omitempty"`
}

// LoggedOut is logout's answer.
type LoggedOut struct {
	Removed bool   `json:"removed"`
	Note    string `json:"note"`
}

// WorkspaceSelected is workspace_set's answer.
type WorkspaceSelected struct {
	Workspace publicapi.MemberWorkspace `json:"workspace"`
	Note      string                    `json:"note"`
}

type workspaceListArgs struct {
	Query  string `json:"query,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}
type workspaceSetArgs struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	Name        string `json:"name,omitempty"`
}

const apiKeyLoginNotice = "login is disabled: this server uses the host credential SIMULATOR_AI_API_KEY, which always wins over a browser sign-in. To sign in as yourself instead, remove SIMULATOR_AI_API_KEY (and SIMULATOR_AI_WORKSPACE if you want to pick the workspace here) from the host configuration, restart the MCP server and call login"

func accountTool[T any](name, description, schema string, readOnly, idempotent bool) mcp.Tool {
	return mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(json.RawMessage(schema)), mcp.WithOutputSchema[T](),
		mcp.WithReadOnlyHintAnnotation(readOnly), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(idempotent))
}

// addAccount registers sign-in and workspace selection for a process that may use a browser login
// (no host Authorization, not delegated). Domain tools keep working on the host credential.
func addAccount(s *server.MCPServer, c *publicapi.Client) {
	a := c.Account()
	login := c.CredentialMode() == "login"
	loginDesc := "Sign in to Simulator.Company as yourself: opens the browser at the SuperAdmin sign-in page (OAuth2 PKCE) and stores the session for this API origin in the user's config directory, never in the project. Waits up to 45 s; if the answer is pending, the user finishes in the browser and you call whoami. If no browser page appeared, call login again: it returns authorizationUrl for the user to open. Then list workspaces with workspaces_list and ask the user which one to use (workspace_set). Call it only when the user wants to sign in or a tool says the sign-in is missing or expired."
	if !login {
		loginDesc = "DISABLED here: this server uses the host API key (SIMULATOR_AI_API_KEY). Calling it only explains how to switch to a browser sign-in."
	}
	s.AddTool(accountTool[auth.LoginResult]("login", loginDesc, `{"type":"object","properties":{},"additionalProperties":false}`, false, false), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if _, e := decode[struct{}](r); e != nil {
			return failure(e), nil
		}
		if !login {
			return failure(errors.New(apiKeyLoginNotice)), nil
		}
		v, e := a.Login(ctx, loginWait)
		return result(v, e), nil
	})
	s.AddTool(accountTool[LoggedOut]("logout", "Sign out locally: forget this API origin's stored browser sign-in and workspace choice (the SuperAdmin session in the browser stays). Only when the user asks.", `{"type":"object","properties":{},"additionalProperties":false}`, false, true), func(_ context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if _, e := decode[struct{}](r); e != nil {
			return failure(e), nil
		}
		if !login {
			return failure(errors.New("logout is disabled: this server uses the host API key (SIMULATOR_AI_API_KEY); remove it from the host configuration instead")), nil
		}
		removed, e := a.Logout()
		return result(LoggedOut{Removed: removed, Note: "Signed out of " + c.Origin() + " on this computer. Call login to sign in again."}, e), nil
	})
	s.AddTool(accountTool[Identity]("whoami", "Show where requests go and as whom: API origin, credential kind (login or apiKey), the signed-in user (SuperAdmin id, nick, expiry) and the workspace in use with its source. Never returns a token.", `{"type":"object","properties":{},"additionalProperties":false}`, true, true), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if _, e := decode[struct{}](r); e != nil {
			return failure(e), nil
		}
		v, e := whoami(c)
		return result(v, e), nil
	})
	if s.GetTool("workspaces_list") == nil {
		s.AddTool(accountTool[publicapi.WorkspacePage]("workspaces_list", "List the workspaces you belong to (query finds them by name), one page at a time (limit, default 20, max 100; offset); current is the workspace in use. Show them to the user and let the user choose; then workspace_set.", `{"type":"object","properties":{"query":{"type":"string","maxLength":255},"limit":{"type":"integer","minimum":1,"maximum":100},"offset":{"type":"integer","minimum":0}},"additionalProperties":false}`, true, true), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			in, e := decode[workspaceListArgs](r)
			if e != nil {
				return failure(e), nil
			}
			limit := 20
			if in.Limit != nil {
				limit = *in.Limit
			}
			v, e := c.ListWorkspaces(ctx, in.Query, limit, in.Offset)
			return result(v, e), nil
		})
	}
	s.AddTool(accountTool[WorkspaceSelected]("workspace_set", "Use one of your workspaces for every other tool from now on, remembered for this API origin. Pass workspaceId (from workspaces_list) or its exact name. Only with the user's explicit choice; never pick a workspace yourself. Not available when the host sets SIMULATOR_AI_WORKSPACE or an API key.", `{"type":"object","properties":{"workspaceId":{"type":"string","minLength":1,"maxLength":255},"name":{"type":"string","minLength":1,"maxLength":255}},"additionalProperties":false}`, false, true), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		in, e := decode[workspaceSetArgs](r)
		if e != nil {
			return failure(e), nil
		}
		w, e := c.SetWorkspace(ctx, in.WorkspaceID, in.Name)
		return result(WorkspaceSelected{Workspace: w, Note: "Every tool now works in this workspace."}, e), nil
	})
}

func whoami(c *publicapi.Client) (Identity, error) {
	id := Identity{Origin: c.Origin(), Credential: c.CredentialMode(), WorkspaceSource: "none"}
	ws := c.Workspace()
	switch {
	case c.WorkspacePinned():
		id.Workspace, id.WorkspaceSource = ws, "env"
	case ws != "":
		id.Workspace, id.WorkspaceSource = ws, "selected"
	}
	if id.Credential != "login" {
		return id, nil
	}
	a := c.Account()
	id.AccountURL, id.CredentialsFile = a.AccountURL(), a.StorePath()
	st, err := a.Status()
	if err != nil {
		return Identity{}, err
	}
	id.User = &st
	switch {
	case !st.SignedIn || st.Expired && !st.Renewable:
		id.Next = "call login"
	case id.Workspace == "":
		id.Next = "call workspaces_list, ask the user which workspace to use, then workspace_set"
	}
	return id, nil
}
