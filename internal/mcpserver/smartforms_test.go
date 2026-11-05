package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
)

func TestSmartFormsCatalogAndPublicWorkflow(t *testing.T) {
	hash := strings.Repeat("a", 64)
	methods := []string{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/2.0/doc/public" {
			_, _ = w.Write(contracts.SmartForms)
			return
		}
		if r.URL.Path != "/api/2.0/rpc" {
			t.Error("nonpublic request")
			w.WriteHeader(501)
			return
		}
		var q struct {
			ID     string                     `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if string(q.Params["accId"]) != `"workspace-a"` {
			t.Error("unbound workspace")
		}
		methods = append(methods, q.Method)
		output := ""
		switch q.Method {
		case "application/create", "application/operation/get":
			output = `{"application":{"id":"app-1","ref":"test-app"},"envs":[{"id":1,"actorId":"app-1","title":"develop"},{"id":2,"actorId":"app-1","title":"production"}]}`
		case "application/env/list":
			output = `{"canModify":true,"envs":[{"id":1,"actorId":"app-1","title":"develop","rootFolderId":10,"corezoidCredentials":{"apiSecret":"canary"}}]}`
		case "application/snapshot/get":
			output = `{"environmentHash":"` + hash + `","tree":{"id":10,"objType":"folder","title":"root","children":[{"id":30,"objType":"folder","title":"pages"}]}}`
		case "app_folder/create", "app_folder/operation/get":
			output = `{"folders":[{"id":31,"folderId":30,"objType":"folder","title":"main"}]}`
		case "app_file/create", "app_file/update", "app_file/operation/get":
			output = `{"files":[{"id":40,"folderId":31,"objType":"file","title":"config","type":"application/json","source":"{}","sourceHash":"44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"}]}`
		case "release/create", "release/operation/get", "release/get":
			output = `{"id":50,"actorId":"app-1","envId":2,"sourceEnvId":1,"status":"active","prunedByRetention":false}`
		case "route/key/get":
			output = `{"appKey":"test-app","activeReleaseId":50}`
		default:
			t.Error("unexpected RPC")
			w.WriteHeader(501)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, output)
	}))
	defer api.Close()
	c, err := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "workspace-a", APIKey: "synthetic", Profile: "smart-forms"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	if len(s.ListTools()) != 16 {
		t.Fatal("incomplete profile")
	}
	for name, tool := range s.ListTools() {
		write := strings.HasSuffix(name, "_create") || strings.HasSuffix(name, "_update")
		b, _ := json.Marshal(tool.Tool)
		if !strings.Contains(string(b), fmt.Sprintf(`"readOnlyHint":%t`, !write)) || !strings.Contains(string(b), fmt.Sprintf(`"destructiveHint":%t`, write)) {
			t.Fatalf("wrong annotations: %s", name)
		}
	}
	call := func(name, args string) string {
		t.Helper()
		reply := s.HandleMessage(context.Background(), json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, args)))
		b, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	steps := []struct{ name, args string }{
		{"application_create", `{"operationId":"create-1","ref":"test-app","title":"Test"}`},
		{"application_operation_get", `{"operationId":"create-1"}`},
		{"application_environments_list", `{"actorId":"app-1"}`},
		{"application_snapshot_get", `{"actorId":"app-1","envId":1}`},
		{"application_folders_create", `{"actorId":"app-1","operationId":"folder-1","items":[{"folderId":30,"title":"main"}]}`},
		{"application_folders_operation_get", `{"actorId":"app-1","operationId":"folder-1"}`},
		{"application_files_create", `{"actorId":"app-1","operationId":"file-1","items":[{"folderId":31,"title":"config","type":"application/json","source":"{}"}]}`},
		{"application_files_update", `{"actorId":"app-1","operationId":"file-2","environmentPreconditions":[{"envId":1,"environmentHash":"` + hash + `"}],"items":[{"id":40,"title":"config","type":"application/json","ifMatchHash":"` + hash + `","source":"{}"}]}`},
		{"application_files_operation_get", `{"actorId":"app-1","operationId":"file-2","method":"app_file/update"}`},
		{"application_release_create", `{"actorId":"app-1","operationId":"release-1","sourceEnvId":1,"targetEnvId":2,"expectedSourceEnvironmentHash":"` + hash + `","expectedTargetEnvironmentHash":"` + hash + `"}`},
		{"application_release_operation_get", `{"actorId":"app-1","operationId":"release-1"}`},
		{"application_release_get", `{"actorId":"app-1","releaseId":50}`},
		{"application_route_key_get", `{"actorId":"app-1"}`},
	}
	for _, step := range steps {
		out := call(step.name, step.args)
		if strings.Contains(out, `"error":`) || strings.Contains(out, `"isError":true`) || strings.Contains(out, "canary") {
			t.Fatalf("%s: %s", step.name, out)
		}
	}
	if len(methods) != 13 {
		t.Fatalf("workflow calls %v", methods)
	}
	for _, args := range []string{`{"operationId":"create-1","ref":"test-app","title":"Test","accId":"other"}`, `{"operationId":"create-1","ref":"test-app","title":"Test","corezoidCredentials":{}}`, `{"operationId":null,"ref":"test-app","title":"Test"}`} {
		out := call("application_create", args)
		if !strings.Contains(out, `"error":`) && !strings.Contains(out, `"isError":true`) {
			t.Fatalf("unsafe arguments accepted %s", out)
		}
	}
	if len(methods) != 13 {
		t.Fatal("invalid calls reached API")
	}
}
