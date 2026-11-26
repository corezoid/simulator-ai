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
// observation's text or a relation's target, as memory_get shows them); ForgetAll erases everything.
type MemorySaveInput struct {
	MemoryInput
	Text      string   `json:"text,omitempty"`
	Forget    []string `json:"forget,omitempty"`
	ForgetAll bool     `json:"forgetAll,omitempty"`
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

// SaveMemory adds and erases facts in the person's memory. sim-api writes the change
// (actor/memory/save) — the block of their twin's description and the relations' edges from the twin,
// marked as the memory's — in one transaction under the lock its automatic memory takes, so neither
// loses the other's lines; the relation lines follow the graph. Facts already there are not added
// twice; a relation names its actor by id ([[actor:<id>]], a chip) or by title ([[Title]]: the one
// actor of that title, else it stays as a text fact).
func (c *Client) SaveMemory(ctx context.Context, in MemorySaveInput) (Memory, error) {
	if e := consoleOnly(c); e != nil {
		return Memory{}, e
	}
	if strings.TrimSpace(in.Text) == "" && len(in.Forget) == 0 && !in.ForgetAll || utf8.RuneCountInString(in.Text) > 4000 || len(in.Forget) > maxMemoryItems {
		return Memory{}, errors.New("text (facts to remember, up to 4000 characters), forget (up to 50) or forgetAll required")
	}
	adding, err := parseFacts(in.Text)
	if err != nil {
		return Memory{}, err
	}
	if len(adding.Notes)+len(adding.Relations)+len(in.Forget) == 0 && !in.ForgetAll {
		return Memory{}, errors.New("text holds no fact: one per line, \"- [category] fact #tag\" or \"- relation_type [[Actor]]\"")
	}
	if err = memoryLimits(adding, in.Forget); err != nil {
		return Memory{}, err
	}
	first, userID, err := c.twin(ctx, in.MemoryInput)
	if err != nil {
		return Memory{}, err
	}
	// The legacy data.aiMemory moves into the block first; the lines forget names are not moved.
	var legacyLeft string
	if strings.TrimSpace(first.Legacy) != "" && !in.ForgetAll {
		legacyLeft = c.moveLegacyMemory(ctx, first, in.Forget)
	}
	saved, err := c.memorySave(ctx, first.ID, adding, in.Forget, in.ForgetAll)
	if err != nil {
		return Memory{}, err
	}
	held := saved.doc()
	out := Memory{ActorID: first.ID, UserID: userID, Added: saved.Added, Removed: saved.Removed, Notes: held.Notes, Relations: held.Relations}
	if strings.TrimSpace(first.Legacy) != "" && legacyLeft != first.Legacy {
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
	Type    string `json:"type,omitempty"`
	ActorID string `json:"actorId,omitempty"`
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

func (m memorySaved) doc() memoryDoc {
	doc := newMemoryDoc()
	doc.Notes = append(doc.Notes, m.Notes...)
	doc.Relations = append(doc.Relations, relationsOf(m.Relations)...)
	return doc
}

// memorySave sends one change to actor/memory/save: sim-api merges it into the block as it is now.
func (c *Client) memorySave(ctx context.Context, twin string, adding memoryDoc, forget []string, forgetAll bool) (memorySaved, error) {
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
			// A chip or [[actor:<id>]] names the actor; a [[Title]] is resolved by sim-api to the one
			// actor of that title the caller sees, or kept as a text fact.
			typ := r.Type
			if typ == "gantt" {
				typ = ganttEdgeType // the Gantt dependency, as every other tool names it
			}
			relations[i] = memoryRelationWire{Type: typ, ActorID: r.ActorID, Title: capRunes(r.Target, maxFactRunes)}
		}
		params["relations"] = relations
	}
	if len(forget) > 0 {
		params["forget"] = forget
	}
	if forgetAll {
		params["forgetAll"] = true
	}
	saved, err := call[map[string]any, memorySaved](ctx, c, "actor/memory/save", params)
	if err != nil {
		return memorySaved{}, fmt.Errorf("write the memory on your actor: %w", err)
	}
	return saved, nil
}

// moveLegacyMemory writes what the legacy data.aiMemory holds into the memory (actor/memory/save, in
// batches the method takes) and answers what the key keeps — the lines that did not move, rendered: a
// note longer than the method takes, a batch sim-api refused (a relation to an actor that is gone,
// the block full). The lines forget names are dropped, not moved. A refusal never fails the person's
// save: what did not move stays in the key.
func (c *Client) moveLegacyMemory(ctx context.Context, t twinState, forget []string) string {
	held, _ := aimemory.Forget(aimemory.Parse(t.Legacy), forget)
	moving, kept := newMemoryDoc(), newMemoryDoc()
	for _, n := range held.Notes {
		n.Category, n.Text = aimemory.Category(n.Category), aimemory.CleanFact(n.Text)
		switch {
		case n.Text == "":
		case utf8.RuneCountInString(n.Text) > maxFactRunes:
			kept.Notes = append(kept.Notes, n)
		default:
			moving.Notes = append(moving.Notes, n)
		}
	}
	for len(moving.Notes) > 0 {
		batch := newMemoryDoc()
		batch.Notes = moving.Notes[:min(len(moving.Notes), maxMemoryItems)]
		moving.Notes = moving.Notes[len(batch.Notes):]
		if _, err := c.memorySave(ctx, t.ID, batch, nil, false); err != nil {
			kept.Notes = append(kept.Notes, batch.Notes...)
		}
	}
	// Each relation on its own: one to an actor that is gone keeps only itself in the key.
	for _, r := range held.Relations {
		one := newMemoryDoc()
		one.Relations = append(one.Relations, r)
		if _, err := c.memorySave(ctx, t.ID, one, nil, false); err != nil {
			kept.Relations = append(kept.Relations, r)
		}
	}
	return kept.Render()
}
