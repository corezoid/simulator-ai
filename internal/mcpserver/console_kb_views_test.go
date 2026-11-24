package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

// The saved-view and memory tools are wired to their client calls (behaviour: publicapi tests).
func TestSavedViewAndMemoryTools(t *testing.T) {
	call, f, _ := kbWorkspace(t)
	f.AddActor(12, "Contract A", nil)
	reply, failed := call("saved_view_create", `{"title":"kb-test drafts","formId":12,"where":[[{"field":"status","op":"=","value":"Draft"}]]}`)
	var view struct {
		ViewID string `json:"viewId"`
		Q      string `json:"q"`
	}
	if failed || json.Unmarshal([]byte(reply), &view) != nil || view.Q != "status=draft" {
		t.Fatalf("create: %s", reply)
	}
	if reply, failed = call("saved_views_find", `{"query":"drafts"}`); failed || !strings.Contains(reply, view.ViewID) {
		t.Fatalf("find: %s", reply)
	}
	if reply, failed = call("saved_view_run", `{"viewId":"`+view.ViewID+`","count":true}`); failed || !strings.Contains(reply, "Contract A") || !strings.Contains(reply, `"total":1`) {
		t.Fatalf("run: %s", reply)
	}
	// The memory tools reach the client: this credential names no user, so whose memory it would
	// be is not the model's selfUserId to say (identity: publicapi TestPersonalMemoryOnTheTwin).
	f.TwinOf(7, 70)
	for _, tool := range []string{"memory_save", "memory_get"} {
		args := `{"selfUserId":7,"text":"- [preference] Short answers"}`
		if tool == "memory_get" {
			args = `{"selfUserId":7}`
		}
		if reply, failed = call(tool, args); !failed || !strings.Contains(reply, "names its user") {
			t.Fatalf("%s: %s", tool, reply)
		}
	}
}
