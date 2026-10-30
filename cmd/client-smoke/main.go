// client-smoke installs the plugin into disposable client profiles, without a model turn.
// Supported hosts: macOS and Linux (process-group cleanup).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type evidence struct {
	Profile                  string `json:"profile"`
	CodexCreate              bool   `json:"codexCreate"`
	FixtureCreates           int32  `json:"fixtureCreates"`
	RejectedTokenCountProbes int32  `json:"rejectedTokenCountProbes"`
	CodexVersion             string `json:"codexVersion"`
	ClaudeVersion            string `json:"claudeVersion"`
	CodexInstall             bool   `json:"codexInstall"`
	CodexSkill               bool   `json:"codexSkill"`
	CodexTools               int    `json:"codexTools"`
	CodexRead                bool   `json:"codexRead"`
	ClaudeInstall            bool   `json:"claudeInstall"`
	ClaudeSkill              bool   `json:"claudeSkill"`
	ClaudeMCP                bool   `json:"claudeMCP"`
	FixtureReads             int32  `json:"fixtureReads"`
}

func main() {
	root := flag.String("repo", ".", "repository root containing the built bin/simulator-ai")
	profile := flag.String("profile", "read", "read or smart-forms")
	flag.Parse()
	result, err := smoke(*root, *profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client smoke:", err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
func smoke(root, profile string) (evidence, error) {
	var result evidence
	if profile != "read" && profile != "smart-forms" {
		return result, errors.New("invalid profile")
	}
	result.Profile = profile
	root, err := filepath.Abs(root)
	if err != nil {
		return result, err
	}
	binary := filepath.Join(root, "bin", "simulator-ai")
	if info, err := os.Stat(binary); err != nil || info.Mode()&0111 == 0 {
		return result, errors.New("run make build first")
	}
	codex, err := exec.LookPath("codex")
	if err != nil {
		return result, err
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		return result, err
	}
	temp, err := os.MkdirTemp("", "simulator-ai-client-smoke-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(temp)
	for _, dir := range []string{"codex", "claude", "project"} {
		if err = os.Mkdir(filepath.Join(temp, dir), 0700); err != nil {
			return result, err
		}
	}
	f := startFixture()
	defer f.server.Close()
	// An allowlist prevents inherited provider/production credentials and user config from
	// entering the disposable clients. HOME remains the user's HOME, never overwritten.
	env := []string{
		"HOME=" + os.Getenv("HOME"), "PATH=" + filepath.Dir(binary) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMPDIR=" + temp, "CODEX_HOME=" + filepath.Join(temp, "codex"), "CLAUDE_CONFIG_DIR=" + filepath.Join(temp, "claude"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1",
		"SIMULATOR_AI_PROFILE=" + profile, "SIMULATOR_AI_ORIGIN=" + f.server.URL, "SIMULATOR_AI_WORKSPACE=synthetic-workspace", "SIMULATOR_AI_API_KEY=synthetic-client-key",
	}
	// Fail closed if a future smoke change accidentally attempts a model turn.
	config := fmt.Sprintf("model = \"scripted-no-model\"\nmodel_provider = \"scripted\"\n[model_providers.scripted]\nname = \"Scripted smoke only\"\nbase_url = %q\nwire_api = \"responses\"\n", f.server.URL+"/disabled-model")
	if err = os.WriteFile(filepath.Join(temp, "codex", "config.toml"), []byte(config), 0600); err != nil {
		return result, err
	}
	env = append(env, "ANTHROPIC_BASE_URL="+f.server.URL+"/disabled-model", "ANTHROPIC_API_KEY=synthetic-no-model-key")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	project := filepath.Join(temp, "project")
	if err = codexSmoke(ctx, env, project, root, codex, &result); err != nil {
		return result, err
	}
	if err = claudeSmoke(ctx, env, project, root, claude, &result); err != nil {
		return result, err
	}
	result.FixtureReads = f.reads.Load()
	result.FixtureCreates = f.creates.Load()
	expectedCreates := int32(0)
	if profile == "smart-forms" {
		expectedCreates = 1
	}
	result.RejectedTokenCountProbes = f.tokenCountProbes.Load()
	if f.unexpected.Load() != 0 || result.FixtureReads != 1 || result.FixtureCreates != expectedCreates {
		return result, errors.New("unexpected fixture calls or missing Codex read")
	}
	return result, nil
}
