package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// exampleIDs are the example pages embedded, one per everyday shape of a Smart Form: a form with
// validation and submit, a multi-step form, a server-driven table, a Row/Col/Card/Tabs layout, a
// list rendered by a loop, and a stored config with routes. They come from the ui-kit's
// docs/cdu-examples (CI parses each and checks every node's props against the catalog), not from
// apps/cdu-lab/src/fixtures, which are block benches without a Form, a Table or a form.submit.
var exampleIDs = []string{"E12-conditional-validation", "E11-wizard", "E22-data-grid", "E10-workspace", "E03-loop-list", "R01-page-routing"}

// helpEntry is one entry of the ui-kit help bundle (cdu-help.json): the text the Smart Form Editor
// shows in its hints.
type helpEntry struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

// manifestType is what the reference takes from a catalog manifest type: where its children go and
// the prop examples the editor offers. cdu/catalog/get does not return either.
type manifestType struct {
	Kind   string `json:"kind"`
	Editor struct {
		Slots    []json.RawMessage `json:"slots"`
		Examples []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Props       json.RawMessage `json:"props"`
		} `json:"examples"`
	} `json:"editor"`
}

type example struct {
	id      string
	config  bool // a stored page config (R##), not a page/render payload
	page    []byte
	writeup string
}

type inputs struct {
	protocol   string
	pageSchema map[string]any
	manifest   map[string]manifestType
	help       map[string]helpEntry
	examples   []example
}

func readInputs(sim, kit source) (*inputs, error) {
	in := &inputs{}
	raw, err := sim.read("internal/cdu/catalog/supported.json")
	if err != nil {
		return nil, err
	}
	var policy struct {
		SupportedVersions []string `json:"supportedVersions"`
	}
	if err = json.Unmarshal(raw, &policy); err != nil || len(policy.SupportedVersions) == 0 {
		return nil, errors.New("sim-api supported.json names no protocol")
	}
	in.protocol = policy.SupportedVersions[len(policy.SupportedVersions)-1] // oldest first
	contracts := "internal/cdu/catalog/contracts/" + in.protocol + "/"
	if raw, err = sim.read(contracts + "page-config.schema.json"); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &in.pageSchema); err != nil {
		return nil, fmt.Errorf("page-config.schema.json: %w", err)
	}
	if raw, err = sim.read(contracts + "catalog.manifest.json"); err != nil {
		return nil, err
	}
	var manifest struct {
		Types map[string]manifestType `json:"types"`
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("catalog.manifest.json: %w", err)
	}
	in.manifest = manifest.Types
	if raw, err = kit.read("packages/cdu-catalog/cdu-help.json"); err != nil {
		return nil, err
	}
	var help struct {
		Entries map[string]helpEntry `json:"entries"`
	}
	if err = json.Unmarshal(raw, &help); err != nil {
		return nil, fmt.Errorf("cdu-help.json: %w", err)
	}
	in.help = help.Entries
	for _, id := range exampleIDs {
		ex := example{id: id, config: strings.HasPrefix(id, "R")}
		path := "docs/cdu-examples/pages/" + id + ".page.json"
		if ex.config {
			path = "docs/cdu-examples/configs/" + id + ".config.json"
		}
		if ex.page, err = kit.read(path); err != nil {
			return nil, err
		}
		if raw, err = kit.read("docs/cdu-examples/" + id + ".md"); err != nil {
			return nil, err
		}
		ex.writeup = string(raw)
		in.examples = append(in.examples, ex)
	}
	return in, nil
}

// helpLines is a help entry's summary and each paragraph of its body, one line each.
func (in *inputs) helpLines(id string) []string {
	e, ok := in.help[id]
	if !ok || e.Summary == "" {
		return nil
	}
	lines := []string{oneLine(e.Summary)}
	return append(lines, paragraphs(e.Body)...)
}

// paragraphs splits text at blank lines, each paragraph joined into one line; a fenced block keeps
// its lines (a drawn tree), without the fences.
func paragraphs(text string) []string {
	var out, para []string
	fenced := false
	flush := func() {
		if p := oneLine(strings.Join(para, " ")); p != "" {
			out = append(out, p)
		}
		para = nil
	}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "```"):
			flush()
			fenced = !fenced
		case fenced:
			if line = strings.TrimRight(line, " "); line != "" {
				out = append(out, line)
			}
		case strings.TrimSpace(line) == "":
			flush()
		default:
			para = append(para, line)
		}
	}
	flush()
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
