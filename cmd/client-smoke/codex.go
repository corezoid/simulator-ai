package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func codexSmoke(ctx context.Context, env []string, dir, root, executable string, e *evidence) error {
	version, err := run(ctx, env, dir, executable, "--version")
	if err != nil {
		return err
	}
	e.CodexVersion = strings.TrimSpace(string(version))
	if _, err = run(ctx, env, dir, executable, "plugin", "marketplace", "add", root, "--json"); err != nil {
		return err
	}
	installed, err := run(ctx, env, dir, executable, "plugin", "add", "simulator-ai@simulator-ai", "--json")
	if err != nil {
		return err
	}
	var install struct {
		InstalledPath string `json:"installedPath"`
	}
	if err = json.Unmarshal(installed, &install); err != nil {
		return err
	}
	skill, err := os.ReadFile(filepath.Join(install.InstalledPath, "skills", "simulator-ai-read", "SKILL.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(skill), "name: simulator-ai-read") {
		return errors.New("Codex installed skill missing")
	}
	e.CodexInstall = true
	p, err := startPeer(ctx, env, dir, executable, "app-server")
	if err != nil {
		return err
	}
	defer p.close()
	if err = p.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "simulator-ai-smoke", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return err
	}
	var skills struct {
		Data []struct {
			Skills []struct {
				Name    string `json:"name"`
				Enabled bool   `json:"enabled"`
			} `json:"skills"`
		} `json:"data"`
	}
	if err = p.call("skills/list", map[string]any{"cwds": []string{dir}, "forceReload": true}, &skills); err != nil {
		return err
	}
	loaded := map[string]bool{}
	for _, group := range skills.Data {
		for _, s := range group.Skills {
			if s.Enabled {
				loaded[s.Name] = true
			}
		}
	}
	e.CodexSkill = loaded["simulator-ai:simulator-ai-read"] && loaded["simulator-ai:simulator-ai-smart-forms"]
	if !e.CodexSkill {
		return errors.New("Codex skills/list did not load installed skill")
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = p.call("thread/start", map[string]any{"cwd": dir, "ephemeral": true, "approvalPolicy": "never", "sandbox": "read-only"}, &thread); err != nil {
		return err
	}
	var status struct {
		Data []struct {
			Name  string                     `json:"name"`
			Tools map[string]json.RawMessage `json:"tools"`
		} `json:"data"`
	}
	if err = p.call("mcpServerStatus/list", map[string]any{"threadId": thread.Thread.ID}, &status); err != nil {
		return err
	}
	serverName := ""
	expectedTools := 3
	if e.Profile == "smart-forms" {
		expectedTools = 16
	}
	for _, s := range status.Data {
		if s.Name == "simulator-ai" && len(s.Tools) == expectedTools {
			_, actor := s.Tools["actor_get"]
			_, form := s.Tools["form_get"]
			_, layer := s.Tools["layer_actors_list"]
			if actor && form && layer {
				serverName = s.Name
				e.CodexTools = len(s.Tools)
			}
		}
	}
	if serverName == "" {
		return errors.New("Codex did not load the exact installed MCP catalog")
	}
	var call struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "actor_get", "arguments": map[string]string{"actorId": "synthetic-actor"}}, &call); err != nil {
		return err
	}
	encoded, _ := json.Marshal(call)
	if call.IsError || !strings.Contains(string(encoded), "Client smoke fixture") {
		return errors.New("Codex tool did not return fixture metadata")
	}
	e.CodexRead = true
	if e.Profile == "smart-forms" {
		if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "application_create", "arguments": map[string]string{"operationId": "smoke-create-1", "ref": "smoke-app", "title": "Smoke app"}}, &call); err != nil {
			return err
		}
		encoded, _ = json.Marshal(call)
		if call.IsError || !strings.Contains(string(encoded), "synthetic-app") || strings.Contains(string(encoded), "credential-canary") {
			return errors.New("Codex Smart Form creation failed")
		}
		e.CodexCreate = true
	}
	return nil
}
