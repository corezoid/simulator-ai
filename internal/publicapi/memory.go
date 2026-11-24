package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Personal memory is what a person asked the agent to remember about them, kept on their twin actor
// (the System actor of their user, actor/by-system_obj/get), so it has the twin's rights and the
// person sees and erases it there; nothing is held outside the graph. It is written in Basic
// Memory's micro-syntax, one fact per line:
//
//	- [preference] Answers in Ukrainian #language
//	- works_on [[Project X]]
//
// An observation ("- [category] fact #tag") is stored as such in data.aiMemory, a Markdown text; a
// relation ("- <edge type> [[Actor title]]" or "[[actor:<id>]]") becomes an edge from the twin to
// that actor too, of a known edge type.

// memoryKey is the twin's data key of the memory text.
const memoryKey = "aiMemory"

const maxMemoryBytes = 32 << 10

var (
	noteLine     = regexp.MustCompile(`^\s*[-*]\s*\[([^\]\[]{1,64})\]\s*(.+?)\s*$`)
	relationLine = regexp.MustCompile(`^\s*[-*]\s*([A-Za-z_][A-Za-z0-9_-]{0,254})\s*\[\[(.+?)\]\]\s*$`)
	tagPattern   = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_-]{1,64})`)
)

// MemoryNote is one observation.
type MemoryNote struct {
	Category string   `json:"category"`
	Text     string   `json:"text"`
	Tags     []string `json:"tags,omitempty"`
}

// MemoryRelation is one typed link to an actor.
type MemoryRelation struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	ActorID string `json:"actorId,omitempty"`
}

// memoryDoc is the parsed memory: observations and relations in their order.
type memoryDoc struct {
	Notes     []MemoryNote     `json:"notes"`
	Relations []MemoryRelation `json:"relations"`
}

// parseMemory reads the micro-syntax; any other non-empty line is refused with its number.
func parseMemory(text string) (memoryDoc, error) {
	doc := memoryDoc{Notes: []MemoryNote{}, Relations: []MemoryRelation{}}
	for i, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := relationLine.FindStringSubmatch(line); m != nil {
			r := MemoryRelation{Type: m[1], Target: strings.TrimSpace(m[2])}
			// [[actor:<id>|Title]]: a stored relation keeps its actor and the title it was named by;
			// [[actor:<id>]] names its actor alone (forget must still find its edge).
			ref, title, titled := strings.Cut(r.Target, "|")
			if id, ok := strings.CutPrefix(strings.TrimSpace(ref), "actor:"); ok {
				r.ActorID = strings.TrimSpace(id)
				if titled {
					r.Target = strings.TrimSpace(title)
				}
			}
			doc.Relations = append(doc.Relations, r)
			continue
		}
		m := noteLine.FindStringSubmatch(line)
		if m == nil {
			return memoryDoc{}, fmt.Errorf("line %d: %q is neither \"- [category] fact #tag\" nor \"- relation_type [[Actor]]\"", i+1, strings.TrimSpace(line))
		}
		note := MemoryNote{Category: strings.TrimSpace(m[1]), Text: m[2]}
		for _, t := range tagPattern.FindAllStringSubmatch(m[2], -1) {
			note.Tags = append(note.Tags, t[1])
		}
		doc.Notes = append(doc.Notes, note)
	}
	return doc, nil
}

// text writes the memory back in the micro-syntax; a relation keeps its resolved actor id.
func (d memoryDoc) text() string {
	var b strings.Builder
	for _, n := range d.Notes {
		fmt.Fprintf(&b, "- [%s] %s\n", n.Category, n.Text)
	}
	for _, r := range d.Relations {
		target := r.Target
		if r.ActorID != "" && !strings.HasPrefix(r.Target, "actor:") {
			target = "actor:" + r.ActorID + "|" + r.Target
		}
		fmt.Fprintf(&b, "- %s [[%s]]\n", r.Type, target)
	}
	return b.String()
}

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
	Added     int              `json:"added,omitempty"`
	Removed   int              `json:"removed,omitempty"`
	Edges     int              `json:"edges,omitempty"`
}

// twin finds the person's twin actor and its memory text.
func (c *Client) twin(ctx context.Context, in MemoryInput) (string, int64, string, error) {
	// Whose memory is the credential's to say, never the model's: a run's JWT names its user. The
	// argument only helps find them when the member list is closed to the caller (no viewUsers),
	// and is then proven by the twin's own access rule carrying the token's SuperAdmin id.
	who, err := c.selfUserID(ctx, in.SelfUserID)
	if err != nil {
		return "", 0, "", err
	}
	if !who.Trusted && who.SAID == 0 {
		return "", 0, "", errors.New("personal memory needs a credential that names its user (a Simulator login or an AI Console run)")
	}
	if who.ID < 1 {
		return "", 0, "", errors.New("selfUserId required: the user id the run acts for (your credential cannot list the members to find it)")
	}
	a, err := call[map[string]any, struct {
		ID   string                     `json:"id"`
		Data map[string]json.RawMessage `json:"data"`
	}](ctx, c, "actor/by-system_obj/get", map[string]any{"accId": c.workspace(), "objType": "user", "objId": strconv.FormatInt(who.ID, 10), "fields": []string{"id", "data"}})
	if err != nil {
		return "", 0, "", fmt.Errorf("your actor (user %d): %w", who.ID, err)
	}
	if !who.Trusted {
		if mine, err := c.isParticipant(ctx, a.ID, who); err != nil || !mine {
			return "", 0, "", fmt.Errorf("selfUserId %d is not the user your credential names", who.ID)
		}
	}
	var text string
	if raw := a.Data[memoryKey]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &text)
	}
	return a.ID, who.ID, text, nil
}

// GetMemory reads a person's memory from their twin.
func (c *Client) GetMemory(ctx context.Context, in MemoryInput) (Memory, error) {
	if e := consoleOnly(c); e != nil {
		return Memory{}, e
	}
	actorID, userID, text, err := c.twin(ctx, in)
	if err != nil {
		return Memory{}, err
	}
	doc, err := parseMemory(text)
	if err != nil {
		// Edited by hand: hand it back as one note, bounded as a saved memory is.
		raw := strings.TrimSpace(text)
		if len(raw) > maxMemoryBytes {
			raw = strings.ToValidUTF8(raw[:maxMemoryBytes], "") + " …"
		}
		doc = memoryDoc{Notes: []MemoryNote{{Category: "unparsed", Text: raw}}, Relations: []MemoryRelation{}}
	}
	return Memory{ActorID: actorID, UserID: userID, Notes: doc.Notes, Relations: doc.Relations}, nil
}

// SaveMemory adds and erases facts on the person's twin; a new relation is also an edge from the
// twin to its actor. Facts already there are not added twice.
func (c *Client) SaveMemory(ctx context.Context, in MemorySaveInput) (Memory, error) {
	if e := consoleOnly(c); e != nil {
		return Memory{}, e
	}
	if strings.TrimSpace(in.Text) == "" && len(in.Forget) == 0 || utf8.RuneCountInString(in.Text) > 4000 || len(in.Forget) > 50 {
		return Memory{}, errors.New("text (facts to remember, up to 4000 characters) or forget (up to 50) required")
	}
	adding, err := parseMemory(in.Text)
	if err != nil {
		return Memory{}, err
	}
	actorID, userID, text, err := c.twin(ctx, in.MemoryInput)
	if err != nil {
		return Memory{}, err
	}
	doc, err := parseMemory(text)
	if err != nil {
		return Memory{}, fmt.Errorf("the memory on your actor is not in the micro-syntax any more (edited by hand?); fix data.%s first: %w", memoryKey, err)
	}
	out := Memory{ActorID: actorID, UserID: userID}
	held := len(doc.Notes) + len(doc.Relations)
	var dropped []MemoryRelation
	doc, dropped = forget(doc, in.Forget)
	out.Removed = held - len(doc.Notes) - len(doc.Relations)
	for _, n := range adding.Notes {
		if !hasNote(doc, n) {
			doc.Notes, out.Added = append(doc.Notes, n), out.Added+1
		}
	}
	// Everything is resolved and checked before anything is written: a bad relation or an
	// oversized memory leaves the twin and its edges as they were.
	var fresh []MemoryRelation
	for _, r := range adding.Relations {
		if r.ActorID, err = c.relationTarget(ctx, r.Target); err != nil {
			return Memory{}, err
		}
		if !hasRelation(doc, r) {
			fresh = append(fresh, r)
			doc.Relations = append(doc.Relations, r)
		}
	}
	draw, err := c.relationEdges(ctx, actorID, fresh)
	if err != nil {
		return Memory{}, err
	}
	out.Added += len(fresh)
	saved := doc.text()
	if len(saved) > maxMemoryBytes {
		return Memory{}, fmt.Errorf("the memory would be %d bytes, more than %d: forget something first", len(saved), maxMemoryBytes)
	}
	if err = c.forgetEdges(ctx, actorID, dropped); err != nil {
		return Memory{}, err
	}
	if len(draw) > 0 {
		if _, err = c.atomicItems(ctx, draw); err != nil {
			return Memory{}, fmt.Errorf("draw the relations, the memory text was not saved: %w", err)
		}
		out.Edges = len(draw)
	}
	value, _ := json.Marshal(map[string]string{memoryKey: saved})
	if _, err = call[map[string]any, json.RawMessage](ctx, c, "actor/update", map[string]any{"accId": c.workspace(), "actorId": actorID, "data": json.RawMessage(value), "fields": []string{"id"}}); err != nil {
		return Memory{}, fmt.Errorf("write the memory on your actor: %w", err)
	}
	out.Notes, out.Relations = doc.Notes, doc.Relations
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
		if m.Type == r.Type && (m.ActorID == r.ActorID || strings.EqualFold(m.Target, r.Target)) {
			return true
		}
	}
	return false
}

// relationTarget resolves [[actor:<id>]] or [[Title]] (one actor of exactly that title) to an id.
func (c *Client) relationTarget(ctx context.Context, target string) (string, error) {
	if id, ok := strings.CutPrefix(target, "actor:"); ok {
		got, err := call[map[string]any, actorBrief](ctx, c, "actor/get", map[string]any{"accId": c.workspace(), "actorId": strings.TrimSpace(id), "fields": []string{"id"}})
		if err != nil || got.ID == "" {
			return "", fmt.Errorf("[[%s]]: no such actor you see", target)
		}
		return got.ID, nil
	}
	page, err := call[map[string]any, struct {
		List []actorBrief `json:"list"`
	}](ctx, c, "actor/search/list", map[string]any{"accId": c.workspace(), "query": target, "searchType": "text", "limit": 20, "fields": []string{"id", "title", "formId"}})
	if err != nil {
		return "", fmt.Errorf("[[%s]]: %w", target, err)
	}
	var hits []actorBrief
	for _, a := range page.List {
		if sameName(a.Title, target) {
			hits = append(hits, a)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0].ID, nil
	case 0:
		return "", fmt.Errorf("[[%s]]: no actor has this title; name it [[actor:<id>]] (actors_search)", target)
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	return "", fmt.Errorf("[[%s]]: %d actors have this title (%s): name one as [[actor:<id>]]", target, len(hits), strings.Join(ids, ", "))
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
