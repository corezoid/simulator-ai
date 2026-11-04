package mcpserver

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/corezoid/simulator-ai/internal/simulation"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// The simulation tools (legacy simulationCheck, simulationRun, simulationSnapshot) run the model
// format of the simulator-ai-simulate skill in this process. They only read Simulator.

const (
	simulationInputs = "Model: `model` (YAML text, the format of the simulator-ai-simulate skill). Scenarios: `scenarios` " +
		"(YAML list); without them one scenario `base` runs. Graph: `layerId` (read live: actors, links, account values) " +
		"or `graph` (the layer YAML of simulation_snapshot_get, possibly edited). Reads only: nothing is written to Simulator."
	simulationProps = `"model":{"type":"string","minLength":1,"description":"Model YAML text."},` +
		`"scenarios":{"type":"string","description":"Scenarios YAML text: a list of {name, params, set, horizon, seed}."},` +
		`"layerId":{"type":"string","minLength":1,"maxLength":255,"description":"Layer actor id to read live."},` +
		`"graph":{"type":"string","description":"Layer YAML from simulation_snapshot_get; layerId wins when both are sent."},` +
		`"period":{"type":"string","maxLength":32,"description":"With layerId: account values are the turnover of this last period (e.g. 30d) instead of the balance."}`
	reportList     = `{"type":"array","items":{"type":"string"}}`
	checkOutput    = `{"type":"object","properties":{"ok":{"type":"boolean"},"errors":` + reportList + `,"warnings":` + reportList + `},"required":["ok","errors","warnings"]}`
	runOutput      = `{"type":"object","properties":{"ok":{"type":"boolean"},"message":{"type":"string"},"errors":` + reportList + `,"warnings":` + reportList + `,"graph":{"type":"string"},"source":{"type":"object"},"runs":{"type":"integer"},"table":{"type":"string"},"scenarios":{"type":"array","items":{"type":"object"}}},"required":["ok"]}`
	snapshotOutput = `{"type":"object","properties":{"graph":{"type":"string"},"source":{"type":"object"},"yaml":{"type":"string"}},"required":["graph","source","yaml"]}`
)

// addSimulation registers the three read-only simulation tools.
func addSimulation(s *server.MCPServer, c *publicapi.Client) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(checkOutput)), "simulation_check", "Check a behaviour model before running it: "+
		"unknown actions or functions, missing fields, typos in params, refs that match no actor, events nobody handles, handlers never "+
		"triggered, add on a conserved account, goals over unknown metrics. Returns errors (a run would fail) and warnings. "+simulationInputs,
		`{"type":"object","properties":{`+simulationProps+`},"required":["model"],"additionalProperties":false}`, false,
		func(ctx context.Context, in simulation.Input) (map[string]any, error) {
			return simulation.CheckTool(ctx, c, in)
		})
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(runOutput)), "simulation_run", "Simulate a Simulator graph: run behaviour rules "+
		"in model time for each scenario and compare metrics, in memory. The model is checked first; a model with errors does not run "+
		"(ok false with the errors). With runs > 1 each scenario runs in that many random worlds and the result is the median with the "+
		"10-90 % range, plus the share of runs meeting each goal, over completed runs only: runs that failed or stopped before the horizon "+
		"are counted apart and must be reported. Report a single run only when the model has no randomness. "+simulationInputs,
		`{"type":"object","properties":{`+simulationProps+`,`+
			`"scenario":{"type":"string","description":"Comma-separated scenario names to run (default: all)."},`+
			`"runs":{"type":"integer","minimum":1,"maximum":`+strconv.Itoa(simulation.MaxRuns)+`,"description":"Runs per scenario with different random seeds (default 1)."},`+
			`"goals":{"type":"string","description":"Extra goals as a YAML/JSON map name -> condition over metrics, e.g. {no_leaves: 'leaves == 0'}."},`+
			`"logEvents":{"type":"integer","minimum":0,"maximum":`+strconv.Itoa(simulation.MaxLogEvents)+`,"description":"Include the first N processed events of each scenario (single runs; default 0)."},`+
			`"timeLimit":{"type":"string","maxLength":32,"description":"Wall-clock budget for the whole call, e.g. 30s or 5m (default 2m, at most 5m; it also bounds reading a live layer). When it runs out a single run ends with status stopped_by_time and many runs report the runs made so far."}`+
			`},"required":["model"],"additionalProperties":false}`, false,
		func(ctx context.Context, in simulation.RunInput) (map[string]any, error) {
			return simulation.RunTool(ctx, c, in)
		})
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(snapshotOutput)), "simulation_snapshot_get", "Read a layer as a simulation "+
		"sees it: its actors (type = template title, data), links (edge type names) and account values (credit minus debit per account "+
		"name and currency, fact accounts; a formula account keeps its formula next to its current value). Returns the layer YAML with "+
		"sim: sections (pass it, possibly edited, as graph to simulation_check and simulation_run) and a summary. Actors or accounts the "+
		"caller may not read are listed in source, as is a layer of more than 2000 actors, whose later actors get no accounts. Reads only: nothing is written to Simulator.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255,"description":"Layer actor id."},`+
			`"period":{"type":"string","maxLength":32,"description":"Account values = turnover of this last period (e.g. 30d) instead of the balance."}},`+
			`"required":["layerId"],"additionalProperties":false}`, false,
		func(ctx context.Context, in simulation.SnapshotInput) (map[string]any, error) {
			return simulation.SnapshotTool(ctx, c, in)
		})
}
