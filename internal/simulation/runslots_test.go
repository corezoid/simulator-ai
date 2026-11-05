package simulation

import (
	"context"
	"strings"
	"testing"
)

func TestRunSlotsCapAndRefuse(t *testing.T) {
	prev := runSlots
	t.Cleanup(func() { runSlots = prev })
	if cap(newRunSlots("")) != defaultRunSlots || cap(newRunSlots("x")) != defaultRunSlots || cap(newRunSlots(" 5 ")) != 5 || newRunSlots("0") != nil {
		t.Fatal("SIMULATOR_AI_MAX_CONCURRENT_SIMULATIONS is not read as documented")
	}
	runSlots = newRunSlots("")
	var releases []func()
	for i := 0; i < defaultRunSlots; i++ {
		rel, ok := acquireRunSlot()
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		releases = append(releases, rel)
	}
	if _, err := RunTool(context.Background(), offlineClient(t), RunInput{Input: sampleInput(t)}); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("a run beyond the cap: %v", err)
	}
	releases[0]()
	if _, err := RunTool(context.Background(), offlineClient(t), RunInput{Input: sampleInput(t), Scenario: "base"}); err != nil {
		t.Fatalf("a freed slot is not reusable: %v", err)
	}
	runSlots = newRunSlots("-1")
	for i := 0; i < 10; i++ {
		if _, ok := acquireRunSlot(); !ok {
			t.Fatal("0 or less must not cap simulations")
		}
	}
}
