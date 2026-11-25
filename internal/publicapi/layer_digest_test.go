package publicapi

import (
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
		card.Neighbors = append(card.Neighbors, struct {
			EdgeTypeID int64  `json:"edgeTypeId"`
			EdgeType   string `json:"edgeType"`
			Role       string `json:"role"`
			Direction  string `json:"direction"`
			HasMore    bool   `json:"hasMore"`
			Neighbors  []struct {
				ActorID string `json:"actorId"`
				Weight  string `json:"weight"`
			} `json:"neighbors"`
		}{Role: "hierarchy", Direction: "in", Neighbors: []struct {
			ActorID string `json:"actorId"`
			Weight  string `json:"weight"`
		}{{ActorID: c.parent}}})
		cards[c.id] = card
	}
	out := LayerDigest{Forms: []DigestForm{}, Levels: []DigestLevel{}, Actors: []DigestActor{}}
	summarise(&out, []string{"a", "b"}, cards, nil, false)
	if len(out.Levels) != 1 || out.Levels[0].Count != 2 || len(out.Forms) != 2 {
		t.Fatalf("levels %+v forms %+v", out.Levels, out.Forms)
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
