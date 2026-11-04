package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// LinkInput names what build_link links to (legacy buildLink, plus a Smart Form route). Ids are
// path segments of the Simulator web app's routes; mode is the graph view (layers, actors, trees).
type LinkInput struct {
	Entity      string `json:"entity"`
	ID          string `json:"id,omitempty"`
	StreamID    string `json:"streamId,omitempty"`
	SecondaryID string `json:"secondaryId,omitempty"`
	Mode        string `json:"mode,omitempty"`
	FocusID     string `json:"focusId,omitempty"`
	// Ref, Route, Params and Query build a smartForm link: the Smart Form's ref, a route name its
	// released pages declare, that route's parameter values (strings or numbers) and a query string.
	Ref    string                     `json:"ref,omitempty"`
	Route  string                     `json:"route,omitempty"`
	Params map[string]json.RawMessage `json:"params,omitempty"`
	Query  map[string]string          `json:"query,omitempty"`
	// AppKey (the /sf/<key> of the request's context) or AppID names the current Smart Form: the
	// link is then built inside it (console_runtime_links.go). GraphID is a runtime layer's graph.
	AppKey  string `json:"appKey,omitempty"`
	AppID   string `json:"appId,omitempty"`
	GraphID string `json:"graphId,omitempty"`
}

// Link is a root-relative runtime link inside the current Smart Form, or an absolute legacy web app
// URL on the configured origin, marked Legacy when a runtime one was asked for but has no page.
type Link struct {
	URL    string `json:"url"`
	Legacy bool   `json:"legacy,omitempty"`
}

// linkPath builds one entity's path from the short workspace id and escaped ids; the legacy
// buildLink route table (pong-front-end routes.json).
type linkPath func(acc string, in LinkInput) (string, error)

func required(entity, name, v string) (string, error) {
	if v == "" {
		return "", fmt.Errorf("entity %s requires %s", entity, name)
	}
	return v, nil
}

var linkPaths = map[string]linkPath{
	"actor":     idPath("actor", "/actors_graph/%s/view/%s"),
	"actorList": fixedPath("/actors_graph/%s/list"),
	"graph":     fixedPath("/actors_graph/%s/graph"),
	"layer": func(acc string, in LinkInput) (string, error) {
		id, err := required("layer", `id (the graph or layer id, "0" for the root)`, in.ID)
		if err != nil {
			return "", err
		}
		mode := in.Mode
		if mode == "" {
			mode = "layers"
		}
		path := fmt.Sprintf("/actors_graph/%s/graph/%s/%s", acc, id, mode)
		if in.FocusID != "" {
			path += "/" + in.FocusID
		}
		return path, nil
	},
	"event": func(acc string, in LinkInput) (string, error) {
		path := fmt.Sprintf("/events/%s/list", acc)
		if in.StreamID != "" {
			path += "/" + in.StreamID
			if in.SecondaryID != "" {
				path += "/" + in.SecondaryID
			}
		}
		return path, nil
	},
	"chat": func(acc string, in LinkInput) (string, error) {
		if in.ID == "" {
			return fmt.Sprintf("/chats/%s", acc), nil
		}
		stream := in.StreamID
		if stream == "" {
			stream = "chats"
		}
		return fmt.Sprintf("/chats/%s/list/%s/%s?tab=chat", acc, stream, in.ID), nil
	},
	"form": func(acc string, in LinkInput) (string, error) {
		path := fmt.Sprintf("/form/%s/edit", acc)
		if in.ID != "" {
			path += "/" + in.ID
		}
		return path, nil
	},
	"transaction": idPath("transaction", "/actors_graph/%s/transactions/view/%s"),
	"transfer":    idPath("transfer", "/actors_graph/%s/transfers/view/%s"),
	"accounts":    fixedPath("/actors_graph/%s/accounts"),
	"chart":       idPath("chart", "/dashboard/%s/view/%s"),
	"meeting":     idPath("meeting", "/meetingRoom/%s/%s"),
	"settings":    fixedPath("/settings/%s"),
}

func fixedPath(format string) linkPath {
	return func(acc string, _ LinkInput) (string, error) { return fmt.Sprintf(format, acc), nil }
}

func idPath(entity, format string) linkPath {
	return func(acc string, in LinkInput) (string, error) {
		id, err := required(entity, "id", in.ID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(format, acc, id), nil
	}
}

// shortWorkspace is the 8-character workspace segment of web app URLs.
func shortWorkspace(acc string) string {
	if len(acc) > 8 && acc[8] == '-' {
		return acc[:8]
	}
	return acc
}

// escaped percent-encodes every id that becomes a path segment, so an id cannot add segments.
func (in LinkInput) escaped() LinkInput {
	for _, v := range []*string{&in.ID, &in.StreamID, &in.SecondaryID, &in.FocusID} {
		*v = url.PathEscape(strings.TrimSpace(*v))
	}
	return in
}

type routeTarget struct {
	Kind        string `json:"kind"`
	WorkspaceID string `json:"workspaceId"`
	ScriptRef   string `json:"scriptRef"`
	Environment string `json:"environment"`
}
type routeBuildRequest struct {
	SourceTarget routeTarget                `json:"sourceTarget"`
	Name         string                     `json:"name"`
	Params       map[string]json.RawMessage `json:"params,omitempty"`
	Query        map[string]string          `json:"query,omitempty"`
}
type routeBuildWire struct {
	Href string `json:"href"`
}

// BuildLink returns the web app URL of an entity on the configured origin. A smartForm link is the
// short URL of a route of its active production release, which public route/build builds.
func (c *Client) BuildLink(ctx context.Context, in LinkInput) (Link, error) {
	if e := consoleOnly(c); e != nil {
		return Link{}, e
	}
	origin := strings.TrimRight(c.config.Origin, "/")
	if in.Entity != "smartForm" && (in.AppKey != "" || in.AppID != "") {
		return c.runtimeLink(ctx, in)
	}
	if in.Entity == "smartForm" {
		if !validID(in.Ref) || !validID(in.Route) {
			return Link{}, errors.New("entity smartForm requires ref (the Smart Form's ref) and route (a route name its pages declare)")
		}
		r, err := call[routeBuildRequest, routeBuildWire](ctx, c, "route/build", routeBuildRequest{SourceTarget: routeTarget{Kind: "direct", WorkspaceID: c.config.Workspace,
			ScriptRef: in.Ref, Environment: "production"}, Name: in.Route, Params: in.Params, Query: in.Query})
		if err != nil {
			return Link{}, err
		}
		if !strings.HasPrefix(r.Href, "/sf/") {
			return Link{}, errors.New("route/build returned no Smart Form path")
		}
		return Link{URL: origin + r.Href}, nil
	}
	return c.legacyLink(in)
}

// legacyLink is the legacy web app URL of an entity (pong-front-end routes.json).
func (c *Client) legacyLink(in LinkInput) (Link, error) {
	origin := strings.TrimRight(c.config.Origin, "/")
	build, ok := linkPaths[in.Entity]
	if !ok {
		return Link{}, fmt.Errorf("unknown entity %q", in.Entity)
	}
	if in.Mode != "" && in.Mode != "layers" && in.Mode != "actors" && in.Mode != "trees" {
		return Link{}, errors.New("mode is layers, actors or trees")
	}
	path, err := build(shortWorkspace(c.config.Workspace), in.escaped())
	if err != nil {
		return Link{}, err
	}
	return Link{URL: origin + path}, nil
}
