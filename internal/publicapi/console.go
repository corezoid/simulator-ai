package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"unicode/utf8"
)

// ConsoleEnabled is the AI Console profile: actor_get plus a plain actor/create, run per request
// under the initiator's own SIMULATOR_AI_AUTHORIZATION. Retries are deduplicated by sim-api for the
// Console task, not here, so there is no receipt key and no contract preflight.
func (c *Client) ConsoleEnabled() bool { return c.config.Profile == "console" }

// ActorCreateInput is the console subset of public actor/create.
type ActorCreateInput struct {
	FormID      int64           `json:"formId"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

type actorCreateRequest struct {
	AccID       string          `json:"accId"`
	FormID      int64           `json:"formId"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Data        json.RawMessage `json:"data"`
	Fields      []string        `json:"fields"`
}

// CreateActor creates one actor in the configured workspace and returns its metadata projection.
func (c *Client) CreateActor(ctx context.Context, in ActorCreateInput) (Actor, error) {
	if !c.ConsoleEnabled() {
		return Actor{}, errors.New("console profile is disabled")
	}
	if in.FormID < 1 || in.Title == "" || utf8.RuneCountInString(in.Title) > 1000 || utf8.RuneCountInString(in.Description) > 10000 {
		return Actor{}, errors.New("positive formId and a 1..1000 character title required")
	}
	data := bytes.TrimSpace(in.Data)
	if len(data) == 0 {
		data = []byte("{}")
	}
	if data[0] != '{' {
		return Actor{}, errors.New("data must be an object of form field values")
	}
	a, e := call[actorCreateRequest, Actor](ctx, c, "actor/create", actorCreateRequest{AccID: c.config.Workspace, FormID: in.FormID,
		Title: in.Title, Description: in.Description, Data: data, Fields: actorFields})
	if e == nil && (a.ID == nil || *a.ID == "") {
		return Actor{}, errors.New("actor/create returned no actor id")
	}
	return a, e
}
