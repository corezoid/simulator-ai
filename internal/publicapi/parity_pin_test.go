package publicapi

import (
	"encoding/json"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
)

// TestParitySourcePin keeps the pinned projection, its one-revision source record and the closed method set in step.
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
	if len(doc.Methods) != 107 || source.Methods != len(doc.Methods) || len(pinned) != len(doc.Methods) {
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
	if len(source.Overrides) != 0 {
		t.Fatalf("%d methods pinned from other branches; the whole projection comes from one sim-api revision", len(source.Overrides))
	}
}
