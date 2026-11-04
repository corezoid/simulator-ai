package mcpserver

import (
	"encoding/json"
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

// The page tools address a page by ref and envTitle: sim-api refuses target next to accId, which
// every call carries, so their schemas do not offer it. The import tool offers no credentials.
func TestPageToolsHaveNoTarget(t *testing.T) {
	send, _ := consoleServer(t, nil)
	var list struct {
		Result struct {
			Tools []struct {
				Name   string          `json:"name"`
				Input  json.RawMessage `json:"inputSchema"`
				Annots map[string]any  `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(send(`"method":"tools/list"`)), &list); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, tool := range list.Result.Tools {
		// Corezoid credentials never travel in tool arguments.
		if tool.Name == "graph_import_create" && (strings.Contains(string(tool.Input), "scripts") || strings.Contains(string(tool.Input), "apiSecret") || !strings.Contains(string(tool.Input), `"fileName"`)) {
			t.Errorf("graph_import_create: %s", tool.Input)
		}
		if tool.Name != "smart_form_page_render" && tool.Name != "smart_form_page_submit" {
			continue
		}
		seen++
		if strings.Contains(string(tool.Input), `"target"`) || !strings.Contains(string(tool.Input), `"envTitle"`) || tool.Annots["readOnlyHint"] != false {
			t.Errorf("%s: %s %v", tool.Name, tool.Input, tool.Annots)
		}
	}
	if seen != 2 {
		t.Fatal("page tools listed:", seen)
	}
}
