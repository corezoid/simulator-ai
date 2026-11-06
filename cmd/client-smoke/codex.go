package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/corezoid/simulator-ai/internal/mcpserver"
	"github.com/corezoid/simulator-ai/internal/publicapi"
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
		return errors.New("codex installed skill missing")
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
		return errors.New("codex skills/list did not load installed skill")
	}
	if strings.HasPrefix(e.Profile, "reactions-") && !loaded["simulator-ai:simulator-ai-reactions"] {
		return errors.New("codex did not load reactions skill")
	}
	if strings.HasPrefix(e.Profile, "graph-") && !loaded["simulator-ai:simulator-ai-graph"] {
		return errors.New("codex did not load graph skill")
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
	// Compare the installed client's catalog to this checkout. Profile safety is tested
	// independently in mcpserver; this smoke verifies plugin installation and loading.
	cfg := publicapi.Config{Origin: "http://127.0.0.1", Workspace: "synthetic-workspace", APIKey: "synthetic-client-key", Profile: e.Profile}
	if e.Profile == "reactions-actor" {
		cfg.Profile, cfg.ActorID = "reactions-read", "synthetic-actor"
	} else if e.Profile == "graph-actor" {
		cfg.Profile, cfg.ActorID = "graph-read", "synthetic-layer"
	}
	client, err := publicapi.New(cfg)
	if err != nil {
		return err
	}
	expectedTools := mcpserver.New(client).ListTools()
	for _, s := range status.Data {
		if s.Name != "simulator-ai" || len(s.Tools) != len(expectedTools) {
			continue
		}
		matches := true
		for name := range expectedTools {
			if _, ok := s.Tools[name]; !ok {
				matches = false
			}
		}
		if matches {
			serverName = s.Name
			e.CodexTools = len(s.Tools)
		}
	}
	if serverName == "" {
		return errors.New("codex did not load the exact installed MCP catalog")
	}
	var call struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if e.Profile != "reactions-actor" && e.Profile != "graph-actor" {
		args := map[string]any{"actorId": "synthetic-actor"}
		if e.Profile == "read" || e.Profile == "graph-read" {
			args["fields"] = []string{"id", "title", "description", "formId", "status", "ref"}
		}
		if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "actor_get", "arguments": args}, &call); err != nil {
			return err
		}
		encoded, _ := json.Marshal(call)
		if call.IsError || !strings.Contains(string(encoded), "Client smoke fixture") {
			return errors.New("codex tool did not return fixture metadata")
		}
	}
	e.CodexRead = true
	if strings.HasPrefix(e.Profile, "reactions-") {
		args := map[string]string{}
		if e.Profile == "reactions-read" {
			args["actorId"] = "synthetic-actor"
		}
		if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "reaction_stats_get", "arguments": args}, &call); err != nil {
			return err
		}
		var stats map[string]int64
		if call.IsError || json.Unmarshal(call.StructuredContent, &stats) != nil || len(stats) != 2 || stats["comment"] != 2 {
			return errors.New("codex reaction counts failed")
		}
		if value, ok := stats["view"]; !ok || value != 0 {
			return errors.New("codex reaction zero count lost")
		}
		e.CodexReactionStats = true
	}
	if strings.HasPrefix(e.Profile, "graph-") {
		args := map[string]string{}
		if e.Profile == "graph-read" {
			args["actorId"] = "synthetic-layer"
		}
		if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "graph_layer_stats_get", "arguments": args}, &call); err != nil {
			return err
		}
		var counts map[string]int64
		if call.IsError || json.Unmarshal(call.StructuredContent, &counts) != nil || len(counts) != 6 {
			return errors.New("codex layer counts failed")
		}
		for key, want := range map[string]int64{"nodes": 7, "edges": 8, "holes": 1, "holesClosed": 2, "edgeHoles": 3, "edgeHolesClosed": 0} {
			if value, ok := counts[key]; !ok || value != want {
				return errors.New("codex layer count missing or changed")
			}
		}
		e.CodexLayerStats = true
	}
	if e.Profile == "graph-read" {
		for offset := 0; offset < 2; offset++ {
			args := map[string]any{"isSystem": false, "filter": "id,name,userId", "limit": 1, "offset": offset}
			if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "graph_edge_types_list", "arguments": args}, &call); err != nil {
				return err
			}
			var page struct {
				List []map[string]json.RawMessage `json:"list"`
			}
			if call.IsError || json.Unmarshal(call.StructuredContent, &page) != nil || page.List == nil || len(page.List) != 1-offset {
				return errors.New("codex edge type paging failed")
			}
			if offset == 0 {
				row := page.List[0]
				if len(row) != 3 || string(row["id"]) != "2" || string(row["name"]) != `"Synthetic custom"` || string(row["userId"]) != "null" {
					return errors.New("codex edge type projection/null failed")
				}
			}
		}
		e.CodexEdgeTypes = true
	}
	if e.Profile == "smart-forms" {
		if err = p.call("mcpServer/tool/call", map[string]any{"threadId": thread.Thread.ID, "server": serverName, "tool": "application_create", "arguments": map[string]string{"operationId": "smoke-create-1", "ref": "smoke-app", "title": "Smoke app"}}, &call); err != nil {
			return err
		}
		encoded, _ := json.Marshal(call)
		if call.IsError || !strings.Contains(string(encoded), "synthetic-app") || strings.Contains(string(encoded), "credential-canary") {
			return errors.New("codex Smart Form creation failed")
		}
		e.CodexCreate = true
	}
	return nil
}
