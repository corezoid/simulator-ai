package publicapi

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/simfake"
)

// A saved view is an ActorFilters actor whose filter holds the editor's q, unescaped; it is found by
// title and run by the server's filterActorId.
func TestSavedViewsOnActorFilters(t *testing.T) {
	c, f, _ := kbClient(t)
	ctx := context.Background()
	f.AddActor(12, "Contract A", nil)
	f.AddActor(12, "Contract B", nil)
	view, err := c.CreateSavedView(ctx, SavedViewCreateInput{Title: "kb-test signed", FormID: 12, Status: []string{"verified"}, OrderBy: "updatedAt", OrderValue: "ASC",
		Where: [][]ViewCondition{{{Field: "status", Op: "=", Value: "Signed"}, {Field: "amount", Op: ">=", Value: "1000"}}, {{Field: "status", Op: "=", Value: "draft"}}}})
	if err != nil || view.Q != "status=signed&amount>=1000||status=draft" {
		t.Fatalf("%+v %v", view, err)
	}
	stored, _ := f.ActorData(view.ViewID)
	filter, _ := stored.Data["filter"].(string)
	for _, want := range []string{`"formId":12`, `"q":"status=signed&amount>=1000||status=draft"`, `"status":["verified"]`, `"orderBy":"updatedAt"`, `"selectedForms":[12]`} {
		if !strings.Contains(filter, want) || stored.FormID != 20 {
			t.Errorf("filter %s (form %d) lacks %s", filter, stored.FormID, want)
		}
	}
	found, err := c.FindSavedViews(ctx, SavedViewsFindInput{Query: "signed", FormID: 12})
	if err != nil || len(found.List) != 1 || found.List[0].ViewID != view.ViewID || found.HasMore {
		t.Fatalf("find %+v %v", found, err)
	}
	if other, _ := c.FindSavedViews(ctx, SavedViewsFindInput{FormID: 13}); len(other.List) != 0 {
		t.Fatalf("another template's views: %+v", other)
	}
	rows, err := c.RunSavedView(ctx, SavedViewRunInput{ViewID: view.ViewID, Count: true, Limit: 1})
	if err != nil || !strings.Contains(string(rows.List), "Contract A") || !rows.HasMore || rows.Total == nil || *rows.Total != 2 {
		t.Fatalf("run %s %+v %v", rows.List, rows, err)
	}
	for _, bad := range []struct {
		in   SavedViewCreateInput
		want string
	}{
		{SavedViewCreateInput{Title: "v", FormID: 12, Where: [][]ViewCondition{{{Field: "stauts", Op: "=", Value: "x"}}}}, `field "stauts" is not in template`},
		{SavedViewCreateInput{Title: "v", FormID: 12, Where: [][]ViewCondition{{{Field: "status", Op: "=", Value: "a&b"}}}}, `cannot hold & or |`},
		{SavedViewCreateInput{Title: "v", FormID: 12, Where: [][]ViewCondition{{{Field: "status", Op: "=", Value: "a%26b"}}}}, `cannot hold & or |`},
		{SavedViewCreateInput{Title: "v", FormID: 12, Where: [][]ViewCondition{{{Field: "amount", Op: "<", Value: "=5"}}}}, `cannot start with =`},
		{SavedViewCreateInput{Title: "v", FormID: 12, Where: [][]ViewCondition{{{Field: "status", Op: "~", Value: "a"}}}}, `op is one of`},
		{SavedViewCreateInput{Title: "v", FormID: 12, Where: [][]ViewCondition{{}}}, `an empty group`},
		{SavedViewCreateInput{Title: "v", FormID: 12, OrderBy: "balance"}, `balance needs accountNameId`},
		{SavedViewCreateInput{Title: "v", FormID: 12, OrderByKey: "nope"}, `orderByKey "nope" is not a field`},
		{SavedViewCreateInput{Title: "v", FormID: 12, Status: []string{"done"}}, `status is one of`},
		{SavedViewCreateInput{Title: "v", FormID: 99}, `template 99`},
		{SavedViewCreateInput{FormID: 12}, `title`},
	} {
		if _, err := c.CreateSavedView(ctx, bad.in); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: %v", bad.in, err)
		}
	}
	if _, err := c.RunSavedView(ctx, SavedViewRunInput{ViewID: view.ViewID, Limit: 101}); err == nil {
		t.Error("limit 101 accepted")
	}
}

// The person's memory lives on their twin in the micro-syntax; a relation is an edge too, a repeat
// adds nothing, forget erases, and another syntax is refused with its line.
func TestPersonalMemoryOnTheTwin(t *testing.T) {
	anonymous, f, project := kbClient(t)
	ctx := context.Background()
	twin := f.TwinOf(7, 70)
	f.TwinOf(8, 80)
	// The run's JWT names SuperAdmin user 70 (member 7): whose memory is its to say.
	c, err := New(Config{Origin: f.URL, Workspace: "ws", Profile: "console-full", Authorization: "Simulator x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"70","nick":"Ann"}`)) + ".y"})
	if err != nil {
		t.Fatal(err)
	}
	self := MemoryInput{SelfUserID: 7}
	if _, err := anonymous.GetMemory(ctx, self); err == nil || !strings.Contains(err.Error(), "names its user") {
		t.Fatalf("a credential that names nobody read user 7's memory: %v", err)
	}
	got, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- [preference] Answers in Ukrainian #language\n\n- interaction [[Project X]]"})
	if err != nil || got.Added != 2 || got.Edges != 1 || len(got.Notes) != 1 || got.Notes[0].Tags[0] != "language" || got.ActorID != twin {
		t.Fatalf("%+v %v", got, err)
	}
	stored, _ := f.ActorData(twin)
	if stored.Description != "[h3]AI memory[/h3]\n- [preference] Answers in Ukrainian #language\n- interaction [actor="+project+"]Project X[/actor]\n" {
		t.Fatalf("stored %q", stored.Description)
	}
	edge := false
	f.Set(func(s *simfake.Server) {
		for _, e := range s.Edges {
			edge = edge || e.Source == twin && e.Target == project && e.TypeID == 6
		}
	})
	if !edge {
		t.Fatal("no interaction edge from the twin")
	}
	again, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- [Preference] answers in ukrainian #language\n- interaction [[actor:" + project + "]]"})
	if err != nil || again.Added != 0 || again.Edges != 0 { // the same note, and [[actor:id]] is the same actor
		t.Fatalf("repeat %+v %v", again, err)
	}
	read, err := c.GetMemory(ctx, self)
	if err != nil || len(read.Relations) != 1 || read.Relations[0].ActorID != project || read.Relations[0].Target != "Project X" {
		t.Fatalf("get %+v %v", read, err)
	}
	forgot, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Forget: []string{"Answers in Ukrainian #language", "actor:" + project}})
	if err != nil || forgot.Removed != 2 || len(forgot.Notes) != 0 {
		t.Fatalf("forget %+v %v", forgot, err)
	}
	f.Set(func(s *simfake.Server) {
		for _, e := range s.Edges {
			if e.Source == twin {
				t.Errorf("the forgotten relation's edge stays: %+v", e)
			}
		}
	})
	// A relation named [[actor:<id>]] alone is stored so, and forgetting it deletes its edge too.
	if got, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- interaction [[actor:" + project + "]]"}); err != nil || got.Edges != 1 {
		t.Fatalf("by id %+v %v", got, err)
	}
	if forgot, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Forget: []string{"actor:" + project}}); err != nil || forgot.Removed != 1 {
		t.Fatalf("forget by id %+v %v", forgot, err)
	}
	f.Set(func(s *simfake.Server) {
		for _, e := range s.Edges {
			if e.Source == twin {
				t.Errorf("the edge of a relation named by id stays: %+v", e)
			}
		}
	})
	for _, bad := range []struct {
		in   MemorySaveInput
		want string
	}{
		{MemorySaveInput{MemoryInput: self, Text: "I like tea"}, `line 1: "I like tea" is neither`},
		{MemorySaveInput{MemoryInput: self, Text: "- likes [[Project X]]"}, `a relation type is an edge type`},
		{MemorySaveInput{MemoryInput: self, Text: "- interaction [[Nobody]]"}, `no actor has this title`},
		{MemorySaveInput{MemoryInput: self, Text: "- interaction [[actor:missing]]"}, `no such actor you see`},
		{MemorySaveInput{MemoryInput: self, Text: "- transfersAmount_x [[Project X]]"}, `an edge a transfer draws`},
		{MemorySaveInput{MemoryInput: self}, `text (facts to remember`},
		{MemorySaveInput{MemoryInput: MemoryInput{SelfUserID: 8}, Text: "- [a] b"}, `selfUserId 8 is not you: you are user 7`},
	} {
		if _, err := c.SaveMemory(ctx, bad.in); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: %v", bad.in, err)
		}
	}
	// Without viewUsers the argument finds the twin, and the twin's own rule must carry the token's
	// SuperAdmin id: another member's id is refused, the person's own is served.
	f.Set(func(s *simfake.Server) { s.KB.NoViewUsers = true })
	if _, err := c.GetMemory(ctx, MemoryInput{SelfUserID: 8}); err == nil || !strings.Contains(err.Error(), "is not the user your credential names") {
		t.Fatalf("another member's memory read: %v", err)
	}
	if _, err := c.GetMemory(ctx, MemoryInput{}); err == nil || !strings.Contains(err.Error(), "selfUserId required") {
		t.Fatalf("no selfUserId: %v", err)
	}
	if own, err := c.GetMemory(ctx, self); err != nil || own.ActorID != twin {
		t.Fatalf("own memory: %+v %v", own, err)
	}
	f.Set(func(s *simfake.Server) { s.KB.NoViewUsers = false })
	f.AddActor(14, "Project X", nil) // a second actor of the title
	if _, err := c.SaveMemory(ctx, MemorySaveInput{MemoryInput: self, Text: "- interaction [[Project X]]"}); err == nil || !strings.Contains(err.Error(), "2 actors have this title") {
		t.Errorf("ambiguous title: %v", err)
	}
}
