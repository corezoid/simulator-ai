package publicapi

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The person's memory is a block of their twin's description, so they see it on their own actor and
// edit it there like any text. The block is the line memoryHeading and the memory lines right after
// it, with no line between them; the first line that is not a memory line (a blank line, a
// paragraph, a next "[h3]") ends it, and everything after it is the person's text, never read as
// memory even when it looks like a memory line (a "- [x] done" checklist of an "# Agent" profile).
// Everything outside the block is kept byte for byte. Its lines are Basic Memory's micro-syntax:
//
//	[h3]AI memory[/h3]
//	- [preference] Відповідати українською #language
//	- works_on [actor=<id>]Project X[/actor]
//
// A relation is an actor chip (shown in the UI, found by backlinks) and an edge from the twin. The
// model may write [[Title]] or [[actor:<id>]]: the tool turns them into chips. sim-api writes the
// same block after a dialog (e_ai_memory, the same rules and vectors); both merge lines.

// memoryHeading opens the block.
const (
	memoryHeading  = "[h3]AI memory[/h3]"
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

// memoryDoc is the block's facts in order.
type memoryDoc struct {
	Notes     []MemoryNote     `json:"notes"`
	Relations []MemoryRelation `json:"relations"`
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
			// A fact is text, never markup: brackets would be BBCode in the description (an [h3]
			// would end the section, a [/actor] break a chip), as sim-api's FactLine writes them.
			n.Category, n.Text = factCategory(n.Category), cleanFact(n.Text)
			if n.Text == "" {
				return memoryDoc{}, fmt.Errorf("line %d: the fact is empty", i+1)
			}
			doc.Notes = append(doc.Notes, *n)
		default:
			doc.Relations = append(doc.Relations, *r)
		}
	}
	return doc, nil
}

// memoryCategories are sim-api's (e_ai_memory.Categories): both writers name a fact alike, so the
// same fact is not kept twice under two spellings.
var memoryCategories = []string{"preference", "role", "responsibility", "works_with", "rule", "note"}

// factCategory is a category as sim-api writes it: lower case, singular where that is one of its
// own; another English word is kept.
func factCategory(c string) string {
	c = strings.ToLower(strings.Join(strings.Fields(c), "_"))
	if one := strings.TrimSuffix(c, "s"); one != c && slices.Contains(memoryCategories, one) {
		return one
	}
	return c
}

var factCleaner = strings.NewReplacer("[", "(", "]", ")")

// cleanFact is a fact's text with brackets as parentheses and its spacing collapsed.
func cleanFact(text string) string {
	return strings.Join(strings.Fields(factCleaner.Replace(text)), " ")
}

// memoryRaw reads one stored line of the block: its pieces between the [br]s the description
// editor writes; ok false when a piece is not a memory line or the line is blank (the block ends).
func memoryRaw(raw string) (notes []MemoryNote, relations []MemoryRelation, ok bool) {
	for _, piece := range strings.Split(raw, "[br]") {
		if strings.TrimSpace(piece) == "" {
			continue
		}
		n, r, isMemory := memoryLine(piece)
		switch {
		case !isMemory:
			return nil, nil, false
		case n != nil:
			notes = append(notes, *n)
		default:
			relations = append(relations, *r)
		}
	}
	return notes, relations, len(notes)+len(relations) > 0
}

// parseSection reads the block's lines (section bounds them; the rest of the heading's own line is
// blank or "[br]- …"), up to the first that is not memory.
func parseSection(body string) memoryDoc {
	doc := newMemoryDoc()
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(strings.ReplaceAll(raw, "[br]", "")) == "" {
			continue
		}
		notes, relations, ok := memoryRaw(raw)
		if !ok {
			break
		}
		doc.Notes, doc.Relations = append(doc.Notes, notes...), append(doc.Relations, relations...)
	}
	return doc
}

// render writes the block's lines: notes, then relations as actor chips.
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
	return b.String()
}

// section finds the memory block: where its heading line starts, where its lines start and where
// they end (after the last memory line's newline); found false when the description has none. The
// heading is a line of its own (at the start, after a newline or after the editor's [br]), never
// the words inside a sentence.
func section(desc string) (start, body, end int, found bool) {
	at := 0
	for at <= len(desc) {
		i := strings.Index(desc[at:], memoryHeading)
		if i < 0 {
			return len(desc), len(desc), len(desc), false
		}
		i += at
		if i == 0 || desc[i-1] == '\n' || strings.HasSuffix(desc[:i], "[br]") {
			body = i + len(memoryHeading)
			return i, body, blockEnd(desc, body), true
		}
		at = i + len(memoryHeading)
	}
	return len(desc), len(desc), len(desc), false
}

// blockEnd is where the memory lines that start at body end: the rest of the heading's line
// ("[br]- …" from the editor) and every next line while it is a memory line.
func blockEnd(desc string, body int) int {
	pos := body
	lineEnd := func(from int) (int, int) { // the line's end and the next line's start
		if i := strings.IndexByte(desc[from:], '\n'); i >= 0 {
			return from + i, from + i + 1
		}
		return len(desc), len(desc)
	}
	end, next := lineEnd(pos)
	if rest := strings.TrimSuffix(desc[pos:end], "\r"); strings.TrimSpace(rest) != "" {
		if _, _, ok := memoryRaw(rest); !ok {
			return pos // text on the heading's own line: no memory lines
		}
	}
	pos = next
	for pos < len(desc) {
		end, next = lineEnd(pos)
		if _, _, ok := memoryRaw(strings.TrimSuffix(desc[pos:end], "\r")); !ok {
			break
		}
		pos = next
	}
	return pos
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
// block is appended after a blank line, an existing one replaced in place, an empty one removed
// (with the line break that set it apart from the text after it).
func writeSection(desc string, doc memoryDoc) (string, error) {
	rendered := doc.render()
	// The heading counts, as in sim-api's MaxSectionBytes: past it sim-api drops the oldest lines.
	if size := len(memoryHeading) + len(rendered); size > maxMemoryBytes {
		return "", fmt.Errorf("the memory section would be %d bytes, more than %d: forget something first", size, maxMemoryBytes)
	}
	start, _, end, found := section(desc)
	before, after := desc[:start], desc[end:]
	if rendered == "" {
		if !found {
			return desc, nil
		}
		if after == "" {
			before = strings.TrimSuffix(before, "\n\n") // the blank line the block was appended after
		} else {
			after = strings.TrimPrefix(after, "\n") // the blank line between the block and the text
		}
		return before + after, nil
	}
	switch {
	case found || desc == "":
	case strings.HasSuffix(desc, "\n\n"):
	case strings.HasSuffix(desc, "\n"):
		before += "\n"
	default:
		before += "\n\n"
	}
	return before + memoryHeading + "\n" + rendered + after, nil
}
