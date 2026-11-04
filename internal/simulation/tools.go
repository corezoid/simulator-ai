package simulation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// The three simulation tools. They only read Simulator: a run plays the model in memory.

// Input is what every simulation tool takes: a model, scenarios and the graph, read live from a
// layer (LayerID, Period; it wins over Graph) or given as the layer YAML of a snapshot (Graph).
type Input struct {
	Model     string `json:"model"`
	Scenarios string `json:"scenarios,omitempty"`
	LayerID   string `json:"layerId,omitempty"`
	Graph     string `json:"graph,omitempty"`
	Period    string `json:"period,omitempty"`
}

// RunInput adds the run options of simulation_run.
type RunInput struct {
	Input
	Scenario  string `json:"scenario,omitempty"`
	Runs      int    `json:"runs,omitempty"`
	Goals     string `json:"goals,omitempty"`
	LogEvents int    `json:"logEvents,omitempty"`
	TimeLimit string `json:"timeLimit,omitempty"`
}

// SnapshotInput names the layer to read.
type SnapshotInput struct {
	LayerID string `json:"layerId"`
	Period  string `json:"period,omitempty"`
}

const (
	MaxRuns          = 1000
	MaxLogEvents     = 10_000
	DefaultTimeLimit = 2 * time.Minute
	MaxTimeLimit     = 5 * time.Minute // below an MCP client's call timeout
)

type inputs struct {
	modelSrc  []byte
	model     *Model
	graph     *Graph
	scenarios []Scenario
	rawScen   []*OMap
}

func loadInputs(ctx context.Context, c *publicapi.Client, in Input, needGraph bool) (*inputs, error) {
	out := &inputs{modelSrc: []byte(in.Model)}
	var err error
	if out.model, err = LoadModel(out.modelSrc); err != nil {
		return nil, err
	}
	if in.Scenarios != "" {
		if out.scenarios, out.rawScen, err = LoadScenarios([]byte(in.Scenarios)); err != nil {
			return nil, err
		}
	}
	if len(out.scenarios) == 0 {
		out.scenarios = []Scenario{ScenarioFrom(NewOMap())}
		out.rawScen = []*OMap{NewOMap()}
	}
	switch {
	case in.LayerID != "":
		period, err := parsePeriod(in.Period)
		if err != nil {
			return nil, fmt.Errorf("period: %w", err)
		}
		if out.graph, err = Snapshot(ctx, c, in.LayerID, period); err != nil {
			return nil, err
		}
	case in.Graph != "":
		if out.graph, err = LoadGraph([]byte(in.Graph)); err != nil {
			return nil, err
		}
		nameFormTypes(ctx, c, out.graph) // a layer YAML that names forms only by id
	case needGraph:
		return nil, errors.New("pass layerId (live) or graph (layer YAML from simulation_snapshot_get)")
	}
	return out, nil
}

// CheckTool checks a model against the graph and scenarios without running it.
func CheckTool(ctx context.Context, c *publicapi.Client, in Input) (map[string]any, error) {
	ins, err := loadInputs(ctx, c, in, false)
	if err != nil {
		return nil, err
	}
	rep := Check(ins.modelSrc, ins.graph, ins.rawScen)
	return map[string]any{"ok": rep.OK(), "errors": nonNil(rep.Errors), "warnings": nonNil(rep.Warnings)}, nil
}

// RunTool checks the model and runs the selected scenarios, once each or runs times.
func RunTool(ctx context.Context, c *publicapi.Client, in RunInput) (map[string]any, error) {
	release, ok := acquireRunSlot()
	if !ok {
		return nil, errors.New("busy: the maximum number of simulations is already running; retry in a minute")
	}
	defer release()
	limit, err := parseTimeLimit(in.TimeLimit)
	if err != nil {
		return nil, fmt.Errorf("timeLimit: %w", err)
	}
	// The live snapshot is read within the same budget as the runs.
	runCtx, cancel := context.WithTimeoutCause(ctx, limit, fmt.Errorf("time limit %s reached", limit))
	defer cancel()
	ins, err := loadInputs(runCtx, c, in.Input, true)
	if err != nil {
		if cause := context.Cause(runCtx); cause != nil && ctx.Err() == nil {
			return nil, fmt.Errorf("%w while reading the layer: %v", cause, err)
		}
		return nil, err
	}
	rep := Check(ins.modelSrc, ins.graph, ins.rawScen)
	if !rep.OK() {
		return map[string]any{"ok": false, "message": "model check failed: fix the errors, nothing was run",
			"errors": nonNil(rep.Errors), "warnings": nonNil(rep.Warnings)}, nil
	}
	selected, err := selectScenarios(ins.scenarios, in.Scenario)
	if err != nil {
		return nil, err
	}
	goals := NewOMap()
	if in.Goals != "" {
		v, err := parseYAML([]byte(in.Goals))
		if err != nil {
			return nil, fmt.Errorf("goals: %w", err)
		}
		if m, ok := v.(*OMap); ok {
			goals = m
		}
	}
	out := map[string]any{"ok": true, "graph": ins.graph.summary(), "source": toJSON(ins.graph.Source)}
	if len(rep.Warnings) > 0 {
		out["warnings"] = rep.Warnings
	}
	runs := min(max(in.Runs, 1), MaxRuns)
	if runs > 1 {
		var summaries []*Summary
		for _, sc := range selected {
			summaries = append(summaries, RunMany(runCtx, ins.graph, ins.model, sc, runs, nil, goals))
		}
		out["runs"], out["table"], out["scenarios"] = runs, SummaryTable(summaries), SummariesJSON(summaries)
	} else {
		logN := min(max(in.LogEvents, 0), MaxLogEvents)
		var results []*RunResult
		for _, sc := range selected {
			results = append(results, RunScenario(ins.graph, ins.model, sc, nil, RunOptions{Ctx: runCtx, LogLimit: logN}))
		}
		out["table"], out["scenarios"] = CompareTable(results), ResultsJSON(results, logN)
	}
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	return out, nil
}

// SnapshotTool reads a layer as the simulation sees it and returns its layer YAML.
func SnapshotTool(ctx context.Context, c *publicapi.Client, in SnapshotInput) (map[string]any, error) {
	period, err := parsePeriod(in.Period)
	if err != nil {
		return nil, fmt.Errorf("period: %w", err)
	}
	g, err := Snapshot(ctx, c, in.LayerID, period)
	if err != nil {
		return nil, err
	}
	b, err := GraphFile(g, in.LayerID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"graph": g.summary(), "source": toJSON(g.Source), "yaml": string(b)}, nil
}

func selectScenarios(all []Scenario, names string) ([]Scenario, error) {
	if names == "" {
		return all, nil
	}
	want := map[string]bool{}
	for _, n := range strings.Split(names, ",") {
		want[strings.TrimSpace(n)] = true
	}
	var selected []Scenario
	for _, sc := range all {
		if want[sc.Name] {
			selected = append(selected, sc)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no scenarios named %s", names)
	}
	return selected, nil
}

func parseTimeLimit(p string) (time.Duration, error) {
	if p == "" {
		return DefaultTimeLimit, nil
	}
	sec, err := parseDuration(p)
	if err != nil {
		return 0, err
	}
	d := time.Duration(sec) * time.Second
	if d <= 0 {
		return 0, errors.New("must be positive")
	}
	return min(d, MaxTimeLimit), nil
}

func parsePeriod(p string) (time.Duration, error) {
	if p == "" {
		return 0, nil
	}
	sec, err := parseDuration(p)
	if err != nil {
		return 0, err
	}
	return time.Duration(sec) * time.Second, nil
}

// nonNil keeps an empty list a list in the answer.
func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
