package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// renderExamples writes each example as the Smart Form files it becomes, pretty-printed in the
// source's key order so its lines grep well, after the write-up's goal and mechanisms.
func renderExamples(head string, in *inputs) ([]byte, error) {
	d := &doc{}
	d.add("# CDU v2 example pages", head,
		"Complete pages, CI-checked against the catalog: patterns to adapt (ids, texts, data), not to paste whole. A page/render payload becomes the files pages/<page>/config ({schemaVersion, id, title?, tree, bindings?}), pages/<page>/viewModel and pages/<page>/locale; each example below is already split that way.",
		"Sections: "+strings.Join(exampleIDs, ", ")+". Read one with smart_form_reference_read {doc: \"examples/<id>\"}.")
	for _, ex := range in.examples {
		title, goal, mechanisms := writeup(ex.writeup)
		heading := "## " + ex.id // the section id the agent reads it by
		if _, rest, ok := strings.Cut(title, " — "); ok {
			heading += " — " + rest
		}
		d.add("", heading)
		if goal != "" {
			d.add("Goal: " + goal)
		}
		if len(mechanisms) > 0 {
			d.add("Mechanisms:")
			d.add(mechanisms...)
		}
		files, err := exampleFiles(ex)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ex.id, err)
		}
		for _, f := range files {
			d.add(f.path+":", f.body)
		}
	}
	return d.bytes(), nil
}

type exampleFile struct{ path, body string }

// exampleFiles splits a page/render payload into the files of a Smart Form page; a stored config
// (R##) is one file already.
func exampleFiles(ex example) ([]exampleFile, error) {
	keys, values, err := orderedObject(ex.page)
	if err != nil {
		return nil, err
	}
	if ex.config {
		body, err := indent(ex.page)
		return []exampleFile{{"pages/<page>/config", body}}, err
	}
	var id string
	if err = json.Unmarshal(values["pageId"], &id); err != nil || id == "" {
		return nil, fmt.Errorf("no pageId")
	}
	config := &bytes.Buffer{}
	config.WriteString("{")
	sep := ""
	for _, k := range keys {
		value := values[k]
		switch k {
		case "pageId":
			k, value = "id", values["pageId"]
		case "schemaVersion", "title", "description", "tree", "bindings", "meta", "routes":
		default:
			continue // viewModel and locale are files of their own; the rest is render output
		}
		name, _ := json.Marshal(k)
		fmt.Fprintf(config, "%s%s:%s", sep, name, value)
		sep = ","
	}
	config.WriteString("}")
	var files []exampleFile
	for _, f := range []struct {
		path string
		raw  []byte
	}{{"pages/" + id + "/config", config.Bytes()}, {"pages/" + id + "/viewModel", values["viewModel"]}, {"pages/" + id + "/locale", values["locale"]}} {
		if len(f.raw) == 0 {
			continue
		}
		body, err := indent(f.raw)
		if err != nil {
			return nil, err
		}
		files = append(files, exampleFile{f.path, body})
	}
	return files, nil
}

// orderedObject is a JSON object's keys in source order with their raw values.
func orderedObject(raw []byte) ([]string, map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, fmt.Errorf("not a JSON object")
	}
	var keys []string
	values := map[string]json.RawMessage{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		k, _ := t.(string)
		var v json.RawMessage
		if err = dec.Decode(&v); err != nil {
			return nil, nil, err
		}
		keys = append(keys, k)
		values[k] = v
	}
	return keys, values, nil
}

func indent(raw []byte) (string, error) {
	out := &bytes.Buffer{}
	if err := json.Indent(out, bytes.TrimSpace(raw), "", "  "); err != nil {
		return "", err
	}
	return out.String(), nil
}

// writeup reads an example's write-up: its title (the "# " line), the **Goal.** paragraph and the
// bullets of its Mechanisms section, each on one line.
func writeup(md string) (title, goal string, mechanisms []string) {
	lines := strings.Split(md, "\n")
	inGoal, inMechanisms := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "# ") && title == "":
			title = strings.TrimPrefix(line, "# ")
		case strings.HasPrefix(trimmed, "**Goal.**"):
			inGoal, goal = true, strings.TrimSpace(strings.TrimPrefix(trimmed, "**Goal.**"))
		case inGoal && trimmed == "":
			inGoal = false
		case inGoal:
			goal += " " + trimmed
		case strings.HasPrefix(line, "## "):
			inMechanisms = strings.HasPrefix(line, "## Mechanisms")
		case inMechanisms && strings.HasPrefix(line, "- "):
			mechanisms = append(mechanisms, trimmed)
		case inMechanisms && trimmed != "" && len(mechanisms) > 0:
			mechanisms[len(mechanisms)-1] += " " + trimmed
		}
	}
	return title, oneLine(goal), mechanisms
}

// typeNote is what the components document adds to a type of the live catalog.
type typeNote struct {
	Purpose  string        `json:"purpose,omitempty"`
	Children string        `json:"children"`
	Examples []typeExample `json:"examples,omitempty"`
}

type typeExample struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Props       json.RawMessage `json:"props"`
}

// renderTypeNotes writes each type's purpose (the help bundle's type:<T> summary, what the editor
// shows), its children (the manifest's slots) and the manifest's prop examples.
func renderTypeNotes(head string, in *inputs) ([]byte, error) {
	notes := map[string]typeNote{}
	for name, t := range in.manifest {
		note := typeNote{Purpose: oneLine(in.help["type:"+name].Summary), Children: children(t)}
		for _, e := range t.Editor.Examples {
			props := &bytes.Buffer{}
			if err := json.Compact(props, e.Props); err != nil {
				return nil, fmt.Errorf("%s example %q: %w", name, e.Name, err)
			}
			note.Examples = append(note.Examples, typeExample{e.Name, oneLine(e.Description), props.Bytes()})
		}
		notes[name] = note
	}
	out, err := json.MarshalIndent(struct {
		Source string              `json:"source"`
		Types  map[string]typeNote `json:"types"`
	}{head, notes}, "", "  ")
	return append(out, '\n'), err
}

// children says where a type's tree children go: its slots, else none (blocks and most inputs
// ignore children).
func children(t manifestType) string {
	if len(t.Editor.Slots) == 0 {
		return "none (a leaf: children are ignored)"
	}
	var parts []string
	for _, raw := range t.Editor.Slots {
		var slot struct {
			Name  string   `json:"name"`
			Allow []string `json:"allow"`
		}
		if json.Unmarshal(raw, &slot) != nil {
			_ = json.Unmarshal(raw, &slot.Name) // a slot may be its bare name
		}
		if len(slot.Allow) > 0 {
			parts = append(parts, slot.Name+" — allows "+strings.Join(slot.Allow, ", "))
		} else {
			parts = append(parts, slot.Name+" — any children")
		}
	}
	return strings.Join(parts, "; ")
}
