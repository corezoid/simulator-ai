package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ParityMethod is one pinned public method (contracts/parity-openrpc.json) behind a legacy parity
// tool. Its result is an explicit passthrough: the backend's own projection, shaped by the caller's
// documented fields/with, never re-modelled here.
type ParityMethod struct {
	Domain string // actor, form, graph; smartform, corezoid, task, meeting, access, reaction, attachment, user, finance (console only)
	Write  bool
}

// ParityMethods is the closed set of public methods the parity tools may call.
var ParityMethods = map[string]ParityMethod{
	"actor/create": {"actor", true}, "actor/get": {"actor", false}, "actor/by-ref/get": {"actor", false},
	"actor/update": {"actor", true}, "actor/delete": {"actor", true}, "actor/status/update": {"actor", true},
	"actor/search/list": {"actor", false}, "actor/filter/list": {"actor", false}, "actor/filter/count": {"actor", false},
	"actor/by-system_obj/get": {"actor", false}, "actor/corezoid/process/list": {"actor", false},
	"form/get": {"form", false}, "form/tree/get": {"form", false}, "form/list": {"form", false}, "form/count": {"form", false},
	"form/create": {"form", true}, "form/update": {"form", true}, "form/delete": {"form", true},
	"form/status/update": {"form", true}, "account_form/create": {"form", true}, "account_form/list": {"form", false},
	"account_form/delete": {"form", true},
	"edge/create":         {"graph", true}, "edge/get": {"graph", false}, "edge/update": {"graph", true},
	"edge/endpoints/update": {"graph", true}, "edge/delete": {"graph", true}, "edge/by-node/delete": {"graph", true},
	"edge/by-node/get":        {"graph", false},
	"edge_type/list":          {"graph", false},
	"graph/layer/actors/list": {"graph", false}, "graph/layer/edges/list": {"graph", false},
	"graph/layer/skeleton/get": {"graph", false}, "graph/layer/actors/manage": {"graph", true},
	"graph/layer/actors/positions/update": {"graph", true}, "graph/layer/actors/move": {"graph", true},
	"graph/layer/clean": {"graph", true}, "graph/layer/element/exist": {"graph", false},
	"graph/layer/stats/get": {"graph", false}, "graph/layer/actors/search/list": {"graph", false},
	"graph/list": {"graph", false}, "graph/get": {"graph", false},
	"cdu/catalog/get": {"smartform", false}, "app_folder/create": {"smartform", true},
	"release/list": {"smartform", false}, "release/diff": {"smartform", false}, "release/rollback": {"smartform", true},
	"file_version/list": {"smartform", false}, "file_version/get": {"smartform", false}, "file/rollback": {"smartform", true},
	"trash/list": {"smartform", false}, "trash/restore": {"smartform", true}, "page/render": {"smartform", true}, "page/submit": {"smartform", true},
	"corezoid/project/list": {"corezoid", false}, "corezoid/project/get": {"corezoid", false}, "corezoid/project/create": {"corezoid", true},
	"corezoid/project/bind": {"corezoid", true}, "corezoid/stage/list": {"corezoid", false}, "corezoid/process/list": {"corezoid", false},
	"graph/layer/edge_settings/save": {"graph", true},
	"actor/hole/conditions/get":      {"graph", false}, "actor/hole/revert": {"graph", true}, "edge/hole/revert": {"graph", true},
	"task/export/create": {"task", true}, "task/import/create": {"task", true}, "task/get": {"task", false},
	"public_link/create": {"meeting", true}, "public_link/get": {"meeting", false}, "public_link/revoke": {"meeting", true},
	"meet/transcription/list": {"meeting", false},
	"access_rule/actor/list":  {"access", false}, "access_rule/actor/share": {"access", true},
	"access_rule/formTemplate/list": {"access", false}, "access_rule/formTemplate/share": {"access", true},
	"access_rule/templateActors/list": {"access", false}, "access_rule/templateActors/share": {"access", true},
	"access_rule/account/list": {"access", false}, "access_rule/account/share": {"access", true},
	"access_rule/actor/request/create": {"access", true}, "access_rule/actor/request/list": {"access", false},
	"access_rule/actor/request/confirm": {"access", true}, "access_rule/actor/request/cancel": {"access", true},
	"reaction/list": {"reaction", false}, "reaction/count": {"reaction", false}, "reaction/create": {"reaction", true},
	"reaction/update": {"reaction", true}, "reaction/delete": {"reaction", true}, "reaction/read/update": {"reaction", true},
	"attachment/list": {"attachment", false}, "attachment/by-actor/list": {"attachment", false},
	"attachment/attach": {"attachment", true}, "attachment/update": {"attachment", true}, "attachment/detach": {"attachment", true},
	"user/list": {"user", false}, "user/get": {"user", false}, "workspace/list": {"user", false},
	"account/delete": {"finance", true}, "account_actor/create": {"finance", true},
	"account_actor/by-ref/create": {"finance", true}, "account_actor/list": {"finance", false}, "account_actor/count": {"finance", false},
	"account_actor/by-ref/list": {"finance", false}, "account_actor/get": {"finance", false},
	"account_actor/by-ref/get": {"finance", false}, "account_actor/update": {"finance", true}, "account_actor/status/update": {"finance", true},
	"account_actor/delete": {"finance", true}, "account_currency/create": {"finance", true}, "account_currency/get": {"finance", false},
	"account_currency/list": {"finance", false}, "account_currency/update": {"finance", true}, "account_name/create": {"finance", true},
	"account_name/get": {"finance", false}, "account_name/list": {"finance", false}, "account_name/update": {"finance", true},
	"counter/get": {"finance", false}, "counter/set": {"finance", true}, "counter/increment": {"finance", true},
	"counter/by-period/get": {"finance", false}, "transaction/create": {"finance", true}, "transaction/by-actor_ref/create": {"finance", true},
	"transaction/get": {"finance", false}, "transaction/by-ref/get": {"finance", false}, "transaction/list": {"finance", false},
	"transaction/account/list": {"finance", false}, "transaction/filter": {"finance", false}, "transaction/filter/count": {"finance", false},
	"transaction/children/list": {"finance", false}, "transfer/create": {"finance", true}, "transfer/get": {"finance", false},
	"transfer/by-ref/get": {"finance", false}, "transfer/filter": {"finance", false}, "transfer/filter/count": {"finance", false},
	"account_actor/amount/update": {"finance", true}, "account_actor/formula/get": {"finance", false}, "account_actor/formula/preview": {"finance", false},
	"account_actor/formula/update": {"finance", true}, "transaction/atom/create": {"finance", true}, "account_actor/by-id/get": {"finance", false},
	"account/tags/create": {"finance", true}, "account/tags/list": {"finance", false}, "account/tags/update": {"finance", true},
	"account/triggers/list": {"finance", false}, "account/triggers/update": {"finance", true}, "account_actor/triggers/list": {"finance", false},
	"account_actor/triggers/update": {"finance", true}, "account_actor/tags/list": {"finance", false}, "account_actor/tags/count": {"finance", false},
	"actor/field/triggers/list": {"finance", false}, "actor/field/triggers/update": {"finance", true},
}

// ParityAllowed says whether this profile exposes the method: console all of them; the read-only
// desktop profiles their reads (read: actors and forms, graph-read: graphs too).
func (c *Client) ParityAllowed(method string) bool {
	m, ok := ParityMethods[method]
	switch {
	case !ok:
		return false
	case c.ConsoleEnabled():
		return true
	case m.Write:
		return false
	case c.config.Profile == "" || c.config.Profile == "read":
		return m.Domain == "actor" || m.Domain == "form"
	case c.config.Profile == "graph-read" && c.config.ActorID == "":
		return m.Domain == "actor" || m.Domain == "form" || m.Domain == "graph"
	}
	return false
}

type parityParams map[string]json.RawMessage

// Pass calls one allowed parity method with the tool's (already schema-validated) arguments and
// the host workspace.
func (c *Client) Pass(ctx context.Context, method string, args map[string]json.RawMessage) (json.RawMessage, error) {
	if !c.ParityAllowed(method) {
		return nil, fmt.Errorf("%s is not available in this profile", method)
	}
	params := parityParams{}
	for k, v := range args {
		if k != "accId" {
			params[k] = v
		}
	}
	// workspace/list takes no workspace; a login that has not chosen one yet must still list them.
	if ws := c.workspace(); ws != "" || method != "workspace/list" {
		params["accId"], _ = json.Marshal(ws)
	}
	return call[parityParams, json.RawMessage](ctx, c, method, params)
}

// BatchResult is one item of a non-atomic batch: the method's result or its bounded error.
type BatchResult struct {
	Index  int             `json:"index"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type batchItem struct {
	ID     string       `json:"id"`
	Method string       `json:"method"`
	Params parityParams `json:"params"`
}
type batchRequest struct {
	List []batchItem `json:"list"`
}
type batchWire struct {
	List []struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	} `json:"list"`
}

// PassEach runs one allowed method once per item in a single public batch call. The batch is not
// atomic, as with the legacy mass tools: every item reports its own result or error.
func (c *Client) PassEach(ctx context.Context, method string, items []map[string]json.RawMessage) ([]BatchResult, error) {
	if !c.ParityAllowed(method) {
		return nil, fmt.Errorf("%s is not available in this profile", method)
	}
	if len(items) < 1 || len(items) > 100 {
		return nil, errors.New("1..100 items required")
	}
	req := batchRequest{List: make([]batchItem, len(items))}
	workspace, _ := json.Marshal(c.workspace())
	for i, item := range items {
		params := parityParams{"accId": workspace}
		for k, v := range item {
			if k != "accId" {
				params[k] = v
			}
		}
		req.List[i] = batchItem{ID: strconv.Itoa(i), Method: method, Params: params}
	}
	wire, err := call[batchRequest, batchWire](ctx, c, "batch", req)
	if err != nil {
		return nil, err
	}
	if len(wire.List) != len(items) {
		return nil, errors.New("batch returned a different number of items")
	}
	out := make([]BatchResult, len(items))
	for i, w := range wire.List {
		out[i] = BatchResult{Index: i, Result: w.Result}
		if w.Error != nil {
			out[i].Result = nil
			out[i].Error = c.apiError(w.Error).Error()
		}
	}
	return out, nil
}
