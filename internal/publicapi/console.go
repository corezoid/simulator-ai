package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// ConsoleEnabled is the AI Console profile: plain public writes for a flowchart on a layer and for a
// released Smart Form, run per request under the initiator's own SIMULATOR_AI_AUTHORIZATION. Retries
// are deduplicated by sim-api for the Console task, not here, so there are no receipt keys and no
// contract preflight.
func (c *Client) ConsoleEnabled() bool {
	return c.config.Profile == "console" || c.config.Profile == "console-full"
}

// ConsoleCore is the plain console profile: only the tools the Console scenarios use, because every
// tool schema rides in the model context on every turn. console-full has every console tool.
func (c *Client) ConsoleCore() bool { return c.config.Profile == "console" }

// ActorCreateInput is the console subset of public actor/create.
type ActorCreateInput struct {
	FormID      int64           `json:"formId"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Color       string          `json:"color,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	// Hole creates a placeholder for an actor not known yet; its data fields are the closing conditions.
	Hole bool `json:"hole,omitempty"`
}

type actorCreateRequest struct {
	AccID       string          `json:"accId"`
	FormID      int64           `json:"formId"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Color       string          `json:"color,omitempty"`
	Data        json.RawMessage `json:"data"`
	Hole        bool            `json:"hole,omitempty"`
	Fields      []string        `json:"fields"`
}

func consoleOnly(c *Client) error {
	if !c.ConsoleEnabled() {
		return errors.New("console profile is disabled")
	}
	return nil
}

// objectData returns form field values as a JSON object, {} when omitted.
func objectData(raw json.RawMessage, optional bool) (json.RawMessage, error) {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 {
		if optional {
			return nil, nil
		}
		return json.RawMessage("{}"), nil
	}
	if data[0] != '{' {
		return nil, errors.New("data must be an object of form field values")
	}
	return data, nil
}

func validTitle(s string, optional bool) bool {
	return (optional || s != "") && utf8.RuneCountInString(s) <= 1000
}

func validID(s string) bool { return strings.TrimSpace(s) != "" && len(s) <= 255 }

// CreateActor creates one actor in the configured workspace and returns its metadata projection.
func (c *Client) CreateActor(ctx context.Context, in ActorCreateInput) (Actor, error) {
	if e := consoleOnly(c); e != nil {
		return Actor{}, e
	}
	if in.FormID < 1 || !validTitle(in.Title, false) || utf8.RuneCountInString(in.Description) > 10000 {
		return Actor{}, errors.New("positive formId and a 1..1000 character title required")
	}
	data, e := objectData(in.Data, false)
	if e != nil {
		return Actor{}, e
	}
	a, e := call[actorCreateRequest, Actor](ctx, c, "actor/create", actorCreateRequest{AccID: c.config.Workspace, FormID: in.FormID,
		Title: in.Title, Description: in.Description, Color: in.Color, Data: data, Fields: actorFields})
	if e == nil && (a.ID == nil || *a.ID == "") {
		return Actor{}, errors.New("actor/create returned no actor id")
	}
	return a, e
}
