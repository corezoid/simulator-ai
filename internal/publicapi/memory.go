package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
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
	// Unparsed are lines of the section that are not the micro-syntax (the person's own); kept.
	Unparsed []string `json:"unparsed,omitempty"`
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

// rereadTwin reads the twin again right before a write, from the primary.
func (c *Client) rereadTwin(ctx context.Context, id string) (twinState, error) {
	a, err := call[map[string]any, twinWire](ctx, c, "actor/get", map[string]any{"accId": c.workspace(), "actorId": id, "fields": twinFields})
	if err != nil {
		return twinState{}, fmt.Errorf("read your actor again before writing: %w", err)
	}
	return a.state(), nil
}

// memoryOf is the twin's memory: the section with what the legacy key still holds merged in.
func memoryOf(t twinState) memoryDoc {
	doc := readSection(t.Desc)
	if strings.TrimSpace(t.Legacy) != "" {
		legacy := parseSection(t.Legacy)
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
		doc.Other = append(doc.Other, legacy.Other...)
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
	return Memory{ActorID: t.ID, UserID: userID, Notes: doc.Notes, Relations: doc.Relations, Unparsed: doc.Other, Legacy: strings.TrimSpace(t.Legacy) != ""}, nil
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
	doc, dropped = forget(doc, m.forget)
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

// SaveMemory adds and erases facts in the person's memory section. Facts already there are not
// added twice; a new relation is also an edge from the twin, a forgotten one loses its edge. The
// twin is read again right before the write and the change merged into what it holds then, so a
// section sim-api wrote meanwhile (after a dialog) is kept, not overwritten.
func (c *Client) SaveMemory(ctx context.Context, in MemorySaveInput) (Memory, error) {
	if e := consoleOnly(c); e != nil {
		return Memory{}, e
	}
	if strings.TrimSpace(in.Text) == "" && len(in.Forget) == 0 || utf8.RuneCountInString(in.Text) > 4000 || len(in.Forget) > 50 {
		return Memory{}, errors.New("text (facts to remember, up to 4000 characters) or forget (up to 50) required")
	}
	adding, err := parseFacts(in.Text)
	if err != nil {
		return Memory{}, err
	}
	first, userID, err := c.twin(ctx, in.MemoryInput)
	if err != nil {
		return Memory{}, err
	}
	// Everything is resolved and checked before anything is written: a bad relation or an
	// oversized memory leaves the twin and its edges as they were.
	for i := range adding.Relations {
		r := &adding.Relations[i]
		if r.ActorID, r.Target, err = c.relationTarget(ctx, *r); err != nil {
			return Memory{}, err
		}
	}
	change := memoryChange{adding: adding, forget: in.Forget}
	doc, fresh, dropped, _, _ := change.apply(first)
	if _, err = writeSection(first.Desc, doc); err != nil {
		return Memory{}, err
	}
	draw, err := c.relationEdges(ctx, first.ID, fresh)
	if err != nil {
		return Memory{}, err
	}
	if err = c.forgetEdges(ctx, first.ID, dropped); err != nil {
		return Memory{}, err
	}
	out := Memory{ActorID: first.ID, UserID: userID}
	if len(draw) > 0 {
		if _, err = c.atomicItems(ctx, draw); err != nil {
			return Memory{}, fmt.Errorf("draw the relations, the memory was not saved: %w", err)
		}
		out.Edges = len(draw)
	}
	// The edges are the slow part; the twin is read again only now, and the change merged into
	// what its description holds at this moment.
	t, err := c.rereadTwin(ctx, first.ID)
	if err != nil {
		return Memory{}, err
	}
	doc, _, _, out.Added, out.Removed = change.apply(t)
	desc, err := writeSection(t.Desc, doc)
	if err != nil {
		return Memory{}, err
	}
	update := map[string]any{"accId": c.workspace(), "actorId": t.ID, "description": desc, "fields": []string{"id"}}
	if strings.TrimSpace(t.Legacy) != "" {
		update["data"] = map[string]any{legacyMemoryKey: nil} // moved into the description
	}
	if desc != t.Desc || update["data"] != nil {
		if _, err = call[map[string]any, json.RawMessage](ctx, c, "actor/update", update); err != nil {
			return Memory{}, fmt.Errorf("write the memory on your actor: %w", err)
		}
	}
	out.Notes, out.Relations, out.Unparsed = doc.Notes, doc.Relations, doc.Other
	return out, nil
}

// forget drops the notes and relations names name and answers the relations it dropped.
func forget(doc memoryDoc, names []string) (memoryDoc, []MemoryRelation) {
	gone := map[string]bool{}
	for _, n := range names {
		gone[strings.ToLower(strings.TrimSpace(n))] = true
	}
	notes := doc.Notes[:0:0]
	for _, n := range doc.Notes {
		if !gone[strings.ToLower(n.Text)] {
			notes = append(notes, n)
		}
	}
	var dropped []MemoryRelation
	relations := doc.Relations[:0:0]
	for _, r := range doc.Relations {
		if gone[strings.ToLower(r.Target)] || r.ActorID != "" && (gone[strings.ToLower(r.ActorID)] || gone["actor:"+strings.ToLower(r.ActorID)]) {
			dropped = append(dropped, r)
			continue
		}
		relations = append(relations, r)
	}
	doc.Notes, doc.Relations = notes, relations
	return doc, dropped
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
		return fmt.Errorf("delete the forgotten relations' edges, nothing was saved: %w", err)
	}
	for _, r := range results {
		if r.err != nil && r.err.Reason != "notFound" && r.err.Reason != "noFoundOrNoAccess" {
			return fmt.Errorf("delete a forgotten relation's edge, nothing was saved: %w", r.err)
		}
	}
	return nil
}

func hasNote(doc memoryDoc, n MemoryNote) bool {
	for _, m := range doc.Notes {
		if strings.EqualFold(m.Category, n.Category) && strings.EqualFold(m.Text, n.Text) {
			return true
		}
	}
	return false
}

func hasRelation(doc memoryDoc, r MemoryRelation) bool {
	for _, m := range doc.Relations {
		if m.Type == r.Type && (m.ActorID != "" && m.ActorID == r.ActorID || m.ActorID == "" && strings.EqualFold(m.Target, r.Target)) {
			return true
		}
	}
	return false
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
