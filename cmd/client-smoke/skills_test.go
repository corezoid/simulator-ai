package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// maxSkillDescription bounds a SKILL.md description. Claude Code, Codex and
// Kiro load the name and description of every installed skill into every
// session, Simulator task or not, and once the skill listing overflows Claude
// Code shows skills by name only. A description says what the skill does and a
// few trigger phrases; everything else belongs in the body, which loads only
// when the skill runs.
const maxSkillDescription = 200

func TestSkillFrontmatter(t *testing.T) {
	root := filepath.Join("..", "..", "plugins", "simulator-ai", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		checked++
		name, description, err := skillFrontmatter(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if name != entry.Name() {
			t.Errorf("%s: name %q differs from its directory", path, name)
		}
		if description == "" {
			t.Errorf("%s: missing description", path)
		}
		if n := utf8.RuneCountInString(description); n > maxSkillDescription {
			t.Errorf("%s: description is %d characters, over %d: keep what the skill does and a few trigger phrases, move the rest into the body",
				path, n, maxSkillDescription)
		}
	}
	if checked == 0 {
		t.Fatalf("no SKILL.md under %s", root)
	}
}

// skillFrontmatter parses the leading YAML block the way the hosts do, so a
// folded, literal or multi-line description is measured whole.
func skillFrontmatter(data []byte) (string, string, error) {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return "", "", errNoFrontmatter
	}
	block, _, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return "", "", errNoFrontmatter
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return "", "", err
	}
	return fm.Name, fm.Description, nil
}

var errNoFrontmatter = errors.New("no --- frontmatter block")

func TestSkillFrontmatterScalarForms(t *testing.T) {
	const want = "First line. Second line."
	for name, fm := range map[string]string{
		"plain":           "description: First line. Second line.\n",
		"plain multiline": "description: First line.\n  Second line.\n",
		"folded strip":    "description: >-\n  First line.\n  Second line.\n",
		"literal strip":   "description: |-\n  First line. Second line.\n",
		"double quoted":   "description: \"First line. Second line.\"\n",
	} {
		_, got, err := skillFrontmatter([]byte("---\nname: x\n" + fm + "---\nbody\n"))
		if err != nil || got != want {
			t.Errorf("%s: description = %q, %v; want %q", name, got, err, want)
		}
	}
}
