package publicapi

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode"
)

// Config is supplied by the host, never by model/tool arguments.
type Config struct {
	Origin    string
	Workspace string
	APIKey    string
}

func LoadConfig(getenv func(string) string) (Config, error) {
	c := Config{Origin: getenv("SIMULATOR_AI_ORIGIN"), Workspace: getenv("SIMULATOR_AI_WORKSPACE"), APIKey: getenv("SIMULATOR_AI_API_KEY")}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
func (c Config) validate() error {
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
	for _, v := range []string{c.Workspace, c.APIKey} {
		if v == "" || strings.IndexFunc(v, unicode.IsSpace) >= 0 || strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return errors.New("SIMULATOR_AI_WORKSPACE and SIMULATOR_AI_API_KEY must be nonempty without whitespace")
		}
	}
	return nil
}
