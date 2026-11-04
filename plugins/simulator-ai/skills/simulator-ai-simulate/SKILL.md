---
name: simulator-ai-simulate
description: Simulate a Simulator graph ("what if") with the public Simulator AI tools (profile console-full) - describe how actors behave, change parameters (price, capacity, delivery time, growth, AI share), play model time forward and compare scenarios on a layer. Any graph works (a company, a process, infrastructure, a supply chain), and accounts can hold any quantity (money, items, hours, slots). Use for "what if", "simulate", "forecast", "compare scenarios", "where is the bottleneck", "що буде якщо", "симуляція", "змоделюй", "прогноз", "что будет если", "смоделируй", "сравни сценарии", or when the user gives a behaviour model or scenarios to check or run. Read-only - nothing is written to Simulator.
---

Use the installed `simulator-ai` catalog with profile `console-full`; plain `console` has no
simulation tools. The engine runs inside the plugin: it reads a layer (actors, links, account
values), runs behaviour rules in model time for each scenario and returns metrics. It never
writes to Simulator.

Read [the model format](references/model-format.md) before writing a model.

## 1. Pick the graph - always ask

- If the context names an active layer, offer it first; otherwise help the user find one
  (`actors_search`, `layer_placements_list`, `layer_stats_get`) and confirm which.
- Look at what is there before modelling: `simulation_snapshot_get {layerId, period?}` returns the
  layer YAML with every actor, link and account value the simulation will see, and a summary.
  Actor types are template titles; `source` lists what the caller may not read.
- Account values that accumulate over time (bills, turnover) need `period` (e.g. `30d`): values
  become the turnover of that last period. Check for accounts that are breakdowns of the same total
  (by service, region, operation): summing them counts one bill many times.
- A formula account carries its current value and its formula; the run treats the value as fixed.

## 2. Write the model with the user

Ask only what changes the result: which actors behave and how, what the scenarios change, the
horizon, what to measure. Show the rules in plain words before running, and label assumptions
(durations, probabilities, growth rates) as assumptions.

A model is YAML (format §5): `value_types`, `refs`, `params`, `horizon`, `initial_events`,
`behaviors.<actor type>.<event>: [actions]`, `metrics`, `goals`, optional helper `actors` (people or
resources that are not on the graph). Account names are the Simulator account names; their value
type is the currency name.

Rules of thumb:
- Money and other quantities that only move between actors: `conserved: true` and `transfer`.
  Things that only grow (hours worked, missed sales): not conserved, `add`.
- Capacity (people, machines, slots): an integer conserved account on a resource actor plus
  `enqueue` / `release`.
- Processes drawn as graphs: steps are actors, transitions are links; an application walks with
  `pick(where(actor(self.at).children(), ...))`.
- Never write a key named `on:` (YAML reads it as true); use `actor:`.

## 3. Check, then run

    simulation_check {model, scenarios?, layerId | graph}

`simulation_run` checks the model too and refuses one with errors (`ok: false` and the errors).
Warnings (an event nobody handles, a param typo) usually mean the model does not do what the user
thinks.

    simulation_run {model, scenarios?, layerId | graph, period?, scenario?, runs?, goals?, logEvents?, timeLimit?}

- Pass `layerId` to read the layer live, or the snapshot YAML (possibly edited) as `graph` to rerun
  on exactly what was inspected.
- The model has randomness (`rand()`, `pick`, `chance`, a `decide` without a `rule`): never report
  a single run. Use `runs: 100` and report the median, the 10-90 % range and the share of runs
  meeting each goal.
- These numbers cover completed runs only (`runs completed/total` in the table). When runs failed
  or stopped before the horizon (`stopped_by_time`, `stopped_by_limit`), say how many and why (the
  lines under the table): a scenario that stops early is not cheaper or faster, it is unfinished.
  Raise `timeLimit` or shorten the horizon rather than compare it.
- A warning that the graph holds numbers as text (a v2.9.0 snapshot) means its numbers are
  strings: take a new one with `simulation_snapshot_get` before trusting any result.
- No randomness: one run is exact; say so.
- `logEvents: 30` returns the first processed events - use it to explain why a result happened.

## 4. Explain

- Lead with the answer, then the comparison table. Name the scenario and metric behind every
  number; separate results from assumptions.
- A range from `runs` is spread over the model's random outcomes, not a confidence interval for
  the real world.
- The engine computes the consequences of the rules exactly (decimal arithmetic, conserved totals
  checked on every step); whether the rules match reality is a separate question, best answered by
  comparing with history.
