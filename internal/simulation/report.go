package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// Run reports: the comparison tables and JSON a simulation run returns.

func metricNames[T any](items []T, get func(T) []string) []string {
	set := map[string]bool{}
	for _, it := range items {
		for _, n := range get(it) {
			set[n] = true
		}
	}
	return sortedKeys(set)
}

// CompareTable renders one row per scenario.
func CompareTable(results []*RunResult) string {
	names := metricNames(results, func(r *RunResult) []string {
		out := make([]string, 0, len(r.Metrics))
		for k := range r.Metrics {
			out = append(out, k)
		}
		return out
	})
	var b strings.Builder
	b.WriteString("| scenario | status | steps | " + strings.Join(names, " | ") + " |\n|" + strings.Repeat("---|", 3+len(names)) + "\n")
	var errs []string
	for _, r := range results {
		cells := []string{r.Scenario, r.Status, fmt.Sprint(r.Steps)}
		for _, n := range names {
			v, ok := r.Metrics[n]
			if !ok {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, MetricText(v))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if r.Error != "" {
			errs = append(errs, "- "+r.Scenario+": "+r.Error)
		}
	}
	if len(errs) > 0 {
		b.WriteString("\nErrors:\n" + strings.Join(errs, "\n") + "\n")
	}
	return b.String()
}

// SummaryTable renders medians with ranges and goal shares.
func SummaryTable(summaries []*Summary) string {
	metrics := metricNames(summaries, func(s *Summary) []string {
		out := make([]string, 0, len(s.Metrics))
		for k := range s.Metrics {
			out = append(out, k)
		}
		return out
	})
	goals := metricNames(summaries, func(s *Summary) []string {
		out := make([]string, 0, len(s.Goals))
		for k := range s.Goals {
			out = append(out, k)
		}
		return out
	})
	head := append([]string{"scenario", "runs completed/total"}, metrics...)
	for _, g := range goals {
		head = append(head, "goal: "+g)
	}
	var b strings.Builder
	b.WriteString("| " + strings.Join(head, " | ") + " |\n|" + strings.Repeat("---|", len(head)) + "\n")
	for _, s := range summaries {
		ok := fmt.Sprintf("%d/%d", s.Completed, s.Runs)
		var notOK []string
		if s.Stopped > 0 {
			notOK = append(notOK, fmt.Sprintf("%d stopped", s.Stopped))
		}
		if s.Failed > 0 {
			notOK = append(notOK, fmt.Sprintf("%d failed", s.Failed))
		}
		if len(notOK) > 0 {
			ok += " (" + strings.Join(notOK, ", ") + ")"
		}
		cells := []string{s.Scenario, ok}
		for _, m := range metrics {
			if st := s.Metrics[m]; st != nil {
				cells = append(cells, st.Short())
			} else {
				cells = append(cells, "")
			}
		}
		for _, g := range goals {
			v := s.Goals[g]
			if v[1] == 0 {
				cells = append(cells, "no data")
			} else {
				cells = append(cells, fmt.Sprintf("%d%% (%d/%d)", (100*v[0]+v[1]/2)/v[1], v[0], v[1]))
			}
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	b.WriteString("\nNumbers: median (10th–90th percentile) over completed runs. Goal: share of completed runs where it holds; failed runs and runs stopped before the horizon are not counted.\n")
	for _, s := range summaries {
		for i, e := range s.Errors {
			if i == 3 {
				break
			}
			b.WriteString("- " + s.Scenario + ": " + e + "\n")
		}
	}
	return b.String()
}

// ResultsJSON is one entry per single-run scenario, with its first logN events.
func ResultsJSON(results []*RunResult, logN int) []map[string]any {
	var out []map[string]any
	for _, r := range results {
		metrics := map[string]any{}
		for k, v := range r.Metrics {
			metrics[k] = toJSON(v)
		}
		entry := map[string]any{"scenario": r.Scenario, "status": r.Status, "steps": r.Steps, "model_time": r.ModelTime,
			"pending_events": r.PendingEvents, "metrics": metrics}
		if r.Error != "" {
			entry["error"] = r.Error
		}
		if logN > 0 {
			n := logN
			if n > len(r.Log) {
				n = len(r.Log)
			}
			entry["events"] = r.Log[:n]
		}
		out = append(out, entry)
	}
	return out
}

// SummariesJSON is one entry per many-run scenario: metric statistics and goal shares.
func SummariesJSON(summaries []*Summary) []map[string]any {
	var out []map[string]any
	for _, s := range summaries {
		metrics := map[string]any{}
		for k, st := range s.Metrics {
			metrics[k] = map[string]any{"n": st.N, "mean": numString(st.Mean), "p10": numString(st.P10),
				"p50": numString(st.P50), "p90": numString(st.P90), "min": numString(st.Min), "max": numString(st.Max)}
		}
		goals := map[string]any{}
		keys := make([]string, 0, len(s.Goals))
		for k := range s.Goals {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			goals[k] = map[string]int{"held": s.Goals[k][0], "evaluated": s.Goals[k][1]}
		}
		entry := map[string]any{"scenario": s.Scenario, "runs": s.Runs, "completed": s.Completed,
			"failed": s.Failed, "stopped": s.Stopped, "metrics": metrics, "goals": goals, "errors": s.Errors}
		if s.Note != "" {
			entry["note"] = s.Note
		}
		out = append(out, entry)
	}
	return out
}
