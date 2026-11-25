package publicapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// A hierarchy cycle on the layer leaves no actor at level 0: its levels are still counted; forms
// with one title stay apart by id; a full digest keeps within its byte budget, top levels first.
func TestLayerDigestSummary(t *testing.T) {
	cards := map[string]digestCard{}
	for _, c := range []struct{ id, parent, form string }{{"a", "b", "1"}, {"b", "a", "2"}} {
		var card digestCard
		card.Actor.Title, card.Actor.FormTitle = c.id, "Same"
		card.Actor.FormID = int64(c.form[0] - '0')
		card.Neighbors = append(card.Neighbors, digestGroup{Role: "hierarchy", Direction: "in", Neighbors: []digestNeighbor{{ActorID: c.parent}}})
		cards[c.id] = card
	}
	out := LayerDigest{Forms: []DigestForm{}, Levels: []DigestLevel{}, Actors: []DigestActor{}}
	summarise(&out, []string{"a", "b"}, cards, nil, false)
	if len(out.Levels) != 1 || out.Levels[0].Count != 2 || len(out.Forms) != 2 {
		t.Fatalf("levels %+v forms %+v", out.Levels, out.Forms)
	}
	// A person's memory edge (twin -> project) is no structure: the project stays a top level.
	memory := map[string]digestCard{"p": {Neighbors: []digestGroup{{Role: "hierarchy", Direction: "in",
		Neighbors: []digestNeighbor{{ActorID: "twin", AiSource: "memory"}}}}}, "twin": {}}
	levels := LayerDigest{Forms: []DigestForm{}, Levels: []DigestLevel{}, Actors: []DigestActor{}}
	summarise(&levels, []string{"p", "twin"}, memory, nil, false)
	for _, a := range levels.Actors {
		if a.Level != 0 {
			t.Fatalf("a memory edge made a level: %+v", levels.Actors)
		}
	}
	big := LayerDigest{}
	for i := 0; i < 400; i++ {
		big.Actors = append(big.Actors, DigestActor{ID: strings.Repeat("x", 1000)})
	}
	fitDigest(&big)
	if len(big.Actors) == 0 || len(big.Actors) == 400 || !strings.Contains(strings.Join(big.Notes, ""), "too long") {
		t.Fatalf("%d actors, notes %v", len(big.Actors), big.Notes)
	}
}

// The budget holds when the transfers, not the actors, make a full digest too long.
func TestLayerDigestFitsTransfers(t *testing.T) {
	big := LayerDigest{}
	for i := 0; i < 3000; i++ {
		big.Transfers = append(big.Transfers, DigestTransfer{From: strings.Repeat("f", 60), To: strings.Repeat("t", 60), Amount: "1", Count: 1})
	}
	fitDigest(&big)
	b, _ := json.Marshal(big)
	if len(b) > digestMaxBytes || len(big.Transfers) == 0 || !strings.Contains(strings.Join(big.Notes, ""), "transfer lines") {
		t.Fatalf("%d bytes, %d transfers, notes %v", len(b), len(big.Transfers), big.Notes)
	}
}
