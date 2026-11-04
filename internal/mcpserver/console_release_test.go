package mcpserver

import (
	"strings"
	"testing"
)

// Without releaseId the release production serves now is read, so a form released before the run
// can be embedded; a form never released says so.
func TestReleaseGetActive(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{
		"route/key/get": `"result":{"appKey":"leave","activeReleaseId":9}`,
		"release/get":   `"result":{"id":9,"actorId":"app1","envId":2,"sourceEnvId":1,"releaseNumber":3,"status":"active"}`,
	})
	reply := callTool(send, "smart_form_release_get", `{"appId":"app1"}`)
	if methodsOf(*calls) != "application/env/list,route/key/get,release/get" || (*calls)[2].Params["releaseId"] != float64(9) ||
		!strings.Contains(reply, `"releaseId":9,"releaseNumber":3,"status":"active","ready":true,"marker":"appId: app1"`) {
		t.Fatal(methodsOf(*calls), reply)
	}
	send, _ = consoleServer(t, map[string]string{"route/key/get": `"result":{"suggestedAppKey":"leave"}`})
	if reply = callTool(send, "smart_form_release_get", `{"appId":"app1"}`); !strings.Contains(reply, "no active production release") {
		t.Fatal(reply)
	}
	send, _ = consoleServer(t, nil)
	if reply = callTool(send, "smart_form_files_get", `{"appId":"app1"}`); !strings.Contains(reply, `"developEnvId":1,"productionEnvId":2`) {
		t.Fatal("env ids:", reply)
	}
}
