package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/corezoid/simulator-ai/internal/aimemory"
)

// Personal memory is what a person asked the agent to remember about them, kept as a section of
// their twin actor's description (memory_section.go): the twin of their user
// (actor/by-system_obj/get), so it has the twin's rights and the person sees and edits it on their
// own actor; nothing is held outside the graph. A relation is also an edge from the twin.

// legacyMemoryKey is the twin's data key the memory was kept under before it moved into the
// description; a save moves what it holds into the section and clears it.
const legacyMemoryKey = "aiMemory"

// MemoryInput names whose memory: the person the run acts for (selfUserId as in chat_open; a
// Simulator JWT names them itself).
type MemoryInput struct {
	SelfUserID int64 `json:"selfUserId,omitempty"`
}

// MemorySaveInput adds facts (Text, in the micro-syntax) and erases the ones Forget names (an
// observation's text or a relation's target, as memory_get shows them).
type MemorySaveInput struct {
	MemoryInput
	Text   string   `json:"text,omitempty"`
	Forget []string `json:"forget,omitempty"`
}

// Memory is a person's memory as stored on their twin.
type Memory struct {
	ActorID   string           `json:"actorId"`
	UserID    int64            `json:"userId"`
	Notes     []MemoryNote     `json:"notes"`
	Relations []MemoryRelation `json:"relations"`
	// Legacy says data.aiMemory still holds facts: the next memory_save moves them.
	Legacy  bool `json:"legacy,omitempty"`
	Added   int  `json:"added,omitempty"`
	Removed int  `json:"removed,omitempty"`
	Edges   int  `json:"edges,omitempty"`
}

// twinState is the twin as one save reads it: its description and the legacy memory text.
type twinState struct {
	ID, Desc, Legacy string
}

type twinWire struct {
	ID          string                     `json:"id"`
	Description string                     `json:"description"`
	Data        map[string]json.RawMessage `json:"data"`
}

func (w twinWire) state() twinState {
	t := twinState{ID: w.ID, Desc: w.Description}
	if raw := w.Data[legacyMemoryKey]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &t.Legacy)
	}
	return t
}

var twinFields = []string{"id", "description", "data." + legacyMemoryKey}

// twin finds the person's twin actor.
func (c *Client) twin(ctx context.Context, in MemoryInput) (twinState, int64, error) {
	// Whose memory is the credential's to say, never the model's: a run's JWT names its user. The
	// argument only helps find them when the member list is closed to the caller (no viewUsers),
	// and is then proven by the twin's own access rule carrying the token's SuperAdmin id.
	who, err := c.selfUserID(ctx, in.SelfUserID)
	if err != nil {
		return twinState{}, 0, err
	}
	if !who.Trusted && who.SAID == 0 {
		return twinState{}, 0, errors.New("personal memory needs a credential that names its user (a Simulator login or an AI Console run)")
	}
	if who.ID < 1 {
		return twinState{}, 0, errors.New("selfUserId required: the user id the run acts for (your credential cannot list the members to find it)")
	}
	a, err := call[map[string]any, twinWire](ctx, c, "actor/by-system_obj/get", map[string]any{"accId": c.workspace(), "objType": "user", "objId": strconv.FormatInt(who.ID, 10), "fields": twinFields})
	if err != nil {
		return twinState{}, 0, fmt.Errorf("your actor (user %d): %w", who.ID, err)
	}
	if !who.Trusted {
		if mine, err := c.isParticipant(ctx, a.ID, who); err != nil || !mine {
			return twinState{}, 0, fmt.Errorf("selfUserId %d is not the user your credential names", who.ID)
		}
	}
	return a.state(), who.ID, nil
}

// memoryOf is the twin's memory: the section with what the legacy key still holds merged in.
func memoryOf(t twinState) memoryDoc {
	doc := readSection(t.Desc)
	if strings.TrimSpace(t.Legacy) != "" {
		legacy := aimemory.Parse(t.Legacy)
		for _, n := range legacy.Notes {
			if !hasNote(doc, n) {
				doc.Notes = append(doc.Notes, n)
			}
		}
		for _, r := range legacy.Relations {
			if !hasRelation(doc, r) {
				doc.Relations = append(doc.Relations, r)
			}
		}
	}
	return doc
}

// GetMemory reads a person's memory from their twin. Read-only: a legacy data.aiMemory is shown
// merged in and moved into the description by the next memory_save.
func (c *Client) GetMemory(ctx context.Context, in MemoryInput) (Memory, error) {
	if e := consoleOnly(c); e != nil {
		return Memory{}, e
	}
	t, userID, err := c.twin(ctx, in)
	if err != nil {
		return Memory{}, err
	}
	doc := memoryOf(t)
	return Memory{ActorID: t.ID, UserID: userID, Notes: doc.Notes, Relations: doc.Relations, Legacy: strings.TrimSpace(t.Legacy) != ""}, nil
}

// memoryChange is one save's change, applied to whatever the twin holds when it is written.
type memoryChange struct {
	adding memoryDoc
	forget []string
}

// apply merges the change into the twin's memory: forget, then add what is not there yet.
func (m memoryChange) apply(t twinState) (doc memoryDoc, fresh, dropped []MemoryRelation, added, removed int) {
	doc = memoryOf(t)
	held := len(doc.Notes) + len(doc.Relations)
	doc, dropped = aimemory.Forget(doc, m.forget)
	removed = held - len(doc.Notes) - len(doc.Relations)
	for _, n := range m.adding.Notes {
		if !hasNote(doc, n) {
			doc.Notes, added = append(doc.Notes, n), added+1
		}
	}
	for _, r := range m.adding.Relations {
		if !hasRelation(doc, r) {
			doc.Relations, fresh = append(doc.Relations, r), append(fresh, r)
		}
	}
	return doc, fresh, dropped, added + len(fresh), removed
}

// SaveMemory adds and erases facts in the person's memory block. sim-api merges the change
// (actor/memory/save) into the block as it is at the write, under the same locks as its automatic
// memory after a dialog, so neither loses the other's lines. Facts already there are not added
// twice; a new relation is also an edge from the twin, a forgotten one loses its edge.
func (c *Client) SaveMemory(ctx context.Context, in MemorySaveInput) (Memory, error) {
	if e := consoleOnly(c); e != nil {
		return Memory{}, e
	}
	if strings.TrimSpace(in.Text) == "" && len(in.Forget) == 0 || utf8.RuneCountInString(in.Text) > 4000 || len(in.Forget) > maxMemoryItems {
		return Memory{}, errors.New("text (facts to remember, up to 4000 characters) or forget (up to 50) required")
	}
	adding, err := parseFacts(in.Text)
	if err != nil {
		return Memory{}, err
	}
	if len(adding.Notes)+len(adding.Relations)+len(in.Forget) == 0 {
		return Memory{}, errors.New("text holds no fact: one per line, \"- [category] fact #tag\" or \"- relation_type [[Actor]]\"")
	}
	if err = memoryLimits(adding, in.Forget); err != nil {
		return Memory{}, err
	}
	first, userID, err := c.twin(ctx, in.MemoryInput)
	if err != nil {
		return Memory{}, err
	}
	// Everything is resolved and checked before anything is written: a bad relation, an unknown
	// edge type or an oversized memory leaves the twin and its edges as they were.
	for i := range adding.Relations {
		r := &adding.Relations[i]
		if r.ActorID, r.Target, err = c.relationTarget(ctx, *r); err != nil {
			return Memory{}, err
		}
	}
	change := memoryChange{adding: adding, forget: in.Forget}
	doc, fresh, _, _, _ := change.apply(first)
	if _, err = writeSection(first.Desc, doc); err != nil {
		return Memory{}, err
	}
	draw, err := c.relationEdges(ctx, first.ID, fresh)
	if err != nil {
		return Memory{}, err
	}
	// The legacy data.aiMemory moves into the block first, so forget reaches its lines too.
	legacyLeft, legacy, err := c.moveLegacyMemory(ctx, first)
	if err != nil {
		return Memory{}, err
	}
	saved, err := c.memorySave(ctx, first.ID, adding, in.Forget)
	if err != nil {
		return Memory{}, err
	}
	// The edges follow what was written: a new relation's edge is drawn, every relation the save
	// forgot (sim-api's lines among them) loses its own.
	out := Memory{ActorID: first.ID, UserID: userID, Added: saved.Added, Removed: saved.Removed,
		Notes: saved.doc().Notes, Relations: saved.doc().Relations}
	if len(draw) > 0 {
		if _, err = c.atomicItems(ctx, draw); err != nil {
			return Memory{}, fmt.Errorf("the memory was saved, but its relations' edges were not drawn: %w", err)
		}
		out.Edges = len(draw)
	}
	if err = c.forgetEdges(ctx, first.ID, saved.forgotten()); err != nil {
		return Memory{}, fmt.Errorf("the memory was saved, but: %w", err)
	}
	if legacy {
		var left any // nil clears the key
		if legacyLeft != "" {
			left = legacyLeft
		}
		update := map[string]any{"accId": c.workspace(), "actorId": first.ID, "data": map[string]any{legacyMemoryKey: left}, "fields": []string{"id"}}
		if _, err = call[map[string]any, json.RawMessage](ctx, c, "actor/update", update); err != nil {
			return Memory{}, fmt.Errorf("the memory was saved, but the legacy memory on your actor was not cleared: %w", err)
		}
	}
	return out, nil
}

// maxMemoryItems is what one actor/memory/save takes of notes, of relations and of forget;
// maxFactRunes is a fact's or a title's length there.
const (
	maxMemoryItems = 50
	maxFactRunes   = 1000
)

// capRunes cuts a title to n characters (an actor's title has no bound; the method's has).
func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// memoryLimits refuses a change actor/memory/save would refuse, before anything is written.
func memoryLimits(adding memoryDoc, forget []string) error {
	if len(adding.Notes) > maxMemoryItems || len(adding.Relations) > maxMemoryItems {
		return fmt.Errorf("at most %d facts and %d relations in one save", maxMemoryItems, maxMemoryItems)
	}
	for _, n := range adding.Notes {
		if utf8.RuneCountInString(n.Text) > maxFactRunes {
			return fmt.Errorf("a fact is at most %d characters: %.40q…", maxFactRunes, n.Text)
		}
	}
	for _, f := range forget {
		if utf8.RuneCountInString(f) > maxFactRunes {
			return fmt.Errorf("a forget entry is at most %d characters", maxFactRunes)
		}
	}
	return nil
}

// memoryRelationWire is a relation as actor/memory/save takes and answers it.
type memoryRelationWire struct {
	Type    string `json:"type"`
	ActorID string `json:"actorId"`
	Title   string `json:"title,omitempty"`
}

// memoryNoteWire is a note as actor/memory/save takes it: its tags are in its text already.
type memoryNoteWire struct {
	Category string `json:"category"`
	Text     string `json:"text"`
}

// memorySaved is actor/memory/save's answer: the memory after the save.
type memorySaved struct {
	Notes     []MemoryNote         `json:"notes"`
	Relations []memoryRelationWire `json:"relations"`
	Added     int                  `json:"added"`
	Removed   int                  `json:"removed"`
	Forgotten []memoryRelationWire `json:"forgotten"`
}

func relationsOf(wire []memoryRelationWire) []MemoryRelation {
	out := make([]MemoryRelation, len(wire))
	for i, r := range wire {
		out[i] = MemoryRelation{Type: r.Type, ActorID: r.ActorID, Target: r.Title}
	}
	return out
}

func (m memorySaved) forgotten() []MemoryRelation { return relationsOf(m.Forgotten) }

func (m memorySaved) doc() memoryDoc {
	doc := newMemoryDoc()
	doc.Notes = append(doc.Notes, m.Notes...)
	doc.Relations = append(doc.Relations, relationsOf(m.Relations)...)
	return doc
}

// memorySave sends one change to actor/memory/save: sim-api merges it into the block as it is now.
func (c *Client) memorySave(ctx context.Context, twin string, adding memoryDoc, forget []string) (memorySaved, error) {
	params := map[string]any{"accId": c.workspace(), "actorId": twin}
	if len(adding.Notes) > 0 {
		notes := make([]memoryNoteWire, len(adding.Notes))
		for i, n := range adding.Notes {
			notes[i] = memoryNoteWire{Category: n.Category, Text: n.Text}
		}
		params["notes"] = notes
	}
	if len(adding.Relations) > 0 {
		relations := make([]memoryRelationWire, len(adding.Relations))
		for i, r := range adding.Relations {
			relations[i] = memoryRelationWire{Type: r.Type, ActorID: r.ActorID, Title: capRunes(r.Target, maxFactRunes)}
		}
		params["relations"] = relations
	}
	if len(forget) > 0 {
		params["forget"] = forget
	}
	saved, err := call[map[string]any, memorySaved](ctx, c, "actor/memory/save", params)
	if err != nil {
		return memorySaved{}, fmt.Errorf("write the memory on your actor: %w", err)
	}
	return saved, nil
}

// moveLegacyMemory writes what the legacy data.aiMemory holds into the block (actor/memory/save, in
// batches the method takes) and answers what the key keeps: the relations no actor this credential
// sees answers to — a deleted actor's chip, an unknown title — rendered as lines, "" when all moved.
// legacy is false without a legacy text.
func (c *Client) moveLegacyMemory(ctx context.Context, t twinState) (left string, legacy bool, err error) {
	if strings.TrimSpace(t.Legacy) == "" {
		return "", false, nil
	}
	held := aimemory.Parse(t.Legacy)
	moving, kept := newMemoryDoc(), newMemoryDoc()
	for _, n := range held.Notes {
		n.Category, n.Text = aimemory.Category(n.Category), aimemory.CleanFact(n.Text)
		if n.Text != "" {
			moving.Notes = append(moving.Notes, n)
		}
	}
	for _, r := range held.Relations {
		id, title, err := c.relationTarget(ctx, r)
		if err != nil {
			kept.Relations = append(kept.Relations, r)
			continue
		}
		r.ActorID, r.Target = id, title
		moving.Relations = append(moving.Relations, r)
	}
	for len(moving.Notes)+len(moving.Relations) > 0 {
		batch := newMemoryDoc()
		batch.Notes = moving.Notes[:min(len(moving.Notes), maxMemoryItems)]
		batch.Relations = moving.Relations[:min(len(moving.Relations), maxMemoryItems)]
		moving.Notes, moving.Relations = moving.Notes[len(batch.Notes):], moving.Relations[len(batch.Relations):]
		if _, err := c.memorySave(ctx, t.ID, batch, nil); err != nil {
			return "", true, fmt.Errorf("move the legacy memory: %w", err)
		}
	}
	return kept.Render(), true, nil
}

// forgetEdges deletes the edges of forgotten relations from the twin; one already gone is fine.
func (c *Client) forgetEdges(ctx context.Context, twin string, dropped []MemoryRelation) error {
	var params []any
	for _, r := range dropped {
		if r.ActorID == "" {
			continue
		}
		name := r.Type
		if name == "gantt" {
			name = ganttEdgeType
		}
		t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.workspace(), Name: name})
		if err != nil || t.ID < 1 {
			continue // a type gone since: no edge of it to delete
		}
		params = append(params, map[string]any{"accId": c.workspace(), "source": twin, "target": r.ActorID, "edgeTypeId": t.ID})
	}
	if len(params) == 0 {
		return nil
	}
	results, err := c.eachItems(ctx, "edge/by-node/delete", params)
	if err != nil {
		return fmt.Errorf("delete the forgotten relations' edges: %w", err)
	}
	for _, r := range results {
		if r.err != nil && r.err.Reason != "notFound" && r.err.Reason != "noFoundOrNoAccess" {
			return fmt.Errorf("delete a forgotten relation's edge: %w", r.err)
		}
	}
	return nil
}

// relationTarget resolves a relation's actor and title: a chip or [[actor:<id>]] by its id, [[Title]]
// by the one actor of exactly that title.
func (c *Client) relationTarget(ctx context.Context, r MemoryRelation) (string, string, error) {
	if r.ActorID != "" {
		got, err := call[map[string]any, actorBrief](ctx, c, "actor/get", map[string]any{"accId": c.workspace(), "actorId": r.ActorID, "fields": []string{"id", "title"}})
		if err != nil || got.ID == "" {
			return "", "", fmt.Errorf("actor %s: no such actor you see", r.ActorID)
		}
		return got.ID, got.Title, nil
	}
	target := r.Target
	page, err := call[map[string]any, struct {
		List []actorBrief `json:"list"`
	}](ctx, c, "actor/search/list", map[string]any{"accId": c.workspace(), "query": target, "searchType": "text", "limit": 20, "fields": []string{"id", "title", "formId"}})
	if err != nil {
		return "", "", fmt.Errorf("[[%s]]: %w", target, err)
	}
	var hits []actorBrief
	for _, a := range page.List {
		if sameName(a.Title, target) {
			hits = append(hits, a)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0].ID, hits[0].Title, nil
	case 0:
		return "", "", fmt.Errorf("[[%s]]: no actor has this title; name it [[actor:<id>]] (actors_search)", target)
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	return "", "", fmt.Errorf("[[%s]]: %d actors have this title (%s): name one as [[actor:<id>]]", target, len(hits), strings.Join(ids, ", "))
}

// relationEdges is an edge/create of each relation's type from the twin to its actor, for one
// atomic batch; an unknown or ledger-drawn type is refused before anything is written.
func (c *Client) relationEdges(ctx context.Context, twin string, relations []MemoryRelation) ([]atomicItem, error) {
	items := make([]atomicItem, len(relations))
	for i, r := range relations {
		name := r.Type
		if name == "gantt" {
			name = ganttEdgeType
		}
		if ledgerDrawnEdge(name) {
			return nil, fmt.Errorf("%s: an edge a transfer draws is not a relation", r.Type)
		}
		t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.workspace(), Name: name})
		if err != nil || t.ID < 1 {
			return nil, fmt.Errorf("relation %q: a relation type is an edge type of the workspace (edge_types_list: hierarchy, interaction, directedInteraction, …)", r.Type)
		}
		items[i] = atomicItem{Method: "edge/create", Params: edgeCreateRequest{AccID: c.workspace(), Source: twin, Target: r.ActorID, EdgeTypeID: t.ID, ForceDirection: true}}
	}
	return items, nil
}
