package publicapi

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/simfake"
)

// The section is found only as a heading line and ends at the next [h3]; writing it keeps every
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
	big := memoryDoc{Notes: []MemoryNote{{Category: "c", Text: strings.Repeat("x", maxMemoryBytes)}}}
	if _, err := writeSection("", big); err == nil || !strings.Contains(err.Error(), "more than 8192") {
		t.Errorf("oversized section: %v", err)
	}
}

// Stored lines are read leniently: a [br] the editor writes splits lines, the old [[actor:id|T]]
// and chips are relations, and a person's own line is kept as it is.
func TestMemorySectionReadsWhatPeopleAndEditorsWrite(t *testing.T) {
	desc := "[h3]AI memory[/h3][br]- [role] Юрист #work[br]- interaction [actor=p-1]Project X[/actor]\n- works_on [[actor:p-2|Project Y]]\nmy own note\n"
	doc := readSection(desc)
	if len(doc.Notes) != 1 || doc.Notes[0].Tags[0] != "work" || len(doc.Relations) != 2 || doc.Relations[1].ActorID != "p-2" || doc.Relations[1].Target != "Project Y" ||
		len(doc.Other) != 1 || doc.Other[0] != "my own note" {
		t.Fatalf("%+v", doc)
	}
	if got := doc.render(); !strings.HasSuffix(got, "- works_on [actor=p-2]Project Y[/actor]\nmy own note\n") {
		t.Fatalf("render %q", got)
	}
	if _, err := parseFacts("- [a] b\nnot a fact"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("facts: %v", err)
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
