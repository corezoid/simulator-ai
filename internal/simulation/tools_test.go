package simulation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// offlineClient answers no call: tools that take the graph as text must not need one.
func offlineClient(t *testing.T) *publicapi.Client {
	t.Helper()
	c, err := publicapi.New(publicapi.Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console-full", Authorization: "Simulator test"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sampleInput(t *testing.T) Input {
	t.Helper()
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("testdata/conformance/sample01", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return Input{Model: read("model.yaml"), Scenarios: read("scenarios.yaml"), Graph: read("graph.yaml")}
}

func TestToolsRunOfflineFromGraphText(t *testing.T) {
	ctx, c, in := context.Background(), offlineClient(t), sampleInput(t)
	out, err := CheckTool(ctx, c, in)
	if err != nil || out["ok"] != true {
		t.Fatalf("check: %v %v", out, err)
	}
	out, err = RunTool(ctx, c, RunInput{Input: in, Scenario: "base,price80_ai"})
	table, _ := out["table"].(string)
	if err != nil || !strings.Contains(table, "| base | completed | 6 | 1 | 0 | 80 |") || !strings.Contains(table, "| 165 |") {
		t.Fatalf("run table:\n%s\n%v %v", table, out, err)
	}
	out, err = RunTool(ctx, c, RunInput{Input: in, Scenario: "b_random_80", Runs: 50, Goals: "{margin_120: 'margin == 120'}"})
	table, _ = out["table"].(string)
	if err != nil || !strings.Contains(table, "50/50") || !strings.Contains(table, "goal: margin_120") || !strings.Contains(table, "100% (50/50)") {
		t.Fatalf("runs table:\n%s\n%v", table, err)
	}
}

func TestRunRefusesBrokenModelAndMissingGraph(t *testing.T) {
	ctx, c, in := context.Background(), offlineClient(t), sampleInput(t)
	in.Model = strings.Replace(in.Model, "      - release: {target: self}\n", "      - release: {target: self}\n      - explode: {}\n", 1)
	out, err := RunTool(ctx, c, RunInput{Input: in})
	if err != nil || out["ok"] != false || !strings.Contains(strings.Join(out["errors"].([]string), " "), "explode") {
		t.Fatalf("broken model ran: %v %v", out, err)
	}
	in = sampleInput(t)
	in.Graph = ""
	if _, err := RunTool(ctx, c, RunInput{Input: in}); err == nil || !strings.Contains(err.Error(), "layerId") {
		t.Errorf("a run without a graph: %v", err)
	}
	if _, err := RunTool(ctx, c, RunInput{Input: sampleInput(t), Scenario: "nope"}); err == nil {
		t.Error("an unknown scenario name ran")
	}
}
