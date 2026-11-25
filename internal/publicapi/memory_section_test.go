package publicapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/aimemory"
	"github.com/corezoid/simulator-ai/internal/simfake"
)

// The block is found only as a heading line and ends at its last memory line; writing it keeps every
// byte outside it, appends a new one after a blank line and removes an emptied one.
func TestMemorySectionKeepsTheRestOfTheDescription(t *testing.T) {
	doc := memoryDoc{Notes: []MemoryNote{{Category: "preferences", Text: "Відповідати коротко #style"}},
		Relations: []MemoryRelation{{Type: "works_on", ActorID: "a-1", Target: "Project [X]"}}}
	const body = "[h3]AI memory[/h3]\n- [preferences] Відповідати коротко #style\n- works_on [actor=a-1]Project (X)[/actor]\n"
	for _, tc := range []struct{ name, desc, want string }{
		{"empty", "", body},
		{"text without a newline", "# Agent\nrole: lawyer", "# Agent\nrole: lawyer\n\n" + body},
		{"text with a newline", "Hello\n", "Hello\n\n" + body},
		{"text with a blank line", "Hello\n\n", "Hello\n\n" + body},
		{"replace at the end", "Intro\n\n[h3]AI memory[/h3]\n- [old] fact\n", "Intro\n\n" + body},
		{"replace between sections", "[h3]Bio[/h3]\nA\n\n[h3]AI memory[/h3]\n- [old] fact\n\n[h3]Links[/h3]\nB",
			"[h3]Bio[/h3]\nA\n\n" + body + "\n[h3]Links[/h3]\nB"},
		{"heading inside a sentence is text", "See [h3]AI memory[/h3] below", "See [h3]AI memory[/h3] below\n\n" + body},
	} {
		got, err := writeSection(tc.desc, doc)
		if err != nil || got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q (%v)", tc.name, got, tc.want, err)
		}
		if read := readSection(got); len(read.Notes) != 1 || len(read.Relations) != 1 || read.Relations[0].ActorID != "a-1" || read.Relations[0].Target != "Project (X)" {
			t.Errorf("%s: read back %+v", tc.name, read)
		}
	}
	for _, tc := range []struct{ desc, want string }{
		{"Intro\n\n[h3]AI memory[/h3]\n- [a] b\n", "Intro"},
		{"[h3]AI memory[/h3]\n- [a] b\n\n[h3]Links[/h3]\nB", "[h3]Links[/h3]\nB"},
		{"Only text", "Only text"},
	} {
		if got, _ := writeSection(tc.desc, newMemoryDoc()); got != tc.want {
			t.Errorf("emptied %q: %q, want %q", tc.desc, got, tc.want)
		}
	}
	big := memoryDoc{Notes: []MemoryNote{{Category: "c", Text: strings.Repeat("x", aimemory.MaxBytes)}}}
	if _, err := writeSection("", big); err == nil || !strings.Contains(err.Error(), "more than 8192") {
		t.Errorf("oversized section: %v", err)
	}
}

// Stored lines are read as the editor writes them: a [br] splits lines, the old [[actor:id|T]] and
// chips are relations; the first line that is not memory ends the block and stays where it is.
func TestMemorySectionReadsWhatPeopleAndEditorsWrite(t *testing.T) {
	desc := "[h3]AI memory[/h3][br]- [role] Юрист #work[br]- interaction [actor=p-1]Project X[/actor]\n- works_on [[actor:p-2|Project Y]]\nmy own note\n"
	doc := readSection(desc)
	if len(doc.Notes) != 1 || doc.Notes[0].Tags[0] != "work" || len(doc.Relations) != 2 || doc.Relations[1].ActorID != "p-2" || doc.Relations[1].Target != "Project Y" {
		t.Fatalf("%+v", doc)
	}
	got, _ := writeSection(desc, doc)
	if want := "[h3]AI memory[/h3]\n- [role] Юрист #work\n- interaction [actor=p-1]Project X[/actor]\n- works_on [actor=p-2]Project Y[/actor]\nmy own note\n"; got != want {
		t.Fatalf("write\n got %q\nwant %q", got, want)
	}
	if _, err := parseFacts("- [a] b\nnot a fact"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("facts: %v", err)
	}
}

// The person's text after the block is never memory, even where it looks like memory lines: a
// "- [x] done" checklist after a paragraph or after a blank line stays where it is, byte for byte,
// when the memory grows and when it is emptied. (The same vectors as sim-api's e_ai_memory.)
func TestMemoryBlockLeavesThePersonsChecklist(t *testing.T) {
	const after = "My checklist:\n- [x] buy milk\n- [ ] call Ann\n"
	for _, tc := range []struct{ name, desc, grown, emptied string }{
		{"after a paragraph", "[h3]AI memory[/h3]\n- [role] Юрист\n" + after,
			"[h3]AI memory[/h3]\n- [role] Юрист\n- [rule] Short answers\n" + after, after},
		{"after a blank line", "Intro\n\n[h3]AI memory[/h3]\n- [role] Юрист\n\n- [x] done\n",
			"Intro\n\n[h3]AI memory[/h3]\n- [role] Юрист\n- [rule] Short answers\n\n- [x] done\n", "Intro\n\n- [x] done\n"},
		{"in an # Agent profile", "# Agent\nrole: lawyer\n\n[h3]AI memory[/h3]\n- [role] Юрист\n\n## Checklist\n- [x] read the contract\n",
			"# Agent\nrole: lawyer\n\n[h3]AI memory[/h3]\n- [role] Юрист\n- [rule] Short answers\n\n## Checklist\n- [x] read the contract\n",
			"# Agent\nrole: lawyer\n\n## Checklist\n- [x] read the contract\n"},
	} {
		doc := readSection(tc.desc)
		if len(doc.Notes) != 1 || doc.Notes[0].Category != "role" || len(doc.Relations) != 0 {
			t.Errorf("%s: read %+v", tc.name, doc)
			continue
		}
		doc.Notes = append(doc.Notes, MemoryNote{Category: "rule", Text: "Short answers"})
		if got, err := writeSection(tc.desc, doc); err != nil || got != tc.grown {
			t.Errorf("%s: grown\n got %q\nwant %q (%v)", tc.name, got, tc.grown, err)
		} else if again, _ := writeSection(got, readSection(got)); again != got {
			t.Errorf("%s: a rewrite changed the text: %q", tc.name, again)
		}
		if got, _ := writeSection(tc.desc, newMemoryDoc()); got != tc.emptied {
			t.Errorf("%s: emptied\n got %q\nwant %q", tc.name, got, tc.emptied)
		}
	}
	if doc := readSection("[h3]AI memory[/h3] about me\n- [role] X\n"); len(doc.Notes) != 0 {
		t.Errorf("text on the heading's line is not memory, nor what follows it: %+v", doc)
	}
}

// A fact is text: brackets become parentheses, so a fact cannot end the section or break a chip;
// a category is written as sim-api writes it.
func TestMemoryFactsAreNotMarkup(t *testing.T) {
	doc, err := parseFacts("- [Preferences] use [h3] titles and [/actor] [b]bold[/b]   here")
	if err != nil || doc.Notes[0].Category != "preference" || doc.Notes[0].Text != "use (h3) titles and (/actor) (b)bold(/b) here" {
		t.Fatalf("%+v %v", doc, err)
	}
	desc, _ := writeSection("", doc)
	desc += "\n[h3]Next[/h3]\nX"
	if read := readSection(desc); len(read.Notes) != 1 || !strings.HasSuffix(desc, "\n[h3]Next[/h3]\nX") {
		t.Fatalf("read back %+v from %q", read, desc)
	}
}

// Text the person typed under the memory (an "# Agent" profile, a paragraph) keeps its lines and
// blank lines; a heading the editor wrote after [br] is the block, not a reason for a second one.
func TestMemorySectionKeepsThePersonsText(t *testing.T) {
	const profile = "# Agent\n\nrole: lawyer\n\n  indented: yes"
	desc := "Intro\n\n[h3]AI memory[/h3]\n- [role] Юрист\n\n" + profile + "\n"
	doc := readSection(desc)
	doc.Notes = append(doc.Notes, MemoryNote{Category: "rule", Text: "Short answers"})
	got, err := writeSection(desc, doc)
	if want := "Intro\n\n[h3]AI memory[/h3]\n- [role] Юрист\n- [rule] Short answers\n\n" + profile + "\n"; err != nil || got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	again, _ := writeSection(got, readSection(got))
	if again != got {
		t.Fatalf("a rewrite changed the text:\n%q\n%q", got, again)
	}
	br := "Intro[br][h3]AI memory[/h3][br]- [role] Юрист"
	if got, _ := writeSection(br, readSection(br)); strings.Count(got, aimemory.Heading) != 1 {
		t.Fatalf("a second section: %q", got)
	}
}

// A save moves the legacy data.aiMemory into the section and clears it; a section another writer
// (sim-api after a dialog) changed between the first read and the write is merged, not lost.
func TestSaveMemoryMigratesAndMerges(t *testing.T) {
	_, f, project := kbClient(t)
	c := kbJWTClient(t, f)
	ctx := context.Background()
	twin := f.TwinOf(7, 70)
	f.Set(func(s *simfake.Server) {
		a := s.Actors[twin]
		a.Description = "# Agent\nrole: assistant"
		a.Data["aiMemory"] = "- [preference] Short answers\n- interaction [[actor:" + project + "|Project X]]\n"
		s.KB.BeforeRead = func(a *simfake.Actor) {
			if a.ID == twin && !strings.Contains(a.Description, "sim-api") {
				a.Description += "\n\n[h3]AI memory[/h3]\n- [context] Asked about sim-api #dialog\n"
			}
		}
	})
	self := MemoryInput{SelfUserID: 7}
	read, err := c.GetMemory(ctx, self)
	if err != nil || !read.Legacy || len(read.Notes) != 1 || len(read.Relations) != 1 {
		t.Fatalf("get before the move %+v %v", read, err)
	}
	got, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- [workflow] Sends drafts first"})
	if err != nil || got.Added != 1 || got.Edges != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	stored, _ := f.ActorData(twin)
	want := "# Agent\nrole: assistant\n\n[h3]AI memory[/h3]\n- [context] Asked about sim-api #dialog\n- [preference] Short answers\n- [workflow] Sends drafts first\n- interaction [actor=" + project + "]Project X[/actor]\n"
	if stored.Description != want || stored.Data["aiMemory"] != nil {
		t.Fatalf("description %q\nwant        %q\nlegacy %v", stored.Description, want, stored.Data["aiMemory"])
	}
	read, err = c.GetMemory(ctx, self)
	if err != nil || read.Legacy || len(read.Notes) != 3 {
		t.Fatalf("get after %+v %v", read, err)
	}
}

// kbJWTClient is a run's client whose Simulator JWT names SuperAdmin user 70 (member 7 of TwinOf).
func kbJWTClient(t *testing.T, f *simfake.Server) *Client {
	t.Helper()
	c, err := New(Config{Origin: f.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"70","nick":"Ann"}`)) + ".y"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// memory_save never writes the description itself: one actor/memory/save carries the change, which
// sim-api merges into the block as it is at the write; actor/update only clears the legacy key. A
// change actor/memory/save would refuse is refused before anything is written.
func TestSaveMemoryWritesThroughTheServer(t *testing.T) {
	_, f, _ := kbClient(t)
	c := kbJWTClient(t, f)
	ctx := context.Background()
	twin := f.TwinOf(7, 70)
	self := MemoryInput{SelfUserID: 7}
	f.Set(func(s *simfake.Server) {
		s.Actors[twin].Description = "Intro\n\n[h3]AI memory[/h3]\n- [role] CFO\nMy checklist:\n- [x] buy milk\n"
	})
	got, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- [rule] Short answers", Forget: []string{"cfo"}})
	if err != nil || got.Added != 1 || got.Removed != 1 || len(got.Notes) != 1 || got.Notes[0].Text != "Short answers" {
		t.Fatalf("%+v %v", got, err)
	}
	if n, u := f.Count("actor/memory/save"), f.Count("actor/update"); n != 1 || u != 0 {
		t.Fatalf("actor/memory/save %d, actor/update %d", n, u)
	}
	if stored, _ := f.ActorData(twin); stored.Description != "Intro\n\n[h3]AI memory[/h3]\n- [rule] Short answers\nMy checklist:\n- [x] buy milk\n" {
		t.Fatalf("stored %q", stored.Description)
	}
	many := ""
	for i := range 51 {
		many += fmt.Sprintf("- [note] fact %d\n", i)
	}
	for _, bad := range []struct {
		in   MemorySaveInput
		want string
	}{
		{MemorySaveInput{MemoryInput: self, Text: many}, "at most 50 facts"},
		{MemorySaveInput{MemoryInput: self, Text: "- [note] " + strings.Repeat("x", 1001)}, "at most 1000 characters"},
	} {
		if _, err := c.SaveMemory(ctx, bad.in); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%.30q: %v", bad.in.Text, err)
		}
	}
	if n := f.Count("actor/memory/save"); n != 1 {
		t.Fatalf("a refused change reached sim-api: %d saves", n)
	}
}

// A legacy relation no actor answers to — an unknown title, a deleted actor's chip — stays in
// data.aiMemory alone; what resolves moves into the block, the key no longer holds it (so a later
// forget sticks), and a save is never blocked by it.
func TestSaveMemoryKeepsAnUnresolvedLegacyRelation(t *testing.T) {
	_, f, _ := kbClient(t)
	c := kbJWTClient(t, f)
	ctx := context.Background()
	twin := f.TwinOf(7, 70)
	self := MemoryInput{SelfUserID: 7}
	f.Set(func(s *simfake.Server) {
		s.KB.Strict = true // actor/get of a deleted actor answers notFound
		s.Actors[twin].Data["aiMemory"] = "- [Preferences] Tea\n- interaction [[Nobody at all]]\n- works_with [[actor:gone-1|Old Co]]\n"
	})
	if _, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- [role] CFO"}); err != nil {
		t.Fatal(err)
	}
	stored, _ := f.ActorData(twin)
	left := "- interaction [[Nobody at all]]\n- works_with [actor=gone-1]Old Co[/actor]\n"
	if stored.Description != "[h3]AI memory[/h3]\n- [preference] Tea\n- [role] CFO\n" || stored.Data["aiMemory"] != left {
		t.Fatalf("description %q legacy %q", stored.Description, stored.Data["aiMemory"])
	}
	if _, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Forget: []string{"Tea"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- [rule] Short answers"}); err != nil {
		t.Fatal(err)
	}
	if stored, _ := f.ActorData(twin); stored.Description != "[h3]AI memory[/h3]\n- [role] CFO\n- [rule] Short answers\n" {
		t.Fatalf("a forgotten legacy fact came back: %q", stored.Description)
	}
}

// The edges follow the write: a save sim-api refuses (here the block grew past 8 KiB after the first
// read) draws and deletes nothing.
func TestSaveMemoryRefusedDrawsNoEdge(t *testing.T) {
	_, f, project := kbClient(t)
	c := kbJWTClient(t, f)
	twin := f.TwinOf(7, 70)
	f.Set(func(s *simfake.Server) {
		s.KB.BeforeRead = func(a *simfake.Actor) {
			if a.ID == twin {
				a.Description = "[h3]AI memory[/h3]\n- [note] " + strings.Repeat("x", aimemory.MaxBytes) + "\n"
			}
		}
	})
	_, err := c.SaveMemory(context.Background(), MemorySaveInput{MemoryInput: MemoryInput{SelfUserID: 7}, Text: "- interaction [[actor:" + project + "]]"})
	if err == nil {
		t.Fatal("an oversized block was saved")
	}
	f.Set(func(s *simfake.Server) {
		for _, e := range s.Edges {
			if e.Source == twin {
				t.Errorf("an edge was drawn for a refused save: %+v", e)
			}
		}
	})
}
