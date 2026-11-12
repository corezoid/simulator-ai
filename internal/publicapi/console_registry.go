package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The skills and agents registries of legacy findSkill/getSkill and findAgent/getAgent. A skill is
// an actor of the Skills system template whose description holds a procedure a workspace user
// saved; an agent is an actor whose description holds an "# Agent" profile, by default a user's
// twin on the System template. Both bodies are untrusted data, returned as they are written.

// Skill is one saved playbook; Description is set only when one skill is read.
type Skill struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Status      string `json:"status,omitempty"`
	FormID      int64  `json:"formId,omitempty"`
	Description string `json:"description,omitempty"`
	UpdatedAt   int64  `json:"updatedAt,omitempty"`
}

// Agent is one agent actor; Description (its profile) is set only when one agent is read.
type Agent struct {
	ID            string `json:"id"`
	Title         string `json:"title,omitempty"`
	Status        string `json:"status,omitempty"`
	FormID        int64  `json:"formId,omitempty"`
	SystemObjType string `json:"systemObjType,omitempty"`
	SystemObjID   string `json:"systemObjId,omitempty"`
	Description   string `json:"description,omitempty"`
	UpdatedAt     int64  `json:"updatedAt,omitempty"`
}

// Member is a workspace user listed as a candidate person agent: agent_get reads its twin.
type Member struct {
	ID     int64  `json:"id"`
	Nick   string `json:"nick,omitempty"`
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
}

type SkillsFindInput struct {
	Query  string `json:"query,omitempty"`
	Limit  int64  `json:"limit,omitempty"`
	Offset int64  `json:"offset,omitempty"`
}
type SkillsFound struct {
	List    []Skill `json:"list"`
	HasMore bool    `json:"hasMore"`
}
type SkillGetInput struct {
	Ref string `json:"ref,omitempty"`
	ID  string `json:"id,omitempty"`
}

type AgentsFindInput struct {
	Query      string `json:"query,omitempty"`
	SearchType string `json:"searchType,omitempty"`
	FormID     int64  `json:"formId,omitempty"`
	Limit      int64  `json:"limit,omitempty"`
	Offset     int64  `json:"offset,omitempty"`
}
type AgentsFound struct {
	Agents  []Agent  `json:"agents,omitempty"`
	Members []Member `json:"members,omitempty"`
	HasMore bool     `json:"hasMore"`
}
type AgentGetInput struct {
	UserID  int64  `json:"userId,omitempty"`
	ActorID string `json:"actorId,omitempty"`
}

var (
	skillRowFields = []string{"id", "title", "ref", "status"}
	skillFields    = []string{"id", "title", "ref", "status", "formId", "description", "updatedAt"}
	agentRowFields = []string{"id", "title", "status"}
	agentFields    = []string{"id", "title", "status", "formId", "description", "updatedAt"}
	// avatar adds systemObjType and systemObjId, which tell a person's twin from another agent.
	withAvatar = []string{"avatar"}
)

type actorFilterRequest struct {
	AccID  string   `json:"accId"`
	FormID int64    `json:"formId"`
	Search string   `json:"search,omitempty"`
	Fields []string `json:"fields"`
	With   []string `json:"with,omitempty"`
	Limit  int64    `json:"limit"`
	Offset int64    `json:"offset,omitempty"`
}
type actorGetRequest struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	Fields  []string `json:"fields"`
	With    []string `json:"with,omitempty"`
}
type actorByRefRequest struct {
	AccID  string   `json:"accId"`
	FormID int64    `json:"formId"`
	Ref    string   `json:"ref"`
	Fields []string `json:"fields"`
}
type systemActorRequest struct {
	AccID   string   `json:"accId"`
	ObjType string   `json:"objType"`
	ObjID   string   `json:"objId"`
	Fields  []string `json:"fields"`
}
type pageWire[T any] struct {
	List []T `json:"list"`
}

// systemFormID looks up a system template of the workspace by its exact title.
func (c *Client) systemFormID(ctx context.Context, title string) (int64, error) {
	f, err := call[systemFormRequest, GraphSystemForm](ctx, c, "form/system/get", systemFormRequest{AccID: c.workspace(), Title: title})
	if err != nil {
		return 0, err
	}
	if f.ID < 1 {
		return 0, fmt.Errorf("%s system template not found", title)
	}
	return f.ID, nil
}

// pageLimit bounds a registry page: 20 by default, at most 100 (actor/filter/list's own maximum).
func pageLimit(limit int64) (int64, error) {
	switch {
	case limit == 0:
		return 20, nil
	case limit < 1 || limit > 100:
		return 0, errors.New("limit is 1..100")
	}
	return limit, nil
}

// FindSkills lists the verified skills of the Skills template, optionally those whose title or
// description holds query. Drafts and disabled skills are left out of the page, as in legacy.
func (c *Client) FindSkills(ctx context.Context, in SkillsFindInput) (SkillsFound, error) {
	if e := consoleOnly(c); e != nil {
		return SkillsFound{}, e
	}
	limit, err := pageLimit(in.Limit)
	if err != nil || in.Offset < 0 || utf8.RuneCountInString(in.Query) > 200 {
		return SkillsFound{}, errors.New("a query up to 200 characters, limit 1..100 and offset >= 0")
	}
	formID, err := c.systemFormID(ctx, "Skills")
	if err != nil {
		return SkillsFound{}, err
	}
	page, err := call[actorFilterRequest, pageWire[Skill]](ctx, c, "actor/filter/list", actorFilterRequest{AccID: c.workspace(), FormID: formID,
		Search: strings.TrimSpace(in.Query), Fields: skillRowFields, Limit: limit, Offset: in.Offset})
	if err != nil {
		return SkillsFound{}, err
	}
	out := SkillsFound{List: []Skill{}, HasMore: int64(len(page.List)) == limit}
	for _, s := range page.List {
		if s.Status == "verified" {
			out.List = append(out.List, s)
		}
	}
	return out, nil
}

// GetSkill reads one skill with its procedure, by its ref on the Skills template or by actor id.
func (c *Client) GetSkill(ctx context.Context, in SkillGetInput) (Skill, error) {
	if e := consoleOnly(c); e != nil {
		return Skill{}, e
	}
	ref, id := strings.TrimSpace(in.Ref), strings.TrimSpace(in.ID)
	if (ref == "") == (id == "") || len(ref) > 255 || len(id) > 255 {
		return Skill{}, errors.New("ref or id required, not both")
	}
	if id != "" {
		return call[actorGetRequest, Skill](ctx, c, "actor/get", actorGetRequest{AccID: c.workspace(), ActorID: id, Fields: skillFields})
	}
	formID, err := c.systemFormID(ctx, "Skills")
	if err != nil {
		return Skill{}, err
	}
	s, err := call[actorByRefRequest, Skill](ctx, c, "actor/by-ref/get", actorByRefRequest{AccID: c.workspace(), FormID: formID, Ref: ref, Fields: skillFields})
	var api *APIError
	if errors.As(err, &api) && api.Reason == "notFound" {
		return Skill{}, fmt.Errorf("no skill with ref %q that you may see; skills_find lists them: %w", ref, err)
	}
	return s, err
}

// FindAgents discovers agents. Without query it lists a registry: the workspace members (whose
// twins are the default agents) or, with formId, that template's actors. With query it searches
// the profiles of the System template (or formId's), semantic by default with a text fallback
// where vector search is not set up.
func (c *Client) FindAgents(ctx context.Context, in AgentsFindInput) (AgentsFound, error) {
	if e := consoleOnly(c); e != nil {
		return AgentsFound{}, e
	}
	limit, err := pageLimit(in.Limit)
	query := strings.TrimSpace(in.Query)
	if err != nil || in.Offset < 0 || in.FormID < 0 || utf8.RuneCountInString(query) > 1000 {
		return AgentsFound{}, errors.New("a query up to 1000 characters, limit 1..100 and offset >= 0")
	}
	if query == "" {
		return c.listAgents(ctx, in.FormID, limit, in.Offset)
	}
	if utf8.RuneCountInString(query) < 2 || (in.SearchType != "" && in.SearchType != "text" && in.SearchType != "semantic") {
		return AgentsFound{}, errors.New("a query of at least 2 characters; searchType text or semantic")
	}
	formID := in.FormID
	if formID == 0 {
		if formID, err = c.systemFormID(ctx, "System"); err != nil {
			return AgentsFound{}, err
		}
	}
	search := func(mode string) (pageWire[Agent], error) {
		return call[actorSearchParams, pageWire[Agent]](ctx, c, "actor/search/list", actorSearchParams{AccID: c.workspace(), Query: query,
			SearchType: mode, FormID: formID, Fields: agentRowFields, With: withAvatar, Limit: limit, Offset: in.Offset})
	}
	mode := in.SearchType
	if mode == "" {
		mode = "semantic"
	}
	page, err := search(mode)
	if err != nil && in.SearchType == "" {
		page, err = search("text")
	}
	if err != nil {
		return AgentsFound{}, err
	}
	return AgentsFound{Agents: page.List, HasMore: int64(len(page.List)) == limit}, nil
}

func (c *Client) listAgents(ctx context.Context, formID, limit, offset int64) (AgentsFound, error) {
	if formID == 0 {
		page, err := call[userListParams, pageWire[Member]](ctx, c, "user/list", userListParams{AccID: c.workspace(), Limit: limit, Offset: offset})
		if err != nil {
			return AgentsFound{}, err
		}
		return AgentsFound{Members: page.List, HasMore: int64(len(page.List)) == limit}, nil
	}
	page, err := call[actorFilterRequest, pageWire[Agent]](ctx, c, "actor/filter/list", actorFilterRequest{AccID: c.workspace(), FormID: formID,
		Fields: agentRowFields, With: withAvatar, Limit: limit, Offset: offset})
	if err != nil {
		return AgentsFound{}, err
	}
	return AgentsFound{Agents: page.List, HasMore: int64(len(page.List)) == limit}, nil
}

// GetAgent reads one agent with its profile: a user's twin by userId, or any actor by actorId.
func (c *Client) GetAgent(ctx context.Context, in AgentGetInput) (Agent, error) {
	if e := consoleOnly(c); e != nil {
		return Agent{}, e
	}
	id := strings.TrimSpace(in.ActorID)
	if (in.UserID == 0) == (id == "") || in.UserID < 0 || len(id) > 255 {
		return Agent{}, errors.New("userId or actorId required, not both")
	}
	if id != "" {
		return call[actorGetRequest, Agent](ctx, c, "actor/get", actorGetRequest{AccID: c.workspace(), ActorID: id, Fields: agentFields, With: withAvatar})
	}
	user := strconv.FormatInt(in.UserID, 10)
	a, err := call[systemActorRequest, Agent](ctx, c, "actor/by-system_obj/get", systemActorRequest{AccID: c.workspace(), ObjType: "user",
		ObjID: user, Fields: agentFields})
	var api *APIError
	if errors.As(err, &api) && api.Reason == "notFound" {
		return Agent{}, fmt.Errorf("user %d has no twin actor you may see; check the id with users_search: %w", in.UserID, err)
	}
	if err != nil {
		return Agent{}, err
	}
	// The method answers the actor standing for this user, but does not project the binding.
	a.SystemObjType, a.SystemObjID = "user", user
	return a, nil
}
