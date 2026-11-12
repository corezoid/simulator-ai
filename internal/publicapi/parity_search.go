package publicapi

import (
	"context"
	"encoding/json"
	"errors"
)

// SearchAllInput is legacy searchAll: one query over the workspace's users and/or actors.
type SearchAllInput struct {
	Query      string   `json:"query"`
	Filters    []string `json:"filters"`
	SearchType string   `json:"searchType,omitempty"`
	FormID     int64    `json:"formId,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Limit      int64    `json:"limit,omitempty"`
	Offset     int64    `json:"offset,omitempty"`
}

// SearchAll holds each part's own page (no shared ranking, as in legacy /search) or its error.
type SearchAll struct {
	Users  json.RawMessage   `json:"users,omitempty"`
	Actors json.RawMessage   `json:"actors,omitempty"`
	Errors map[string]string `json:"errors,omitempty"`
}

type userListParams struct {
	AccID  string `json:"accId"`
	Query  string `json:"query,omitempty"`
	Limit  int64  `json:"limit,omitempty"`
	Offset int64  `json:"offset,omitempty"`
}
type actorSearchParams struct {
	AccID      string   `json:"accId"`
	Query      string   `json:"query"`
	SearchType string   `json:"searchType,omitempty"`
	FormID     int64    `json:"formId,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	With       []string `json:"with,omitempty"`
	Limit      int64    `json:"limit,omitempty"`
	Offset     int64    `json:"offset,omitempty"`
}

// SearchAll runs user/list and/or actor/search/list in one non-atomic public batch; a failed part
// is reported in Errors and does not hide the other. fullTextActor is the actor search in text mode.
func (c *Client) SearchAll(ctx context.Context, in SearchAllInput) (SearchAll, error) {
	if !c.ParityAllowed("actor/search/list") {
		return SearchAll{}, errors.New("search is not available in this profile")
	}
	// Text by default, as legacy searchAll documented: semantic search ignores offset.
	users, actors, searchType := false, false, in.SearchType
	if searchType == "" {
		searchType = "text"
	}
	for _, f := range in.Filters {
		switch f {
		case "users":
			users = true
		case "actors":
			actors = true
		case "fullTextActor":
			actors, searchType = true, "text"
		default:
			return SearchAll{}, errors.New("filters are users, actors or fullTextActor")
		}
	}
	if len([]rune(in.Query)) < 2 || (!users && !actors) {
		return SearchAll{}, errors.New("a query of at least 2 characters and one filter required")
	}
	var parts []string
	req := atomicRequest{}
	if users {
		parts = append(parts, "users")
		req.List = append(req.List, atomicItem{ID: "users", Method: "user/list", Params: userListParams{AccID: c.workspace(), Query: in.Query, Limit: in.Limit, Offset: in.Offset}})
	}
	if actors {
		parts = append(parts, "actors")
		req.List = append(req.List, atomicItem{ID: "actors", Method: "actor/search/list", Params: actorSearchParams{AccID: c.workspace(), Query: in.Query,
			SearchType: searchType, FormID: in.FormID, Fields: in.Fields, Limit: in.Limit, Offset: in.Offset}})
	}
	wire, err := call[atomicRequest, atomicWire](ctx, c, "batch", req)
	if err != nil {
		return SearchAll{}, err
	}
	if len(wire.List) != len(parts) {
		return SearchAll{}, errors.New("batch returned a different number of items")
	}
	out := SearchAll{}
	for i, item := range wire.List {
		if item.Error != nil {
			if out.Errors == nil {
				out.Errors = map[string]string{}
			}
			out.Errors[parts[i]] = c.apiError(item.Error).Error()
			continue
		}
		if parts[i] == "users" {
			out.Users = item.Result
		} else {
			out.Actors = item.Result
		}
	}
	return out, nil
}
