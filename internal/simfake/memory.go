package simfake

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/corezoid/simulator-ai/internal/aimemory"
)

func init() { kbMethods["actor/memory/save"] = (*Server).memorySave }

type memoryRelationWire struct {
	Type    string `json:"type"`
	ActorID string `json:"actorId"`
	Title   string `json:"title,omitempty"`
}

func memoryRelations(relations []aimemory.Relation) []memoryRelationWire {
	out := make([]memoryRelationWire, len(relations))
	for i, r := range relations {
		out[i] = memoryRelationWire{Type: r.Type, ActorID: r.ActorID, Title: r.Target}
	}
	return out
}

// memorySave is sim-api's actor/memory/save: the change merged into the twin's memory block as it is
// at the write (the request holds the server's lock, as sim-api holds the twin's), with its refusals
// the plugin relies on.
func (s *Server) memorySave(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ActorID string `json:"actorId"`
		Notes   []struct {
			Category string   `json:"category"`
			Text     string   `json:"text"`
			Tags     []string `json:"tags"`
		} `json:"notes"`
		Relations []memoryRelationWire `json:"relations"`
		Forget    []string             `json:"forget"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fail("invalidParams", err.Error())
	}
	a := s.Actors[p.ActorID]
	if a == nil {
		return nil, fail("notFound", "actor not found")
	}
	if !slices.Contains(slices.Collect(maps.Values(s.KB.Twins)), a.ID) {
		return nil, fail("invalidParams", "actorId: not a person's actor")
	}
	if len(p.Notes)+len(p.Relations)+len(p.Forget) == 0 {
		return nil, fail("invalidParams", "notes, relations or forget required")
	}
	if len(p.Notes) > 50 || len(p.Relations) > 50 || len(p.Forget) > 50 {
		return nil, fail("invalidParams", "validation for 'notes' failed on the 'max' tag")
	}
	notes := make([]aimemory.Note, 0, len(p.Notes))
	for _, n := range p.Notes {
		text := aimemory.CleanFact(n.Text)
		if text == "" || len([]rune(n.Text)) > 1000 {
			return nil, fail("invalidParams", "validation for 'text' failed")
		}
		for _, t := range n.Tags {
			text += " #" + strings.ToLower(t)
		}
		notes = append(notes, aimemory.Note{Category: aimemory.Category(n.Category), Text: text})
	}
	relations := make([]aimemory.Relation, 0, len(p.Relations))
	for _, r := range p.Relations {
		if s.Actors[r.ActorID] == nil {
			return nil, fail("notFound", "no found OR no access, actor")
		}
		relations = append(relations, aimemory.Relation{Type: r.Type, ActorID: r.ActorID, Target: aimemory.CleanFact(r.Title)})
	}
	out, saved, err := aimemory.Save(a.Description, notes, relations, p.Forget)
	if err != nil {
		return nil, fail("invalidParams", err.Error())
	}
	changed := out != a.Description
	a.Description = out
	held := aimemory.Read(out) // the tags as the text carries them
	return map[string]any{"actorId": a.ID, "notes": held.Notes, "relations": memoryRelations(held.Relations),
		"added": saved.Added, "removed": saved.Removed, "forgotten": memoryRelations(saved.Forgotten), "changed": changed}, nil
}
