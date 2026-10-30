package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/corezoid/simulator-ai/internal/mcpserver"
)

// Host formats differ only in how process configuration is inherited. Both must
// launch the same binary/catalog and install the same portable skill version.
func TestHostPackagingConfiguration(t *testing.T) {
	root := filepath.Join("..", "..", "plugins", "simulator-ai")
	for _, host := range []string{"codex", "claude"} {
		var manifest struct {
			Version    string `json:"version"`
			MCPServers string `json:"mcpServers"`
			Skills     string `json:"skills"`
		}
		readJSON(t, filepath.Join(root, "."+host+"-plugin", "plugin.json"), &manifest)
		if manifest.Version != mcpserver.Version || manifest.Skills != "./skills/" {
			t.Fatal("version or skill drift")
		}
		var config struct {
			MCPServers map[string]struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				EnvVars []string          `json:"env_vars"`
				Env     map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		readJSON(t, filepath.Join(root, manifest.MCPServers), &config)
		server, ok := config.MCPServers["simulator-ai"]
		if !ok || len(config.MCPServers) != 1 || server.Command != "simulator-ai" || len(server.Args) != 0 || len(server.Env) != 0 {
			t.Fatal("unexpected server or literal environment")
		}
		expected := []string(nil)
		if host == "codex" {
			expected = []string{"SIMULATOR_AI_ORIGIN", "SIMULATOR_AI_WORKSPACE", "SIMULATOR_AI_API_KEY", "SIMULATOR_AI_PROFILE"}
		}
		if !slices.Equal(server.EnvVars, expected) {
			t.Fatalf("%s environment forwarding drift", host)
		}
	}
}
func readJSON(t *testing.T, path string, result any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, result); err != nil {
		t.Fatal(err)
	}
}
