package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// referenceSnapshot is the public input projection needed to reproduce the embedded docs.
// Private repository access is needed only to refresh/verify the pinned source projection.
type referenceSnapshot struct {
	Source     string                  `json:"source"`
	Protocol   string                  `json:"protocol"`
	PageSchema map[string]any          `json:"pageSchema"`
	Manifest   map[string]manifestType `json:"manifest"`
	Help       map[string]helpEntry    `json:"help"`
	Examples   []snapshotExample       `json:"examples"`
}

type snapshotExample struct {
	ID      string `json:"id"`
	Config  bool   `json:"config"`
	Page    string `json:"page"`
	Writeup string `json:"writeup"`
}

func snapshot(in *inputs, head string) ([]byte, error) {
	s := referenceSnapshot{Source: head, Protocol: in.protocol, PageSchema: in.pageSchema, Manifest: in.manifest, Help: in.help}
	for _, ex := range in.examples {
		s.Examples = append(s.Examples, snapshotExample{ex.id, ex.config, string(ex.page), ex.writeup})
	}
	b, err := json.MarshalIndent(s, "", "  ")
	return append(b, '\n'), err
}

// checkSnapshot regenerates from committed inputs on every machine, including fork CI.
func checkSnapshot(dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "inputs.json")) // #nosec G304 -- repository-owned generated inputs.
	if err != nil {
		return err
	}
	var s referenceSnapshot
	if err = json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if !headerRevs.MatchString(s.Source) || s.Protocol == "" || len(s.Examples) != len(exampleIDs) {
		return fmt.Errorf("invalid reference inputs: refresh with make generate")
	}
	in := &inputs{protocol: s.Protocol, pageSchema: s.PageSchema, manifest: s.Manifest, help: s.Help}
	for i, ex := range s.Examples {
		if ex.ID != exampleIDs[i] {
			return fmt.Errorf("unexpected reference example %q", ex.ID)
		}
		in.examples = append(in.examples, example{ex.ID, ex.Config, []byte(ex.Page), ex.Writeup})
	}
	files, err := renderInputs(s.Source, in)
	if err != nil {
		return err
	}
	return compareFiles(dir, files)
}

func compareFiles(dir string, files map[string][]byte) error {
	for name, body := range files {
		committed, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- names are fixed by renderInputs.
		if err != nil {
			return err
		}
		if !bytes.Equal(committed, body) {
			return fmt.Errorf("generated CDU reference drift in %s: run make generate", name)
		}
	}
	return nil
}
