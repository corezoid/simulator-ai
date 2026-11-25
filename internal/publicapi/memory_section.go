package publicapi

import (
	"fmt"
	"regexp"
	"strings"
)

// The person's memory is a section of their twin's description, so they see it on their own actor
// and edit it there like any text. The section starts with the line memoryHeading and runs to the
// next "[h3]" or the end of the description; everything outside it (the agent profile "# Agent",
// the person's own text) is kept byte for byte. Its lines are Basic Memory's micro-syntax:
//
//	[h3]AI memory[/h3]
//	- [preferences] Відповідати українською #language
//	- works_on [actor=<id>]Project X[/actor]
//
// A relation is an actor chip (shown in the UI, found by backlinks) and an edge from the twin. The
// model may write [[Title]] or [[actor:<id>]]: the tool turns them into chips. sim-api writes the
// same section after a dialog; both sides merge lines, neither rewrites the other's.

// memoryHeading opens the section; memoryBreak is where a next section starts.
const (
	memoryHeading  = "[h3]AI memory[/h3]"
	memoryBreak    = "[h3]"
	maxMemoryBytes = 8 << 10
)

var (
	noteLine      = regexp.MustCompile(`^\s*[-*]\s*\[([^\]\[=]{1,64})\]\s*(.+?)\s*$`)
	wikiRelation  = regexp.MustCompile(`^\s*[-*]\s*([A-Za-z_][A-Za-z0-9_-]{0,254})\s*\[\[(.+?)\]\]\s*$`)
	chipRelation  = regexp.MustCompile(`^\s*[-*]\s*([A-Za-z_][A-Za-z0-9_-]{0,254})\s*\[actor=([^\]\s]+)\](.*?)\[/actor\]\s*$`)
	tagPattern    = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_-]{1,64})`)
	chipTitleSafe = strings.NewReplacer("[", "(", "]", ")")
)

// MemoryNote is one observation: an English category, the fact in the person's language, tags.
type MemoryNote struct {
	Category string   `json:"category"`
	Text     string   `json:"text"`
	Tags     []string `json:"tags,omitempty"`
}

// MemoryRelation is one typed link to an actor.
type MemoryRelation struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	ActorID string `json:"actorId,omitempty"`
}

// memoryDoc is the section's facts in order; Other keeps the lines that are neither (a person's
// own remark in their memory), written back as they were.
type memoryDoc struct {
	Notes     []MemoryNote     `json:"notes"`
	Relations []MemoryRelation `json:"relations"`
	Other     []string         `json:"-"`
}

func newMemoryDoc() memoryDoc {
	return memoryDoc{Notes: []MemoryNote{}, Relations: []MemoryRelation{}}
}

// memoryLine reads one line of the micro-syntax: a note, a chip relation or a [[…]] relation.
func memoryLine(line string) (*MemoryNote, *MemoryRelation, bool) {
	if m := chipRelation.FindStringSubmatch(line); m != nil {
		return nil, &MemoryRelation{Type: m[1], ActorID: strings.TrimSpace(m[2]), Target: strings.TrimSpace(m[3])}, true
	}
	if m := wikiRelation.FindStringSubmatch(line); m != nil {
		r := MemoryRelation{Type: m[1], Target: strings.TrimSpace(m[2])}
		// [[actor:<id>|Title]] (the format of data.aiMemory before) or [[actor:<id>]].
		ref, title, titled := strings.Cut(r.Target, "|")
		if id, ok := strings.CutPrefix(strings.TrimSpace(ref), "actor:"); ok {
			r.ActorID, r.Target = strings.TrimSpace(id), ""
			if titled {
				r.Target = strings.TrimSpace(title)
			}
		}
		return nil, &r, true
	}
	if m := noteLine.FindStringSubmatch(line); m != nil {
		n := MemoryNote{Category: strings.TrimSpace(m[1]), Text: m[2]}
		for _, t := range tagPattern.FindAllStringSubmatch(m[2], -1) {
			n.Tags = append(n.Tags, t[1])
		}
		return &n, nil, true
	}
	return nil, nil, false
}

// lines splits a text into lines; a [br] the description editor writes is a line break too.
func lines(text string) []string {
	return strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "[br]", "\n"), "\n")
}

// parseFacts reads facts the model sends: every non-empty line must be the micro-syntax.
func parseFacts(text string) (memoryDoc, error) {
	doc := newMemoryDoc()
	for i, line := range lines(text) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n, r, ok := memoryLine(line)
		switch {
		case !ok:
			return memoryDoc{}, fmt.Errorf("line %d: %q is neither \"- [category] fact #tag\" nor \"- relation_type [[Actor]]\"", i+1, strings.TrimSpace(line))
		case n != nil:
			doc.Notes = append(doc.Notes, *n)
		default:
			doc.Relations = append(doc.Relations, *r)
		}
	}
	return doc, nil
}

// parseSection reads stored lines leniently: what is not the micro-syntax is kept as Other.
func parseSection(body string) memoryDoc {
	doc := newMemoryDoc()
	for _, line := range lines(body) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch n, r, ok := memoryLine(line); {
		case !ok:
			doc.Other = append(doc.Other, strings.TrimRight(line, " \t"))
		case n != nil:
			doc.Notes = append(doc.Notes, *n)
		default:
			doc.Relations = append(doc.Relations, *r)
		}
	}
	return doc
}

// render writes the section's lines: notes, relations as actor chips, then the other lines.
func (d memoryDoc) render() string {
	var b strings.Builder
	for _, n := range d.Notes {
		fmt.Fprintf(&b, "- [%s] %s\n", n.Category, n.Text)
	}
	for _, r := range d.Relations {
		if r.ActorID == "" {
			fmt.Fprintf(&b, "- %s [[%s]]\n", r.Type, r.Target)
			continue
		}
		title := chipTitleSafe.Replace(r.Target)
		if title == "" {
			title = r.ActorID
		}
		fmt.Fprintf(&b, "- %s [actor=%s]%s[/actor]\n", r.Type, r.ActorID, title)
	}
	for _, o := range d.Other {
		b.WriteString(o + "\n")
	}
	return b.String()
}

// section finds the memory section: where its heading line starts, where its body starts and
// where it ends (the next [h3] or the end); found false when the description has none.
func section(desc string) (start, body, end int, found bool) {
	at := 0
	for at <= len(desc) {
		i := strings.Index(desc[at:], memoryHeading)
		if i < 0 {
			return len(desc), len(desc), len(desc), false
		}
		i += at
		if i == 0 || desc[i-1] == '\n' { // a heading line, not the words inside a sentence
			body = i + len(memoryHeading)
			if next := strings.Index(desc[body:], memoryBreak); next >= 0 {
				return i, body, body + next, true
			}
			return i, body, len(desc), true
		}
		at = i + len(memoryHeading)
	}
	return len(desc), len(desc), len(desc), false
}

// readSection is the description's memory, empty without a section.
func readSection(desc string) memoryDoc {
	_, body, end, found := section(desc)
	if !found {
		return newMemoryDoc()
	}
	return parseSection(desc[body:end])
}

// writeSection puts the memory into the description and keeps the rest of it byte for byte: a new
// section is appended after a blank line, an existing one replaced in place, an empty one removed.
func writeSection(desc string, doc memoryDoc) (string, error) {
	rendered := doc.render()
	if len(rendered) > maxMemoryBytes {
		return "", fmt.Errorf("the memory section would be %d bytes, more than %d: forget something first", len(rendered), maxMemoryBytes)
	}
	start, _, end, found := section(desc)
	before, after := desc[:start], desc[end:]
	if rendered == "" {
		if !found {
			return desc, nil
		}
		if after == "" {
			before = strings.TrimSuffix(before, "\n\n") // the blank line the section was appended after
		}
		return before + after, nil
	}
	block := memoryHeading + "\n" + rendered
	if after != "" {
		block += "\n" // a blank line before the next section
	}
	switch {
	case found || desc == "":
	case strings.HasSuffix(desc, "\n\n"):
	case strings.HasSuffix(desc, "\n"):
		before += "\n"
	default:
		before += "\n\n"
	}
	return before + block + after, nil
}
