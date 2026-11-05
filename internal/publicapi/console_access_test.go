package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rpcCall struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// notFound is a result func's answer for sim-api's notFound error.
const notFound = `"error":{"code":-32004,"message":"not found: account","data":{"reason":"notFound"}}`

// rpcBackend answers every JSON-RPC call with result (an "error": member answers with that error)
// and records the calls.
func rpcBackend(t *testing.T, result func(method string) string) (*Client, *[]rpcCall) {
	t.Helper()
	var calls []rpcCall
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID string `json:"id"`
			rpcCall
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		calls = append(calls, q.rpcCall)
		if r := result(q.Method); strings.HasPrefix(r, `"error":`) {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,%s}`, q.ID, r)
		} else {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, r)
		}
	}))
	t.Cleanup(api.Close)
	c, err := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	if err != nil {
		t.Fatal(err)
	}
	return c, &calls
}

func TestAccessRulesDispatchByObjectKind(t *testing.T) {
	c, calls := rpcBackend(t, func(string) string { return `{"list":[]}` })
	for _, tc := range []struct {
		in     AccessObject
		method string
		params string
	}{
		{AccessObject{ObjType: "actor", ObjID: "a-1"}, "access_rule/actor/list", `{"accId":"ws","actorId":"a-1"}`},
		{AccessObject{ObjType: "formTemplate", ObjID: "12"}, "access_rule/formTemplate/list", `{"accId":"ws","formId":12}`},
		{AccessObject{ObjType: "templateActors", ObjID: "12"}, "access_rule/templateActors/list", `{"accId":"ws","formId":12}`},
		{AccessObject{ObjType: "account", ObjID: "name-1", CurrencyID: 3}, "access_rule/account/list", `{"accId":"ws","currencyId":3,"nameId":"name-1"}`},
	} {
		if _, err := c.AccessRules(context.Background(), tc.in); err != nil {
			t.Fatal(tc.in, err)
		}
		last := (*calls)[len(*calls)-1]
		if params, _ := json.Marshal(last.Params); last.Method != tc.method || string(params) != tc.params {
			t.Errorf("%v called %s %s", tc.in, last.Method, params)
		}
	}
	for _, bad := range []AccessObject{{ObjType: "treeLayer", ObjID: "x"}, {ObjType: "formTemplate", ObjID: "x"}, {ObjType: "account", ObjID: "n"}, {ObjType: "actor", ObjID: "a", CurrencyID: 1}} {
		if _, err := c.AccessRules(context.Background(), bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestSaveAccessRulesMergeAndReplace(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		if strings.HasSuffix(method, "/list") {
			return `{"list":[{"userId":1,"userType":"user","privs":{"view":true}},{"userId":7,"userType":"user","privs":{"view":true}},{"userId":9,"userType":"group","privs":{"view":true,"execute":true}},` +
				`{"userId":3,"userType":"user","privs":{"view":true,"modify":true}},{"userId":4,"userType":"user","privs":{"view":true,"remove":true}},{"userId":0,"userType":"system","privs":{"view":true}},{"userId":5,"userType":"anonymous","privs":{"view":true}}]}`
		}
		return `{"list":[]}`
	})
	view := json.RawMessage(`{"view":true}`)
	in := AccessRulesSaveInput{AccessObject: AccessObject{ObjType: "actor", ObjID: "a-1"}, Rules: []AccessRule{{UserID: 7, Privs: json.RawMessage(`{"execute":true}`)}}, Remove: []int64{8}, IsNotify: true}
	if _, err := c.SaveAccessRules(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	merged, _ := json.Marshal((*calls)[0])
	if len(*calls) != 1 || !strings.Contains(string(merged), `"actions":[{"action":"create","privs":{"execute":true,"view":true},"userId":7},{"action":"delete","userId":8}]`) || !strings.Contains(string(merged), `"isNotify":true`) || strings.Contains(string(merged), "isRecursive") {
		t.Fatalf("merge sent %s", merged)
	}
	*calls = nil
	in.Mode = "replace"
	if _, err := c.SaveAccessRules(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	replaced, _ := json.Marshal((*calls)[1])
	// 3 may modify (the caller may be one of those), 4 remove, 0 is the system owner, 5 a public-link guest: kept.
	if len(*calls) != 2 || (*calls)[0].Method != "access_rule/actor/list" || !strings.Contains(string(replaced), `{"action":"delete","userId":8},{"action":"delete","userId":1},{"action":"delete","userId":9}],"actorId"`) || !strings.Contains(string(replaced), `"filterIncorrect":false`) {
		t.Fatalf("replace sent %v %s", (*calls)[0].Method, replaced)
	}
	// Flags the caller sets are sent as set.
	*calls = nil
	no := false
	if _, err := c.SaveAccessRules(context.Background(), AccessRulesSaveInput{AccessObject: in.AccessObject, Remove: []int64{8}, IsRecursive: &no, FilterIncorrect: true}); err != nil {
		t.Fatal(err)
	}
	if flags, _ := json.Marshal((*calls)[0].Params); !strings.Contains(string(flags), `"isRecursive":false`) || !strings.Contains(string(flags), `"filterIncorrect":true`) {
		t.Fatal("flags sent as", string(flags))
	}
	for name, bad := range map[string]AccessRulesSaveInput{
		"nothing to merge": {AccessObject: in.AccessObject},
		"a subject twice":  {AccessObject: in.AccessObject, Rules: []AccessRule{{UserID: 7, Privs: view}}, Remove: []int64{7}},
		"no privs":         {AccessObject: in.AccessObject, Rules: []AccessRule{{UserID: 7}}},
		"unknown mode":     {AccessObject: in.AccessObject, Remove: []int64{7}, Mode: "set"},
		"privs not flags":  {AccessObject: in.AccessObject, Rules: []AccessRule{{UserID: 7, Privs: json.RawMessage(`{"view":"yes"}`)}}},
	} {
		if _, err := c.SaveAccessRules(context.Background(), bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// A replace that names everyone already there changes nothing and returns the rules it read, not an
// empty list that would read as "nobody has access".
func TestReplaceWithNothingToRevokeReturnsTheRules(t *testing.T) {
	rules := `{"list":[{"userId":3,"userType":"user","privs":{"view":true,"modify":true}}]}`
	c, calls := rpcBackend(t, func(string) string { return rules })
	got, err := c.SaveAccessRules(context.Background(), AccessRulesSaveInput{AccessObject: AccessObject{ObjType: "actor", ObjID: "a-1"}, Mode: "replace"})
	if err != nil || string(got) != rules || len(*calls) != 1 {
		t.Fatalf("got %s (%v) after %d calls", got, err, len(*calls))
	}
}
