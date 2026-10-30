package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

func claudeSmoke(ctx context.Context, env []string, dir, root, executable string, e *evidence) error {
	version, err := run(ctx, env, dir, executable, "--version")
	if err != nil {
		return err
	}
	e.ClaudeVersion = strings.TrimSpace(string(version))
	installed, err := run(ctx, env, dir, executable, "plugin", "install", "simulator-ai", "--marketplace", root, "--scope", "user", "--json")
	if err != nil {
		return err
	}
	var install struct {
		Outcome  string `json:"outcome"`
		PluginID string `json:"pluginId"`
	}
	if err = json.Unmarshal(installed, &install); err != nil {
		return err
	}
	if install.Outcome != "ok" || install.PluginID != "simulator-ai@simulator-ai" {
		return errors.New("Claude plugin install not confirmed")
	}
	e.ClaudeInstall = true
	details, err := run(ctx, env, dir, executable, "plugin", "details", "simulator-ai@simulator-ai")
	if err != nil {
		return err
	}
	if !strings.Contains(string(details), "simulator-ai-read") || !strings.Contains(string(details), "simulator-ai-smart-forms") {
		return errors.New("Claude did not load installed skill")
	}
	e.ClaudeSkill = true
	status, err := run(ctx, env, dir, executable, "mcp", "list")
	if err != nil {
		return err
	}
	if !claudeConnected(string(status)) {
		return errors.New("Claude installed MCP did not connect")
	}
	e.ClaudeMCP = true
	return nil
}

func claudeConnected(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "plugin:simulator-ai:simulator-ai:") && (strings.HasSuffix(line, "- ✔ Connected") || strings.HasSuffix(line, "- ✓ Connected")) {
			return true
		}
	}
	return false
}
