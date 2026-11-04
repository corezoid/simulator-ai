package publicapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Runtime links (sim-api docs/ai-console.md "Посилання"). The person is always inside a ui-kit
// runtime Smart Form, /sf/<appKey>/…, where the legacy web app routes open nothing. So a link is
// built against the current Smart Form: its appKey (from the request's context, the /sf/<key> of
// its route) or its appId (route/key/get), the path by public route/build, and the state of a
// block (an open actor, a layer) as a #b=<blockId>:<route> fragment, which route/build never
// gives. Links are root-relative: the Console renders them in the same runtime.

// runtimeRoute is where an entity opens in the workspace's start Smart Form
// (sim-api internal/cdu/templates/init-workspace.json); a form of one's own declares other names,
// which the caller passes as route.
type runtimeRoute struct {
	name     string
	params   map[string]any
	query    map[string]string
	fragment string // "<blockId>:<block route>", unencoded
}

// runtimeRoutes maps an entity to its start Smart Form route; an entity missing here has no
// runtime page and falls back to the legacy web app URL.
var runtimeRoutes = map[string]func(acc string, in LinkInput) (runtimeRoute, error){
	"actor": func(acc string, in LinkInput) (runtimeRoute, error) {
		id, err := required("actor", "id", in.ID)
		return runtimeRoute{name: "workspace.actors", fragment: "actors-list:panel:" + acc + ":" + id}, err
	},
	"actorList": list("workspace.actors"),
	"graph":     list("workspace.graphs"),
	"layer": func(acc string, in LinkInput) (runtimeRoute, error) {
		if in.GraphID == "" || in.ID == "" {
			return runtimeRoute{}, errors.New("a runtime layer link needs graphId (the layer's graph) and id (the layer)")
		}
		state := in.GraphID + ":" + in.ID
		if in.FocusID != "" {
			state += ":" + in.FocusID
		}
		return runtimeRoute{name: "workspace.graphs-edit", params: map[string]any{"graphId": in.GraphID}, fragment: "graphEditor:" + state}, nil
	},
	"event": func(_ string, in LinkInput) (runtimeRoute, error) {
		r := runtimeRoute{name: "workspace.events"}
		if in.ID != "" {
			r.query = map[string]string{"eventId": in.ID}
			if tab := in.Query["tab"]; tab != "" {
				r.query["tab"] = tab
			}
		}
		return r, nil
	},
	"chat": func(acc string, in LinkInput) (runtimeRoute, error) {
		r := runtimeRoute{name: "workspace.chats"}
		if in.ID != "" {
			r.fragment = "chats-list:panel:" + acc + ":" + in.ID
		}
		return r, nil
	},
	"form": func(_ string, in LinkInput) (runtimeRoute, error) {
		if in.ID == "" {
			return runtimeRoute{name: "workspace.templates"}, nil
		}
		id, err := strconv.ParseInt(in.ID, 10, 64)
		if err != nil || id < 1 {
			return runtimeRoute{}, errors.New("a template link needs id, the template's number")
		}
		return runtimeRoute{name: "workspace.templates-edit", params: map[string]any{"templateId": id}}, nil
	},
	"transaction": list("workspace.transactions"),
	"settings":    list("workspace.settings"),
}

func list(name string) func(string, LinkInput) (runtimeRoute, error) {
	return func(string, LinkInput) (runtimeRoute, error) { return runtimeRoute{name: name}, nil }
}

var appKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type dynamicTarget struct {
	Kind   string `json:"kind"`
	AppKey string `json:"appKey"`
	Path   string `json:"path"`
}
type dynamicRouteRequest struct {
	SourceTarget dynamicTarget     `json:"sourceTarget"`
	Name         string            `json:"name"`
	Params       map[string]any    `json:"params,omitempty"`
	Query        map[string]string `json:"query,omitempty"`
}

// runtimeLink builds the link inside the current Smart Form. route (with params, query) names a
// route of the form itself; otherwise the entity's start Smart Form route is used, and an entity
// without one gets the legacy web app URL, marked legacy.
func (c *Client) runtimeLink(ctx context.Context, in LinkInput) (Link, error) {
	key, err := c.appKey(ctx, in)
	if err != nil && in.AppKey == "" {
		// The Smart Form appId names has no key to read (never released, or route/key/get refused):
		// the legacy URL still links the entity, marked as opening outside the current form.
		legacy, lerr := c.legacyLink(in)
		legacy.Legacy = true
		if lerr != nil {
			return Link{}, fmt.Errorf("%w; and no legacy URL either: %v", err, lerr)
		}
		return legacy, nil
	}
	if err != nil {
		return Link{}, err
	}
	var r runtimeRoute
	switch build, known := runtimeRoutes[in.Entity]; {
	case in.Route != "":
		r = runtimeRoute{name: in.Route, query: in.Query, params: map[string]any{}}
		for k, v := range in.Params {
			r.params[k] = v
		}
	case !known:
		legacy, err := c.legacyLink(in)
		legacy.Legacy = true
		return legacy, err
	default:
		if r, err = build(c.config.Workspace, in); err != nil {
			return Link{}, err
		}
	}
	built, err := call[dynamicRouteRequest, routeBuildWire](ctx, c, "route/build", dynamicRouteRequest{SourceTarget: dynamicTarget{Kind: "dynamic", AppKey: key, Path: "/"}, Name: r.name, Params: r.params, Query: r.query})
	if err != nil {
		return Link{}, fmt.Errorf("route/build %s in Smart Form %s (a form of its own names its routes in page/render routing.routes: pass route): %w", r.name, key, err)
	}
	if !strings.HasPrefix(built.Href, "/sf/") {
		return Link{}, errors.New("route/build returned no Smart Form path")
	}
	href := built.Href
	if block, state, ok := strings.Cut(r.fragment, ":"); ok {
		href += "#b=" + encodeURIComponent(block) + ":" + encodeURIComponent(state)
	}
	return Link{URL: href}, nil
}

// appKey is the given short key (lowercased, as the runtime does) or the key of the Smart Form appId.
func (c *Client) appKey(ctx context.Context, in LinkInput) (string, error) {
	if in.AppKey != "" {
		key := strings.ToLower(strings.TrimSpace(in.AppKey))
		if !appKeyPattern.MatchString(key) || len(key) > 32 {
			return "", fmt.Errorf("appKey %q is not a Smart Form short key (the <key> of /sf/<key>)", in.AppKey)
		}
		return key, nil
	}
	got, err := call[map[string]any, struct {
		AppKey string `json:"appKey"`
	}](ctx, c, "route/key/get", map[string]any{"accId": c.config.Workspace, "actorId": in.AppID})
	if err != nil {
		return "", fmt.Errorf("route/key/get: %w", err)
	}
	if got.AppKey == "" {
		return "", fmt.Errorf("Smart Form %s has no short URL yet (never released to production)", in.AppID)
	}
	return got.AppKey, nil
}

// encodeURIComponent is JavaScript's: everything but A-Z a-z 0-9 - _ . ! ~ * ' ( ) is escaped.
func encodeURIComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || strings.IndexByte("-_.!~*'()", ch) >= 0 {
			b.WriteByte(ch)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", ch)
	}
	return b.String()
}
