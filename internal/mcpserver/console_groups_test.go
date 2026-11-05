package mcpserver

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

func consoleTools(t *testing.T, profile string) *server.MCPServer {
	t.Helper()
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: profile, Authorization: "Bearer user"})
	if err != nil {
		t.Fatal(err)
	}
	return New(c)
}

// Every console-full tool has one or more groups from the allow-list, the groups name no other
// tool, and the groups of the Console's everyday scenarios hold the whole plain console catalog.
func TestConsoleGroups(t *testing.T) {
	tools := consoleTools(t, "console-full").ListTools()
	union := map[string]bool{}
	for g, names := range consoleGroups {
		if !slices.Contains(groupOrder, g) {
			t.Errorf("group %s is not in groupOrder", g)
		}
		for _, name := range names {
			if tools[name] == nil {
				t.Errorf("group %s names %s, which console-full does not have", g, name)
			}
			union[name] = true
		}
	}
	for name, tool := range tools {
		groups, _ := tool.Tool.Meta.AdditionalFields[groupsMeta].([]string)
		if tool.Tool.Meta == nil || len(groups) == 0 {
			t.Errorf("%s has no group", name)
			continue
		}
		for _, g := range groups {
			if !slices.Contains(groupOrder, g) {
				t.Errorf("%s: group %s is not allowed", name, g)
			}
		}
	}
	if len(union) != len(tools) {
		t.Errorf("the groups hold %d tools, console-full %d", len(union), len(tools))
	}
	// The everyday money group stays small: most Console finance requests need only it.
	if n := len(consoleGroups["finance"]); n > 18 {
		t.Errorf("finance has %d tools; keep the setup ones in finance_setup", n)
	}
	everyday := map[string]bool{}
	for _, g := range consoleCoreGroups {
		for _, name := range consoleGroups[g] {
			everyday[name] = true
		}
	}
	for name := range consoleCoreTools {
		if !everyday[name] {
			t.Errorf("%s of the plain console catalog is in none of %v", name, consoleCoreGroups)
		}
	}
}

// tools/list carries the groups as _meta, in the plain console catalog too.
func TestConsoleGroupsInToolsList(t *testing.T) {
	send, _ := consoleServer(t, nil)
	var list struct {
		Result struct {
			Tools []struct {
				Name string              `json:"name"`
				Meta map[string][]string `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(send(`"method":"tools/list"`)), &list); err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, tool := range list.Result.Tools {
		got[tool.Name] = tool.Meta[groupsMeta]
	}
	if fmt.Sprint(got["actor_get"]) != "[core]" || fmt.Sprint(got["actor_system_get"]) != "[graph finance people]" || fmt.Sprint(got["counter_set"]) != "[finance_setup]" || len(got) != 180 {
		t.Fatalf("%d tools; actor_get %v, actor_system_get %v", len(got), got["actor_get"], got["actor_system_get"])
	}
	for name, tool := range consoleTools(t, "console").ListTools() {
		if tool.Tool.Meta == nil || tool.Tool.Meta.AdditionalFields[groupsMeta] == nil {
			t.Errorf("console %s has no groups", name)
		}
	}
}

// TestConsoleGroupSizes logs each group's tools and the size of their tools/list entries (go test
// -run GroupSizes -v), the numbers of the README's group table.
func TestConsoleGroupSizes(t *testing.T) {
	tools := consoleTools(t, "console-full").ListTools()
	var lines []string
	for _, g := range groupOrder {
		size := 0
		for _, name := range consoleGroups[g] {
			b, _ := json.Marshal(tools[name].Tool)
			size += len(b)
		}
		lines = append(lines, fmt.Sprintf("%s %d %.1f", g, len(consoleGroups[g]), float64(size)/1024))
	}
	total := 0
	var names []string
	for name, tool := range tools {
		b, _ := json.Marshal(tool.Tool)
		total += len(b)
		names = append(names, name)
	}
	sort.Strings(names)
	t.Logf("groups (tools, KiB):\n%s\nconsole-full %d %.1f", strings.Join(lines, "\n"), len(names), float64(total)/1024)
}
