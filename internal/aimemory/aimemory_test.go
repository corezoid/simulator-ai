package aimemory

import (
	"strings"
	"testing"
)

// The same vectors as sim-api's e_ai_memory/save_test.go: an explicit save forgets first, then adds
// what is not there; the person's text and everything outside the memory lines stay byte for byte.
func TestSaveForgetsThenAdds(t *testing.T) {
	const tail = "\nMy checklist:\n- [x] buy milk\n\n[h3]Links[/h3]\nB"
	desc := "Intro\n\n[h3]AI memory[/h3]\n- [role] Юрист\n- [preference] Short answers #style\n" +
		"- works_on [actor=p1]Project X[/actor]\n- interaction [actor=p2]ACME[/actor]" + tail
	out, saved, err := Save(desc,
		[]Note{{Category: "role", Text: "Senior lawyer"}, {Category: "preference", Text: "short ANSWERS #style"}},
		[]Relation{{Type: "interaction", ActorID: "a3", Target: CleanFact("Kyiv [EU]\nbranch")}, {Type: "works_on", ActorID: "p1", Target: "renamed"}},
		[]string{"юрист", "ACTOR:P2", " ", "no such line"})
	want := "Intro\n\n[h3]AI memory[/h3]\n- [preference] Short answers #style\n- [role] Senior lawyer\n" +
		"- works_on [actor=p1]Project X[/actor]\n- interaction [actor=a3]Kyiv (EU) branch[/actor]" + tail
	if err != nil || out != want {
		t.Fatalf("got  %q\nwant %q (%v)", out, want, err)
	}
	if saved.Added != 2 || saved.Removed != 2 || len(saved.Forgotten) != 1 || saved.Forgotten[0].ActorID != "p2" {
		t.Fatalf("%+v", saved)
	}
	if again, saved, _ := Save(out, []Note{{Category: "role", Text: "Senior lawyer"}}, nil, nil); again != out || saved.Added != 0 {
		t.Fatalf("the same save twice changed %q", again)
	}
	gone, saved, err := Save(out, nil, nil, []string{"project x", "A3", "Short answers #style", "Senior lawyer"})
	if err != nil || saved.Removed != 4 || len(saved.Forgotten) != 2 || gone != "Intro\n\nMy checklist:\n- [x] buy milk\n\n[h3]Links[/h3]\nB" {
		t.Fatalf("emptied %q %+v %v", gone, saved, err)
	}
}

func TestSaveNewBlockAndLimit(t *testing.T) {
	if out, _, err := Save("# Agent\nkeep", []Note{{Category: "role", Text: "CFO"}}, nil, nil); err != nil || out != "# Agent\nkeep\n\n[h3]AI memory[/h3]\n- [role] CFO\n" {
		t.Fatalf("%q %v", out, err)
	}
	desc := "[h3]AI memory[/h3]\n- [role] CFO\n"
	out, _, err := Save(desc, []Note{{Category: "note", Text: strings.Repeat("x", MaxBytes)}}, nil, nil)
	if err == nil || out != desc {
		t.Fatalf("an explicit save dropped nothing to fit: %q %v", out, err)
	}
}
