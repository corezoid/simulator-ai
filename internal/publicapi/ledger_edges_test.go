package publicapi

import (
	"strings"
	"testing"
)

// graph_build refuses a money edge in its own validation, before a layer or an actor is written.
func TestLedgerEdgesRefusedByName(t *testing.T) {
	for _, name := range []string{"transfersAmount_n1_5", "transfersAmount", " transfersFormField_x "} {
		if err := LedgerEdgeError(name); err == nil || !strings.Contains(err.Error(), "transfer") {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"", "hierarchy", "directedInteraction", "mytransfersAmount"} {
		if err := LedgerEdgeError(name); err != nil {
			t.Errorf("%q refused: %v", name, err)
		}
	}
	in := GraphBuildInput{Nodes: gridNodes("n", 2, GraphPoint{}), Edges: []BuildEdge{{From: "n0", To: "n1", EdgeType: "transfersAmount_n1_5"}}}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "drawn by the platform from a transfer") {
		t.Fatal(err)
	}
}
