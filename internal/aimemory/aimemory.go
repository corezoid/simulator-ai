// Package aimemory is the text of a person's AI memory: a block of their twin's description, so they
// see it on their own actor and edit it there like any text. The block is the line Heading and the
// memory lines right after it, with no line between them; the first line that is not a memory line
// (a blank line, a paragraph, a next "[h3]") ends it, and everything after it is the person's text,
// never read as memory even when it looks like a memory line (a "- [x] done" checklist of an
// "# Agent" profile). Everything outside the block is kept byte for byte. Its lines are Basic
// Memory's micro-syntax:
//
//	[h3]AI memory[/h3]
//	- [preference] Відповідати українською #language
//	- works_on [actor=<id>]Project X[/actor]
//
// A relation is an actor chip (shown in the UI, found by backlinks) and an edge from the twin.
//
// sim-api owns the writes: its e_ai_memory holds the same rules and vectors, the automatic memory
// merges into the block after a dialog and actor/memory/save merges memory_save's change (Save here
// is the same merge, for the plugin's preview and the fake). The plugin reads the block and never
// writes the description itself.
package aimemory

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Heading opens the block.
const (
	Heading  = "[h3]AI memory[/h3]"
	MaxBytes = 8 << 10
)

var (
	noteLine      = regexp.MustCompile(`^\s*[-*]\s*\[([^\]\[=]{1,64})\]\s*(.+?)\s*$`)
	wikiRelation  = regexp.MustCompile(`^\s*[-*]\s*([A-Za-z_][A-Za-z0-9_-]{0,254})\s*\[\[(.+?)\]\]\s*$`)
	chipRelation  = regexp.MustCompile(`^\s*[-*]\s*([A-Za-z_][A-Za-z0-9_-]{0,254})\s*\[actor=([^\]\s]+)\](.*?)\[/actor\]\s*$`)
	tagPattern    = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_-]{1,64})`)
	chipTitleSafe = strings.NewReplacer("[", "(", "]", ")")
)

// Note is one observation: an English category, the fact in the person's language, tags.
type Note struct {
	Category string   `json:"category"`
	Text     string   `json:"text"`
	Tags     []string `json:"tags,omitempty"`
}

// Relation is one typed link to an actor.
type Relation struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	ActorID string `json:"actorId,omitempty"`
}

// Doc is the block's facts in order.
type Doc struct {
	Notes     []Note     `json:"notes"`
	Relations []Relation `json:"relations"`
}

func NewDoc() Doc {
	return Doc{Notes: []Note{}, Relations: []Relation{}}
}

// Line reads one line of the micro-syntax: a note, a chip relation or a [[…]] relation.
func Line(line string) (*Note, *Relation, bool) {
	if m := chipRelation.FindStringSubmatch(line); m != nil {
		return nil, &Relation{Type: m[1], ActorID: strings.TrimSpace(m[2]), Target: strings.TrimSpace(m[3])}, true
	}
	if m := wikiRelation.FindStringSubmatch(line); m != nil {
		r := Relation{Type: m[1], Target: strings.TrimSpace(m[2])}
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
		n := Note{Category: strings.TrimSpace(m[1]), Text: m[2]}
		for _, t := range tagPattern.FindAllStringSubmatch(m[2], -1) {
			n.Tags = append(n.Tags, t[1])
		}
		return &n, nil, true
	}
	return nil, nil, false
}

// Categories are sim-api's (e_ai_memory.Categories): both name a fact alike, so the same fact is
// not kept twice under two spellings.
var Categories = []string{"preference", "role", "responsibility", "works_with", "rule", "note"}

// Category is a category as sim-api writes it: lower case, singular where that is one of its
// own; another English word is kept.
func Category(c string) string {
	c = strings.ToLower(strings.Join(strings.Fields(c), "_"))
	if one := strings.TrimSuffix(c, "s"); one != c && slices.Contains(Categories, one) {
		return one
	}
	return c
}

var factCleaner = strings.NewReplacer("[", "(", "]", ")")

// CleanFact is a fact's text with brackets as parentheses and its spacing collapsed.
func CleanFact(text string) string {
	return strings.Join(strings.Fields(factCleaner.Replace(text)), " ")
}

// blockLine reads one stored line of the block: its pieces between the [br]s the description
// editor writes; ok false when a piece is not a memory line or the line is blank (the block ends).
func blockLine(raw string) (notes []Note, relations []Relation, ok bool) {
	for _, piece := range strings.Split(raw, "[br]") {
		if strings.TrimSpace(piece) == "" {
			continue
		}
		n, r, isMemory := Line(piece)
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

// parseBlock reads the block's lines (section bounds them; the rest of the heading's own line is
// blank or "[br]- …"), up to the first that is not memory.
func parseBlock(body string) Doc {
	doc := NewDoc()
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(strings.ReplaceAll(raw, "[br]", "")) == "" {
			continue
		}
		notes, relations, ok := blockLine(raw)
		if !ok {
			break
		}
		doc.Notes, doc.Relations = append(doc.Notes, notes...), append(doc.Relations, relations...)
	}
	return doc
}

// Render writes the block's lines: notes, then relations as actor chips.
func (d Doc) Render() string {
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
		i := strings.Index(desc[at:], Heading)
		if i < 0 {
			return len(desc), len(desc), len(desc), false
		}
		i += at
		if i == 0 || desc[i-1] == '\n' || strings.HasSuffix(desc[:i], "[br]") {
			body = i + len(Heading)
			return i, body, blockEnd(desc, body), true
		}
		at = i + len(Heading)
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
		if _, _, ok := blockLine(rest); !ok {
			return pos // text on the heading's own line: no memory lines
		}
	}
	pos = next
	for pos < len(desc) {
		end, next = lineEnd(pos)
		if _, _, ok := blockLine(strings.TrimSuffix(desc[pos:end], "\r")); !ok {
			break
		}
		pos = next
	}
	return pos
}

// Read is the description's memory, empty without a section.
func Read(desc string) Doc {
	_, body, end, found := section(desc)
	if !found {
		return NewDoc()
	}
	return parseBlock(desc[body:end])
}

// Write puts the memory into the description and keeps the rest of it byte for byte: a new
// block is appended after a blank line, an existing one replaced in place, an empty one removed
// (with the line break that set it apart from the text after it).
func Write(desc string, doc Doc) (string, error) {
	rendered := doc.Render()
	// The heading counts, as in sim-api's MaxSectionBytes: past it sim-api drops the oldest lines.
	if size := len(Heading) + len(rendered); size > MaxBytes {
		return "", fmt.Errorf("the memory section would be %d bytes, more than %d: forget something first", size, MaxBytes)
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
	return before + Heading + "\n" + rendered + after, nil
}

// HasNote reports the same note: category and text alike, case aside (the text carries the tags).
func HasNote(doc Doc, n Note) bool {
	return slices.ContainsFunc(doc.Notes, func(m Note) bool {
		return strings.EqualFold(m.Category, n.Category) && strings.EqualFold(m.Text, n.Text)
	})
}

// HasRelation reports the same relation: its type and actor (or, unresolved, its title).
func HasRelation(doc Doc, r Relation) bool {
	return slices.ContainsFunc(doc.Relations, func(m Relation) bool {
		return m.Type == r.Type && (m.ActorID != "" && m.ActorID == r.ActorID || m.ActorID == "" && strings.EqualFold(m.Target, r.Target))
	})
}

// Forget drops the notes and relations names name — a note by its text, a relation by its title,
// its actor id or "actor:<id>", case aside — and answers the relations it dropped.
func Forget(doc Doc, names []string) (Doc, []Relation) {
	gone := map[string]bool{}
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			gone[n] = true
		}
	}
	notes := doc.Notes[:0:0]
	for _, n := range doc.Notes {
		if !gone[strings.ToLower(n.Text)] {
			notes = append(notes, n)
		}
	}
	var dropped []Relation
	relations := doc.Relations[:0:0]
	for _, r := range doc.Relations {
		id := strings.ToLower(r.ActorID)
		if gone[strings.ToLower(r.Target)] || id != "" && (gone[id] || gone["actor:"+id]) {
			dropped = append(dropped, r)
			continue
		}
		relations = append(relations, r)
	}
	doc.Notes, doc.Relations = notes, relations
	return doc, dropped
}

// Saved is what Save made of the memory.
type Saved struct {
	Doc       Doc
	Added     int
	Removed   int
	Forgotten []Relation
}

// Save is sim-api's actor/memory/save merge (e_ai_memory.Save): forget first, then add the notes and
// relations not there yet at the end of the memory lines; everything else stays byte for byte. An
// error when the block would pass MaxBytes (the description unchanged).
func Save(desc string, notes []Note, relations []Relation, forget []string) (string, Saved, error) {
	doc := Read(desc)
	held := len(doc.Notes) + len(doc.Relations)
	var saved Saved
	doc, saved.Forgotten = Forget(doc, forget)
	saved.Removed = held - len(doc.Notes) - len(doc.Relations)
	for _, n := range notes {
		if !HasNote(doc, n) {
			doc.Notes, saved.Added = append(doc.Notes, n), saved.Added+1
		}
	}
	for _, r := range relations {
		if !HasRelation(doc, r) {
			doc.Relations, saved.Added = append(doc.Relations, r), saved.Added+1
		}
	}
	out, err := Write(desc, doc)
	if err != nil {
		return desc, Saved{}, err
	}
	saved.Doc = doc
	return out, saved, nil
}

// Parse reads memory lines without a heading (the legacy data.aiMemory text), up to the first line
// that is not one.
func Parse(text string) Doc { return parseBlock(text) }
