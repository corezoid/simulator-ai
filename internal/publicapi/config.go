package publicapi

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/corezoid/simulator-ai/internal/auth"
)

// DefaultOrigin is the production public API, used when SIMULATOR_AI_ORIGIN is unset.
const DefaultOrigin = "https://mw.simulator.company"

// Config is supplied by the host, never by model/tool arguments.
type Config struct {
	Origin         string
	Workspace      string
	Profile        string
	ActorID        string
	APIKey         string `json:"-"`
	DelegatedToken string `json:"-"`
	// Authorization is a complete header value, "Bearer <token>", "Simulator <jwt>" or
	// "ApiKey <key>": the run initiator's own credential when a host (Hermes relay) starts this
	// process per run. Exclusive with APIKey and DelegatedToken.
	Authorization string `json:"-"`
	// Account is the person's own browser login (OAuth2 PKCE through SuperAdmin). It supplies the
	// credential, and the workspace when SIMULATOR_AI_WORKSPACE is unset, only when none of the
	// three host credentials above is set; with an API key it only explains that login is off.
	Account *auth.Session `json:"-"`
}

func (c Config) String() string       { return "[Simulator AI configuration redacted]" }
func (c Config) GoString() string     { return c.String() }
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }

func LoadConfig(getenv func(string) string) (Config, error) {
	c := Config{Origin: getenv("SIMULATOR_AI_ORIGIN"), Workspace: getenv("SIMULATOR_AI_WORKSPACE"), APIKey: getenv("SIMULATOR_AI_API_KEY"), Profile: getenv("SIMULATOR_AI_PROFILE"), ActorID: getenv("SIMULATOR_AI_ACTOR"), DelegatedToken: getenv("SIMULATOR_AI_DELEGATED_TOKEN"), Authorization: getenv("SIMULATOR_AI_AUTHORIZATION")}
	if c.Origin == "" {
		c.Origin = DefaultOrigin
	}
	if c.Authorization == "" && c.DelegatedToken == "" && c.Profile != "delegated-read" {
		account := getenv("SIMULATOR_AI_ACCOUNT_URL")
		if account != "" {
			if err := checkOrigin("SIMULATOR_AI_ACCOUNT_URL", account); err != nil {
				return Config{}, err
			}
		}
		if dir := configDir(getenv); dir != "" && checkOrigin("SIMULATOR_AI_ORIGIN", c.Origin) == nil {
			c.Account = auth.NewSession(auth.Options{Origin: strings.TrimRight(c.Origin, "/"), ConfigDir: dir, AccountURL: account, ClientID: getenv("SIMULATOR_AI_OAUTH_CLIENT_ID")})
		}
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// configDir is $SIMULATOR_AI_CONFIG_DIR, else simulator-ai under $XDG_CONFIG_HOME or ~/.config.
func configDir(getenv func(string) string) string {
	if d := getenv("SIMULATOR_AI_CONFIG_DIR"); filepath.IsAbs(d) {
		return d
	}
	if d := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "simulator-ai")
	}
	for _, home := range []string{getenv("HOME"), getenv("USERPROFILE")} {
		if filepath.IsAbs(home) {
			return filepath.Join(home, ".config", "simulator-ai")
		}
	}
	return ""
}

// checkOrigin accepts an HTTPS origin, or HTTP on a literal loopback host for development; name is
// the variable it came from.
func checkOrigin(name, origin string) error {
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return errors.New(name + " must be an origin without credentials, path, query or fragment")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || !(u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())) {
			return errors.New("HTTPS required except literal loopback development origins")
		}
	}
	return nil
}

func (c Config) validate() error {
	if c.Profile != "" && c.Profile != "read" && c.Profile != "smart-forms" && c.Profile != "delegated-read" && c.Profile != "graph-read" && c.Profile != "graph-create" && c.Profile != "reactions-read" && c.Profile != "console" && c.Profile != "console-full" {
		return errors.New("SIMULATOR_AI_PROFILE must be read, smart-forms, graph-read, graph-create, reactions-read, console, console-full or delegated-read")
	}
	if c.ActorID != "" && ((c.Profile != "reactions-read" && c.Profile != "graph-read") || len(c.ActorID) > 255 || strings.IndexFunc(c.ActorID, unicode.IsSpace) >= 0 || strings.IndexFunc(c.ActorID, unicode.IsControl) >= 0) {
		return errors.New("SIMULATOR_AI_ACTOR requires reactions-read or graph-read and a nonempty actor ID up to 255 bytes without whitespace")
	}
	if e := checkOrigin("SIMULATOR_AI_ORIGIN", c.Origin); e != nil {
		return e
	}
	credential := c.APIKey
	if c.Authorization != "" {
		if c.APIKey != "" || c.DelegatedToken != "" || c.Profile == "delegated-read" {
			return errors.New("SIMULATOR_AI_AUTHORIZATION excludes SIMULATOR_AI_API_KEY, SIMULATOR_AI_DELEGATED_TOKEN and the delegated-read profile")
		}
		scheme, token, ok := strings.Cut(c.Authorization, " ")
		if !ok || (scheme != "Bearer" && scheme != "Simulator" && scheme != "ApiKey") {
			return errors.New(`SIMULATOR_AI_AUTHORIZATION must be "Bearer <token>", "Simulator <jwt>" or "ApiKey <key>"`)
		}
		credential = token
	} else if c.Profile == "delegated-read" {
		if c.APIKey != "" || len(c.DelegatedToken) != 43 {
			return errors.New("delegated-read requires only SIMULATOR_AI_DELEGATED_TOKEN; API key must be absent")
		}
		raw, err := base64.RawURLEncoding.Strict().DecodeString(c.DelegatedToken)
		if err != nil || len(raw) != 32 {
			return errors.New("invalid delegated credential encoding")
		}
		credential = c.DelegatedToken
	} else if c.DelegatedToken != "" {
		return errors.New("SIMULATOR_AI_DELEGATED_TOKEN requires the delegated-read profile")
	} else if c.APIKey == "" {
		// Login mode: the stored sign-in and workspace are checked per request, so the server starts
		// without them and its tools say what to do.
		if c.Account == nil {
			return errors.New("no credential: set SIMULATOR_AI_API_KEY, or run with a home directory so the login tool can store a sign-in")
		}
		if c.Workspace != "" && !validToken(c.Workspace) {
			return errors.New("workspace and selected credential must be nonempty without whitespace")
		}
		return nil
	}
	for _, v := range []string{c.Workspace, credential} {
		if !validToken(v) {
			return errors.New("workspace and selected credential must be nonempty without whitespace")
		}
	}
	return nil
}

func validToken(v string) bool {
	return v != "" && strings.IndexFunc(v, unicode.IsSpace) < 0 && strings.IndexFunc(v, unicode.IsControl) < 0
}

func (c *Client) DelegatedReadEnabled() bool { return c.config.Profile == "delegated-read" }

// CredentialMode names where the credential comes from: authorization (SIMULATOR_AI_AUTHORIZATION,
// a host such as the Hermes relay), delegated, apiKey or login (the person's stored sign-in).
func (c *Client) CredentialMode() string {
	switch {
	case c.config.Authorization != "":
		return "authorization"
	case c.config.Profile == "delegated-read":
		return "delegated"
	case c.config.APIKey != "":
		return "apiKey"
	}
	return "login"
}

// Account is the browser login; nil for a host-authorized or delegated process.
func (c *Client) Account() *auth.Session { return c.config.Account }

// Origin is the configured API origin.
func (c *Client) Origin() string { return strings.TrimRight(c.config.Origin, "/") }

// WorkspacePinned says SIMULATOR_AI_WORKSPACE fixes the workspace, so workspace_set cannot change it.
func (c *Client) WorkspacePinned() bool { return c.config.Workspace != "" }

// workspace is the host workspace, else (login mode) the one the person chose; "" when neither.
func (c *Client) workspace() string {
	if c.config.Workspace != "" || c.CredentialMode() != "login" {
		return c.config.Workspace
	}
	w, err := c.config.Account.Workspace()
	if err != nil {
		return ""
	}
	return w
}

// authorization is the header for the next request; a login is renewed when due.
func (c *Client) authorization(ctx context.Context) (string, error) {
	switch c.CredentialMode() {
	case "authorization":
		return c.config.Authorization, nil
	case "delegated":
		return "Delegated " + c.config.DelegatedToken, nil
	case "apiKey":
		return "ApiKey " + c.config.APIKey, nil
	}
	return c.config.Account.Authorization(ctx)
}

// errNoWorkspace is a login without a chosen workspace.
var errNoWorkspace = errors.New("no workspace selected: call workspaces_list, ask the user which workspace to use, then workspace_set (or set SIMULATOR_AI_WORKSPACE)")

// ready returns the request header, refusing early when a login lacks a sign-in or a workspace.
// workspace/list is the one call that needs no workspace.
func (c *Client) ready(ctx context.Context, method string) (string, error) {
	h, err := c.authorization(ctx)
	if err != nil {
		return "", err
	}
	if method != "workspace/list" && c.workspace() == "" {
		return "", errNoWorkspace
	}
	return h, nil
}
