package publicapi

import (
	"encoding/json"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
)

// TestParitySourcePin keeps the pinned projection, its source record and the closed method set in step.
func TestParitySourcePin(t *testing.T) {
	var doc struct {
		Methods []struct {
			Name string `json:"name"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(contracts.Parity, &doc); err != nil {
		t.Fatal(err)
	}
	var source struct {
		Methods   int               `json:"methods"`
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal(contracts.ParitySource, &source); err != nil {
		t.Fatal(err)
	}
	pinned := map[string]bool{}
	for _, m := range doc.Methods {
		pinned[m.Name] = true
	}
	if len(doc.Methods) != 41 || source.Methods != len(doc.Methods) || len(pinned) != len(doc.Methods) {
		t.Fatalf("pinned %d methods (%d distinct), source says %d", len(doc.Methods), len(pinned), source.Methods)
	}
	for method := range ParityMethods {
		if !pinned[method] {
			t.Errorf("%s is callable but not pinned", method)
		}
		if _, err := contracts.ParityInputSchema(method); err != nil {
			t.Error(err)
		}
	}
	for _, method := range []string{"form/tree/get", "edge/by-node/get", "edge_type/list"} {
		if source.Overrides[method] == "" || !pinned[method] {
			t.Errorf("%s has no recorded source branch", method)
		}
	}
	if len(source.Overrides) != 3 {
		t.Fatalf("%d overrides recorded, want 3", len(source.Overrides))
	}
}
