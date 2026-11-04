package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// The access tools take one object of any kind and dispatch to its public access_rule family: an
// actor, a form (account) template, the actors of a template, or an account (name + currency).

// AccessObject names the object whose access rules are read or changed.
type AccessObject struct {
	ObjType    string `json:"objType"`              // actor, formTemplate, templateActors or account
	ObjID      string `json:"objId"`                // actor id, template id or account name id
	CurrencyID int64  `json:"currencyId,omitempty"` // account only
}

// AccessRule grants one subject (user, group or API user) its privileges; all false removes it.
type AccessRule struct {
	UserID         int64           `json:"userId"`
	Privs          json.RawMessage `json:"privs"`
	ReactionOrders json.RawMessage `json:"reactionOrders,omitempty"`
}

type AccessRulesSaveInput struct {
	AccessObject
	Rules           []AccessRule `json:"rules,omitempty"`
	Remove          []int64      `json:"remove,omitempty"`
	Mode            string       `json:"mode,omitempty"` // merge (default) or replace
	IsNotify        bool         `json:"isNotify,omitempty"`
	IsRecursive     bool         `json:"isRecursive,omitempty"`
	FilterIncorrect bool         `json:"filterIncorrect,omitempty"`
}

type accessAction struct {
	Action         string          `json:"action"`
	UserID         int64           `json:"userId"`
	Privs          json.RawMessage `json:"privs,omitempty"`
	ReactionOrders json.RawMessage `json:"reactionOrders,omitempty"`
}

// target is the method family and the object's own params of an access call.
func (o AccessObject) target(workspace string) (string, map[string]any, error) {
	params := map[string]any{"accId": workspace}
	if o.ObjType != "account" && o.CurrencyID != 0 {
		return "", nil, errors.New("currencyId belongs to an account only")
	}
	switch o.ObjType {
	case "actor":
		if !validID(o.ObjID) {
			return "", nil, errors.New("objId: the actor id required")
		}
		params["actorId"] = o.ObjID
	case "formTemplate", "templateActors":
		id, err := strconv.ParseInt(o.ObjID, 10, 64)
		if err != nil || id < 1 {
			return "", nil, errors.New("objId: the template id required")
		}
		params["formId"] = id
	case "account":
		if !validID(o.ObjID) || o.CurrencyID < 1 {
			return "", nil, errors.New("objId (the account name id) and currencyId required")
		}
		params["nameId"], params["currencyId"] = o.ObjID, o.CurrencyID
	default:
		return "", nil, errors.New("objType is actor, formTemplate, templateActors or account")
	}
	return "access_rule/" + o.ObjType, params, nil
}

// AccessRules lists who has access to the object, with their privileges.
func (c *Client) AccessRules(ctx context.Context, o AccessObject) (json.RawMessage, error) {
	if e := consoleOnly(c); e != nil {
		return nil, e
	}
	family, params, err := o.target(c.config.Workspace)
	if err != nil {
		return nil, err
	}
	return call[map[string]any, json.RawMessage](ctx, c, family+"/list", params)
}

type accessList struct {
	List []struct {
		UserID int64 `json:"userId"`
	} `json:"list"`
}

// SaveAccessRules grants, changes or revokes access in one share call. merge leaves the subjects
// the call does not name as they are; replace also removes every other subject's rule (sim-api
// keeps the owner's).
func (c *Client) SaveAccessRules(ctx context.Context, in AccessRulesSaveInput) (json.RawMessage, error) {
	if e := consoleOnly(c); e != nil {
		return nil, e
	}
	family, params, err := in.target(c.config.Workspace)
	if err != nil {
		return nil, err
	}
	if in.Mode != "" && in.Mode != "merge" && in.Mode != "replace" {
		return nil, errors.New("mode is merge or replace")
	}
	if n := len(in.Rules) + len(in.Remove); n > 200 || n == 0 && in.Mode != "replace" {
		return nil, errors.New("1..200 rules and removals required (replace with none removes everyone but the owner)")
	}
	named := map[int64]bool{}
	var actions []accessAction
	for i, r := range in.Rules {
		if r.UserID < 1 || named[r.UserID] || len(r.Privs) == 0 {
			return nil, fmt.Errorf("rules[%d]: a userId named once and privs required", i)
		}
		named[r.UserID] = true
		actions = append(actions, accessAction{Action: "create", UserID: r.UserID, Privs: r.Privs, ReactionOrders: r.ReactionOrders})
	}
	for i, id := range in.Remove {
		if id < 1 || named[id] {
			return nil, fmt.Errorf("remove[%d]: a userId not named elsewhere required", i)
		}
		named[id] = true
		actions = append(actions, accessAction{Action: "delete", UserID: id})
	}
	if in.Mode == "replace" {
		current, err := call[map[string]any, accessList](ctx, c, family+"/list", params)
		if err != nil {
			return nil, err
		}
		for _, r := range current.List {
			if !named[r.UserID] {
				named[r.UserID] = true
				actions = append(actions, accessAction{Action: "delete", UserID: r.UserID})
			}
		}
	}
	if len(actions) == 0 {
		return json.RawMessage(`{"list":[]}`), nil
	}
	params["actions"], params["isNotify"], params["isRecursive"], params["filterIncorrect"] = actions, in.IsNotify, in.IsRecursive, in.FilterIncorrect
	return call[map[string]any, json.RawMessage](ctx, c, family+"/share", params)
}
