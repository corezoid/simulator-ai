package publicapi

import (
	"fmt"
	"strings"

	"github.com/corezoid/simulator-ai/internal/aimemory"
)

// The person's memory is a block of their twin's description (internal/aimemory holds its format):
// the plugin reads it, and sim-api writes it — memory_save sends its change to actor/memory/save,
// which merges it under the same locks as the automatic memory after a dialog. The model may write
// [[Title]] or [[actor:<id>]]: the tool turns them into chips.

type (
	// MemoryNote is one observation: an English category, the fact in the person's language, tags.
	MemoryNote = aimemory.Note
	// MemoryRelation is one typed link to an actor.
	MemoryRelation = aimemory.Relation
	memoryDoc      = aimemory.Doc
)

func newMemoryDoc() memoryDoc { return aimemory.NewDoc() }

func readSection(desc string) memoryDoc { return aimemory.Read(desc) }

func writeSection(desc string, doc memoryDoc) (string, error) { return aimemory.Write(desc, doc) }

func hasNote(doc memoryDoc, n MemoryNote) bool { return aimemory.HasNote(doc, n) }

func hasRelation(doc memoryDoc, r MemoryRelation) bool { return aimemory.HasRelation(doc, r) }

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
		n, r, ok := aimemory.Line(line)
		switch {
		case !ok:
			return memoryDoc{}, fmt.Errorf("line %d: %q is neither \"- [category] fact #tag\" nor \"- relation_type [[Actor]]\"", i+1, strings.TrimSpace(line))
		case n != nil:
			// A fact is text, never markup: brackets would be BBCode in the description (an [h3]
			// would end the section, a [/actor] break a chip), as sim-api writes them.
			n.Category, n.Text = aimemory.Category(n.Category), aimemory.CleanFact(n.Text)
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
