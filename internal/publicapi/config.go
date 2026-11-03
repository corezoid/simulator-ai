package publicapi

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"unicode"
)

// Config is supplied by the host, never by model/tool arguments.
type Config struct {
	Origin         string
	Workspace      string
	Profile        string
	APIKey         string `json:"-"`
	DelegatedToken string `json:"-"`
}

func (c Config) String() string       { return "[Simulator AI configuration redacted]" }
func (c Config) GoString() string     { return c.String() }
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }

func LoadConfig(getenv func(string) string) (Config, error) {
	c := Config{Origin: getenv("SIMULATOR_AI_ORIGIN"), Workspace: getenv("SIMULATOR_AI_WORKSPACE"), APIKey: getenv("SIMULATOR_AI_API_KEY"), Profile: getenv("SIMULATOR_AI_PROFILE"), DelegatedToken: getenv("SIMULATOR_AI_DELEGATED_TOKEN")}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
func (c Config) validate() error {
	if c.Profile != "" && c.Profile != "read" && c.Profile != "smart-forms" && c.Profile != "delegated-read" && c.Profile != "graph-read" && c.Profile != "graph-create" {
		return errors.New("SIMULATOR_AI_PROFILE must be read, smart-forms, graph-read, graph-create or delegated-read")
	}
	u, e := url.Parse(c.Origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return errors.New("SIMULATOR_AI_ORIGIN must be an origin without credentials, path, query or fragment")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || !(u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())) {
			return errors.New("HTTPS required except literal loopback development origins")
		}
	}
	credential := c.APIKey
	if c.Profile == "delegated-read" {
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
	}
	for _, v := range []string{c.Workspace, credential} {
		if v == "" || strings.IndexFunc(v, unicode.IsSpace) >= 0 || strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return errors.New("workspace and selected credential must be nonempty without whitespace")
		}
	}
	return nil
}

func (c *Client) DelegatedReadEnabled() bool { return c.config.Profile == "delegated-read" }

func (c Config) authorization() string {
	if c.Profile == "delegated-read" {
		return "Delegated " + c.DelegatedToken
	}
	return "ApiKey " + c.APIKey
}
