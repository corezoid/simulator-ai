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
	IsRecursive     *bool        `json:"isRecursive,omitempty"` // absent: sim-api's default (an actor share cascades)
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
		UserID   int64           `json:"userId"`
		UserType string          `json:"userType"`
		Privs    map[string]bool `json:"privs"`
	} `json:"list"`
}

// withView adds view to privileges that grant anything: sim-api stores privs as sent and checks
// view alone, so an executor or approver without it could not even open the object.
func withView(privs json.RawMessage) (json.RawMessage, error) {
	var p map[string]bool
	if err := json.Unmarshal(privs, &p); err != nil {
		return nil, errors.New("privs must be an object of booleans")
	}
	for _, granted := range p {
		if granted && !p["view"] {
			p["view"] = true
			return json.Marshal(p)
		}
	}
	return privs, nil
}

// SaveAccessRules grants, changes or revokes access in one share call. merge leaves the subjects
// the call does not name as they are. replace also revokes the people, groups and API users that
// view, sign, sign legally or carry out the object; whoever may modify or remove it (the caller
// among them), the system owner and public-link guests stay unless remove names them. A replace
// with nothing to change returns the rules it read.
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
		return nil, errors.New("1..200 rules and removals required (replace with none revokes everyone who only views, signs or carries it out)")
	}
	named := map[int64]bool{}
	var actions []accessAction
	for i, r := range in.Rules {
		if r.UserID < 1 || named[r.UserID] || len(r.Privs) == 0 {
			return nil, fmt.Errorf("rules[%d]: a userId named once and privs required", i)
		}
		named[r.UserID] = true
		privs, err := withView(r.Privs)
		if err != nil {
			return nil, fmt.Errorf("rules[%d]: %w", i, err)
		}
		actions = append(actions, accessAction{Action: "create", UserID: r.UserID, Privs: privs, ReactionOrders: r.ReactionOrders})
	}
	for i, id := range in.Remove {
		if id < 1 || named[id] {
			return nil, fmt.Errorf("remove[%d]: a userId not named elsewhere required", i)
		}
		named[id] = true
		actions = append(actions, accessAction{Action: "delete", UserID: id})
	}
	var read json.RawMessage
	if in.Mode == "replace" {
		var current accessList
		if read, err = call[map[string]any, json.RawMessage](ctx, c, family+"/list", params); err == nil {
			err = json.Unmarshal(read, &current)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range current.List {
			kept := r.Privs["modify"] || r.Privs["remove"] || r.UserID < 1 || r.UserType == "system" || r.UserType == "anonymous"
			if !named[r.UserID] && !kept {
				named[r.UserID] = true
				actions = append(actions, accessAction{Action: "delete", UserID: r.UserID})
			}
		}
	}
	if len(actions) == 0 {
		return read, nil // replace: nobody to revoke, the rules stay as read
	}
	params["actions"], params["isNotify"], params["filterIncorrect"] = actions, in.IsNotify, in.FilterIncorrect
	if in.IsRecursive != nil {
		params["isRecursive"] = *in.IsRecursive
	}
	return call[map[string]any, json.RawMessage](ctx, c, family+"/share", params)
}
