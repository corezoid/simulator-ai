package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
)

func smartClient(t *testing.T, contract []byte, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/doc/public" {
			if r.Header.Get("Authorization") != "" {
				t.Error("credential sent to public documentation")
			}
			_, _ = w.Write(contract)
			return
		}
		if r.URL.Path != "/api/2.0/rpc" || r.Header.Get("Authorization") != "ApiKey synthetic-test-key" {
			t.Error("unexpected transport")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := New(Config{Origin: s.URL, Workspace: "workspace-a", APIKey: "synthetic-test-key", Profile: "smart-forms"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type smartWire struct {
	ID     string                     `json:"id"`
	Method string                     `json:"method"`
	Params map[string]json.RawMessage `json:"params"`
}

func readSmart(t *testing.T, r *http.Request) smartWire {
	t.Helper()
	var q smartWire
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		t.Error(err)
	}
	if string(q.Params["accId"]) != `"workspace-a"` {
		t.Error("workspace not pinned")
	}
	return q
}
func creation() CreateApplicationInput {
	return CreateApplicationInput{Operation: Operation{"create-1"}, Ref: "test-app", Title: "Test"}
}

const created = `{"application":{"id":"app-1","ref":"test-app","data":{"secret":"not-for-tools"}},"envs":[{"id":1,"actorId":"app-1","title":"develop","corezoidCredentials":{"apiSecret":"credential-canary"}},{"id":2,"actorId":"app-1","title":"production"}]}`

func TestSmartCreationLossRecoveryNoAutomaticRetry(t *testing.T) {
	writes, lookups := 0, 0
	c := smartClient(t, contracts.SmartForms, func(w http.ResponseWriter, r *http.Request) {
		q := readSmart(t, r)
		if string(q.Params["operationId"]) != `"create-1"` {
			t.Error("operation identity changed")
		}
		switch q.Method {
		case "application/create":
			writes++
			if string(q.Params["cduVersion"]) != "2" || q.Params["corezoidCredentials"] != nil {
				t.Error("unsafe creation")
			}
			if writes == 1 {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
				return
			}
		case "application/operation/get":
			lookups++
		default:
			t.Error("unexpected method")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, created)
	})
	p := creation()
	p.AccID = "attacker"
	p.CduVersion = 1
	_, err := c.CreateApplication(context.Background(), p)
	var uncertain *WriteError
	if !errors.As(err, &uncertain) || !strings.Contains(err.Error(), "preserve operationId") {
		t.Fatalf("lost write not marked unresolved: %v", err)
	}
	if writes != 1 {
		t.Fatal("automatically retried")
	}
	recovered, err := c.GetApplicationOperation(context.Background(), ApplicationOperationInput{Operation: Operation{"create-1"}})
	if err != nil || recovered.Application.ID == nil || *recovered.Application.ID != "app-1" {
		t.Fatalf("recovery: %v %+v", err, recovered)
	}
	b, _ := json.Marshal(recovered)
	for _, forbidden := range []string{"credential-canary", "corezoidCredentials", "not-for-tools", "data"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("leak %s", forbidden)
		}
	}
	replay, err := c.CreateApplication(context.Background(), p)
	if err != nil || *replay.Application.ID != *recovered.Application.ID || writes != 2 || lookups != 1 {
		t.Fatalf("explicit replay: %v", err)
	}
}
func TestSmartOldContractAndInvalidInputNeverWrite(t *testing.T) {
	calls := 0
	old := []byte(strings.ReplaceAll(string(contracts.SmartForms), `"name": "operationId"`, `"name": "oldField"`))
	c := smartClient(t, old, func(http.ResponseWriter, *http.Request) { calls++ })
	if _, err := c.CreateApplication(context.Background(), creation()); err == nil {
		t.Fatal("old contract accepted")
	}
	c.config.Profile = "read"
	if _, err := c.CreateApplication(context.Background(), creation()); err == nil {
		t.Fatal("disabled profile wrote")
	}
	c.config.Profile = "smart-forms"
	p := creation()
	p.OperationID = ""
	if _, err := c.CreateApplication(context.Background(), p); err == nil {
		t.Fatal("empty key accepted")
	}
	if calls != 0 {
		t.Fatal("unsafe input reached RPC")
	}
}
func TestSmartErrorReasonsAndReleaseBinding(t *testing.T) {
	for _, reason := range []string{"operationConflict", "operationResultExpired", "notAuthorized", "accessDenied", "notAvailable", "appEnvironmentChanged", "appFileSourceMismatch", "invalidRoutes"} {
		t.Run(reason, func(t *testing.T) {
			c := smartClient(t, contracts.SmartForms, func(w http.ResponseWriter, r *http.Request) {
				q := readSmart(t, r)
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":1,"message":"credential-canary","data":{"reason":%q}}}`, q.ID, reason)
			})
			_, err := c.CreateApplication(context.Background(), creation())
			var api *APIError
			if !errors.As(err, &api) || api.Reason != reason || strings.Contains(err.Error(), "credential-canary") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
	c := smartClient(t, contracts.SmartForms, func(w http.ResponseWriter, r *http.Request) {
		q := readSmart(t, r)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":9,"actorId":"another","envId":2,"sourceEnvId":1}}`, q.ID)
	})
	_, err := c.CreateRelease(context.Background(), CreateReleaseInput{AppTarget: AppTarget{"app-1"}, Operation: Operation{"release-1"}, SourceEnvID: 1, TargetEnvID: 2, ExpectedSourceEnvironmentHash: strings.Repeat("a", 64), ExpectedTargetEnvironmentHash: strings.Repeat("b", 64)})
	if err == nil {
		t.Fatal("wrong actor accepted")
	}
}
func TestSmartUpdateRequiresAllPreconditions(t *testing.T) {
	calls := 0
	c := smartClient(t, contracts.SmartForms, func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := readSmart(t, r)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"files":[{"id":3,"folderId":10,"objType":"file","title":"config","type":"application/json","source":"","sourceHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}]}}`, q.ID)
	})
	empty := ""
	p := UpdateFilesInput{AppTarget: AppTarget{"app-1"}, Operation: Operation{"update-1"}, Items: []FileUpdate{{ID: 3, Title: "config", Type: "application/json", Source: &empty, IfMatchHash: strings.Repeat("a", 64)}}}
	if _, err := c.UpdateFiles(context.Background(), p); err == nil {
		t.Fatal("missing full CAS accepted")
	}
	p.EnvironmentPreconditions = []EnvironmentPrecondition{{EnvID: 1, EnvironmentHash: strings.Repeat("b", 64)}}
	if _, err := c.UpdateFiles(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p.EnvironmentPreconditions = append(p.EnvironmentPreconditions, p.EnvironmentPreconditions[0])
	if _, err := c.UpdateFiles(context.Background(), p); err == nil {
		t.Fatal("duplicate environments accepted")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestReleaseRetentionMustBeExplicit(t *testing.T) {
	for _, field := range []string{"", `,"prunedByRetention":null`} {
		c := smartClient(t, contracts.SmartForms, func(w http.ResponseWriter, r *http.Request) {
			q := readSmart(t, r)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":9,"actorId":"app-1","envId":2,"sourceEnvId":1,"status":"active"%s}}`, q.ID, field)
		})
		if _, err := c.GetRelease(context.Background(), ReleaseInput{AppTarget: AppTarget{"app-1"}, ReleaseID: 9}); err == nil {
			t.Fatal("missing/null current retention accepted")
		}
		if _, err := c.GetReleaseOperation(context.Background(), AppOperationInput{AppTarget: AppTarget{"app-1"}, Operation: Operation{"release-1"}}); err == nil {
			t.Fatal("missing/null receipt retention accepted")
		}
		_, err := c.CreateRelease(context.Background(), CreateReleaseInput{AppTarget: AppTarget{"app-1"}, Operation: Operation{"release-1"}, SourceEnvID: 1, TargetEnvID: 2, ExpectedSourceEnvironmentHash: strings.Repeat("a", 64), ExpectedTargetEnvironmentHash: strings.Repeat("b", 64)})
		var uncertain *WriteError
		if !errors.As(err, &uncertain) {
			t.Fatalf("missing/null retention did not preserve unknown write: %v", err)
		}
	}
}
func TestUpdateWrongFileSuccessIsUnresolved(t *testing.T) {
	c := smartClient(t, contracts.SmartForms, func(w http.ResponseWriter, r *http.Request) {
		q := readSmart(t, r)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"files":[{"id":41,"folderId":1,"objType":"file","title":"config","type":"application/json","source":"","sourceHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}]}}`, q.ID)
	})
	_, err := c.UpdateFiles(context.Background(), UpdateFilesInput{AppTarget: AppTarget{"app-1"}, Operation: Operation{"update-1"}, Items: []FileUpdate{{ID: 40, Title: "config", Type: "application/json", IfMatchHash: strings.Repeat("a", 64)}}, EnvironmentPreconditions: []EnvironmentPrecondition{{EnvID: 1, EnvironmentHash: strings.Repeat("b", 64)}}})
	var uncertain *WriteError
	if !errors.As(err, &uncertain) {
		t.Fatalf("wrong file success was not unresolved: %v", err)
	}
}

func TestDuplicateUpdateFileIDsNeverReachAPI(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	c, err := New(Config{Origin: server.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "smart-forms"})
	if err != nil {
		t.Fatal(err)
	}
	first, second := "first", "second"
	_, err = c.UpdateFiles(context.Background(), UpdateFilesInput{AppTarget: AppTarget{"app-1"}, Operation: Operation{"update-1"}, Items: []FileUpdate{{ID: 40, Title: "config", Type: "application/json", Source: &first, IfMatchHash: strings.Repeat("a", 64)}, {ID: 40, Title: "renamed", Type: "application/json", Source: &second, IfMatchHash: strings.Repeat("a", 64)}}, EnvironmentPreconditions: []EnvironmentPrecondition{{EnvID: 1, EnvironmentHash: strings.Repeat("b", 64)}}})
	if err == nil || calls != 0 {
		t.Fatalf("duplicate ids reached API: calls=%d err=%v", calls, err)
	}
}
