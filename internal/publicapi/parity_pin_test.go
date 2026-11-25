package publicapi

import (
	"encoding/json"
	"strings"
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
		Methods       int               `json:"methods"`
		Overrides     map[string]string `json:"overrides"`
		KnowledgeBase struct {
			Source  string   `json:"source"`
			Methods []string `json:"methods"`
		} `json:"knowledgeBase"`
		LedgerDates struct {
			Source  string   `json:"source"`
			Methods []string `json:"methods"`
		} `json:"ledgerDates"`
		AIDialogs struct {
			Source  string   `json:"source"`
			Methods []string `json:"methods"`
		} `json:"aiDialogs"`
		MemorySave struct {
			Source  string   `json:"source"`
			Methods []string `json:"methods"`
		} `json:"memorySave"`
	}
	if err := json.Unmarshal(contracts.ParitySource, &source); err != nil {
		t.Fatal(err)
	}
	pinned := map[string]bool{}
	for _, m := range doc.Methods {
		pinned[m.Name] = true
	}
	if len(doc.Methods) != 165 || source.Methods != len(doc.Methods) || len(pinned) != len(doc.Methods) {
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
	// The knowledge-base reads are pinned, deliberately and only they, from the sim-api branch that
	// serves them first; the next whole-projection re-pin from develop drops this record.
	if source.KnowledgeBase.Source == "" || len(source.KnowledgeBase.Methods) != 8 {
		t.Fatalf("knowledge-base record: %+v", source.KnowledgeBase)
	}
	for _, method := range source.KnowledgeBase.Methods {
		if !pinned[method] || ParityMethods[method].Domain != "knowledge" && method != "ontology/count" {
			t.Errorf("knowledge-base method %s is not pinned as a knowledge read", method)
		}
	}
	// transfer/filter and its count are re-projected, deliberately and only they, for dateBy.
	if source.LedgerDates.Source == "" || strings.Join(source.LedgerDates.Methods, ",") != "transfer/filter,transfer/filter/count" ||
		!strings.Contains(string(contracts.Parity), `"name": "dateBy"`) {
		t.Fatalf("ledger-dates record: %+v", source.LedgerDates)
	}
	// actor/search/list is re-projected, deliberately and only it, for includeAiDialogs (the
	// server-side AI dialog filter); the knowledge reads carry it from the same document.
	if source.AIDialogs.Source == "" || strings.Join(source.AIDialogs.Methods, ",") != "actor/search/list" ||
		strings.Count(string(contracts.Parity), `"name": "includeAiDialogs"`) != 5 {
		t.Fatalf("AI-dialogs record: %+v", source.AIDialogs)
	}
	// actor/memory/save is pinned, deliberately and only it, for memory_save's server-side merge.
	if source.MemorySave.Source == "" || strings.Join(source.MemorySave.Methods, ",") != "actor/memory/save" || !pinned["actor/memory/save"] {
		t.Fatalf("memory-save record: %+v", source.MemorySave)
	}
	if len(source.Overrides) != 0 {
		t.Fatalf("%d methods pinned from other branches; the whole projection comes from one sim-api revision", len(source.Overrides))
	}
}

// memory_save sends actor/memory/save only the keys its pinned contract declares.
func TestMemorySaveWireMatchesThePin(t *testing.T) {
	var doc struct {
		Methods []struct {
			Name   string `json:"name"`
			Params []struct {
				Name   string          `json:"name"`
				Schema json.RawMessage `json:"schema"`
			} `json:"params"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(contracts.Parity, &doc); err != nil {
		t.Fatal(err)
	}
	params := map[string]json.RawMessage{}
	for _, m := range doc.Methods {
		if m.Name == "actor/memory/save" {
			for _, p := range m.Params {
				params[p.Name] = p.Schema
			}
		}
	}
	keys := func(raw json.RawMessage) map[string]bool {
		var s struct {
			Items struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"items"`
		}
		_ = json.Unmarshal(raw, &s)
		out := map[string]bool{}
		for k := range s.Items.Properties {
			out[k] = true
		}
		return out
	}
	for _, name := range []string{"accId", "actorId", "notes", "relations", "forget"} {
		if params[name] == nil {
			t.Errorf("param %s is not in the pin", name)
		}
	}
	for _, field := range []string{"category", "text"} {
		if !keys(params["notes"])[field] {
			t.Errorf("notes[].%s is not in the pin", field)
		}
	}
	for _, field := range []string{"type", "actorId", "title"} {
		if !keys(params["relations"])[field] {
			t.Errorf("relations[].%s is not in the pin", field)
		}
	}
}
