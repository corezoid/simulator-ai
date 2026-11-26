package simfake

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

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

// memoryTypes are the relation types the fake knows (edge_type/get's), hierarchy the default.
var memoryTypes = map[string]int64{"hierarchy": 3, "flow": 4, "directedInteraction": 5, "interaction": 6}

// memorySave is sim-api's actor/memory/save (service_ai_memory): the change written into the twin's
// memory block as it is at the write (the request holds the server's lock, as sim-api holds the
// twin's), the relations as memory edges from the twin, the relation lines following the edges.
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
		ForgetAll bool                 `json:"forgetAll"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fail("invalidParams", err.Error())
	}
	a := s.Actors[p.ActorID]
	if a == nil {
		return nil, fail("notFound", "actor not found")
	}
	twins := slices.Collect(maps.Values(s.KB.Twins))
	if !slices.Contains(twins, a.ID) {
		return nil, fail("invalidParams", "actorId: not a person's actor")
	}
	if len(p.Notes)+len(p.Relations)+len(p.Forget) == 0 && !p.ForgetAll {
		return nil, fail("invalidParams", "notes, relations, forget or forgetAll required")
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
	var relations []aimemory.Relation
	for _, r := range p.Relations {
		typ := r.Type
		if typ == "" {
			typ = "hierarchy"
		}
		if memoryTypes[typ] == 0 {
			return nil, fail("invalidParams", "relation type "+typ+" is not one a person's actor may link with")
		}
		id, title := r.ActorID, aimemory.CleanFact(r.Title)
		if id != "" {
			if s.Actors[id] == nil {
				return nil, fail("notFound", "no found OR no access, actor")
			}
			if title == "" {
				title = s.Actors[id].Title
			}
		} else if id = s.titled(title, twins); id == "" {
			notes = append(notes, aimemory.Note{Category: "note", Text: aimemory.CleanFact(typ + " " + title)})
			continue
		}
		relations = append(relations, aimemory.Relation{Type: typ, ActorID: id, Target: title})
	}
	// sim-api writes block and edges in one transaction: a refused write leaves the edges as they were.
	edges, keys := maps.Clone(s.Edges), maps.Clone(s.EdgeKeys)
	doc := s.followGraph(a.ID, a.Description, aimemory.Read(a.Description), relations)
	held := len(doc.Notes) + len(doc.Relations)
	var forgotten []aimemory.Relation
	if p.ForgetAll {
		forgotten, doc = doc.Relations, aimemory.NewDoc()
	}
	doc, more := aimemory.Forget(doc, p.Forget)
	forgotten = append(forgotten, more...)
	for _, r := range forgotten {
		if !aimemory.HasRelation(aimemory.Doc{Relations: relations}, r) {
			s.dropMemoryEdge(a.ID, r)
		}
	}
	if p.ForgetAll {
		s.deleteEdges(func(e *Edge) bool { return e.Source == a.ID && e.AiSource == "memory" })
	}
	removed, added := held-len(doc.Notes)-len(doc.Relations), 0
	for _, n := range notes {
		if !aimemory.HasNote(doc, n) {
			doc.Notes, added = append(doc.Notes, n), added+1
		}
	}
	for _, r := range relations {
		if aimemory.HasRelation(doc, r) {
			continue
		}
		if !s.hasEdge(a.ID, r) {
			e := &Edge{ID: s.id("edge"), Source: a.ID, Target: r.ActorID, TypeID: memoryTypes[r.Type], AiSource: "memory"}
			s.Edges[e.ID], s.EdgeKeys[fmt.Sprintf("%s>%s|%d", e.Source, e.Target, e.TypeID)] = e, e.ID
		}
		doc.Relations, added = append(doc.Relations, r), added+1
	}
	out, err := aimemory.Write(a.Description, doc)
	if err != nil {
		s.Edges, s.EdgeKeys = edges, keys
		return nil, fail("invalidParams", err.Error())
	}
	changed := out != a.Description
	a.Description = out
	res := aimemory.Read(out) // the tags as the text carries them
	return map[string]any{"actorId": a.ID, "notes": res.Notes, "relations": memoryRelations(res.Relations),
		"added": added, "removed": removed, "forgotten": memoryRelations(forgotten), "changed": changed}, nil
}

// titled is sim-api's resolution: the one actor of exactly that title (case aside, no twin), else the
// one whose title holds the name as whole words; "" for none or several.
func (s *Server) titled(title string, twins []string) string {
	var exact, words []string
	for _, a := range s.Actors {
		if slices.Contains(twins, a.ID) {
			continue
		}
		got := aimemory.CleanFact(a.Title)
		switch {
		case strings.EqualFold(got, title):
			exact = append(exact, a.ID)
		case holdsWords(got, title):
			words = append(words, a.ID)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0]
	case len(exact) == 0 && len(words) == 1:
		return words[0]
	}
	return ""
}

func holdsWords(title, name string) bool {
	split := func(s string) []string {
		return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	}
	t, n := split(title), split(name)
	for i := 0; len(n) > 0 && i+len(n) <= len(t); i++ {
		if slices.Equal(t[i:i+len(n)], n) {
			return true
		}
	}
	return false
}

// followGraph: a memory edge whose actor the description names nowhere goes (sim-api searches the
// whole description, not the parsed block); a line whose edge is gone drops.
func (s *Server) followGraph(twin, desc string, stored aimemory.Doc, adding []aimemory.Relation) aimemory.Doc {
	s.deleteEdges(func(e *Edge) bool {
		if e.Source != twin || e.AiSource != "memory" {
			return false
		}
		named := strings.Contains(desc, "[actor="+e.Target+"]") || strings.Contains(desc, "actor:"+e.Target)
		return !named && !slices.ContainsFunc(adding, func(r aimemory.Relation) bool { return r.ActorID == e.Target && memoryTypes[r.Type] == e.TypeID })
	})
	doc := aimemory.Doc{Notes: stored.Notes, Relations: []aimemory.Relation{}}
	for _, r := range stored.Relations {
		if r.ActorID == "" || s.hasEdge(twin, r) {
			doc.Relations = append(doc.Relations, r)
		}
	}
	return doc
}

func (s *Server) hasEdge(twin string, r aimemory.Relation) bool {
	return s.Edges[s.EdgeKeys[fmt.Sprintf("%s>%s|%d", twin, r.ActorID, memoryTypes[r.Type])]] != nil
}

func (s *Server) dropMemoryEdge(twin string, r aimemory.Relation) {
	s.deleteEdges(func(e *Edge) bool {
		return e.Source == twin && e.Target == r.ActorID && e.TypeID == memoryTypes[r.Type] && e.AiSource == "memory"
	})
}

// deleteEdges deletes the edges gone says, with their natural keys.
func (s *Server) deleteEdges(gone func(*Edge) bool) {
	for id, e := range s.Edges {
		if gone(e) {
			delete(s.Edges, id)
			delete(s.EdgeKeys, fmt.Sprintf("%s>%s|%d", e.Source, e.Target, e.TypeID))
		}
	}
}
