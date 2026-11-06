//go:build race

// Package testutil exposes build instrumentation to test-only performance assertions.
package testutil

// RaceEnabled separates wall-clock performance budgets from race instrumentation.
// Functional assertions and allocation limits still run in both CI jobs.
const RaceEnabled = true
