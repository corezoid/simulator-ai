package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// planActor is one actor of the patch, resolved.
type planActor struct {
	in     OntologyActor
	formID int64
	data   json.RawMessage
	desc   string
	// id is the actor's id when it exists: named (existing), or created by an earlier call (exists).
	id         string
	status     string // existing, exists or create (created once phase a ran)
	title      string
	candidates []OntologyCandidate
}

// planEdge is an edge between two keys with its type resolved.
type planEdge struct {
	from, to, name string
	typeID         int64
}

// planAccount is an account to open on an actor of the patch.
type planAccount struct {
	actor string
	pair  AccountPair
	kind  string
}

// ontologyPlan is the patch resolved against the workspace: everything phase a and b need, and every
// problem found, so one dry run answers all of them at once.
type ontologyPlan struct {
	in        OntologyInput
	actors    []*planActor
	byKey     map[string]*planActor
	edges     []planEdge
	accounts  []planAccount
	pairs     map[[2]string]AccountPair
	templates map[int64]template
	problems  []string
	warnings  []string
	// sides holds each transaction's posting-side convention (resolveSides), for the dry run.
	sides map[string]string
}

func (p *ontologyPlan) problem(format string, args ...any) {
	p.problems = append(p.problems, fmt.Sprintf(format, args...))
}

// ganttEdgeType is the type the Gantt view draws a dependency with (ui-kit GanttCommands).
const ganttEdgeType = "directedInteraction"

// ledgerDrawnEdge says the edge type is one a ledger effect draws, never the agent.
func ledgerDrawnEdge(name string) bool {
	return strings.HasPrefix(name, "transfersAmount") || strings.HasPrefix(name, "transfersFormField")
}

// planOntology resolves the patch. Reads only.
func (c *Client) planOntology(ctx context.Context, in OntologyInput) (*ontologyPlan, error) {
	p := &ontologyPlan{in: in, byKey: map[string]*planActor{}, pairs: map[[2]string]AccountPair{}, templates: map[int64]template{}}
	for _, a := range in.Actors {
		pa := &planActor{in: a, formID: a.FormID, title: a.Title, desc: a.Desc}
		p.actors = append(p.actors, pa)
		p.byKey[a.Key] = pa
	}
	steps := []func(context.Context, *Client) error{p.resolveTemplates, p.resolveExisting, p.resolveSources, p.resolveEdges, p.recognize, p.resolveAccounts, p.resolveSides}
	for _, step := range steps {
		if err := step(ctx, c); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// resolveTemplates finds each new actor's template and checks its data against it.
func (p *ontologyPlan) resolveTemplates(ctx context.Context, c *Client) error {
	byTitle := map[string]int64{}
	for _, a := range p.actors {
		if a.in.Existing != "" {
			continue
		}
		if a.formID == 0 {
			id, ok := byTitle[strings.ToLower(a.in.Form)]
			if !ok {
				var err error
				if id, err = c.findTemplate(ctx, a.in.Form); err != nil {
					p.problem("actor %q: %v", a.in.Key, err)
					continue
				}
				byTitle[strings.ToLower(a.in.Form)] = id
			}
			a.formID = id
		}
		t, ok := p.templates[a.formID]
		if !ok {
			var err error
			if t, err = c.readTemplate(ctx, a.formID); err != nil {
				p.problem("actor %q: template %d: %v", a.in.Key, a.formID, err)
				continue
			}
			p.templates[a.formID] = t
		}
		data, warnings, err := t.checkData(a.in.Data)
		if err != nil {
			p.problem("actor %q: %v", a.in.Key, err)
			continue
		}
		for _, w := range warnings {
			p.warnings = append(p.warnings, fmt.Sprintf("actor %q: %s", a.in.Key, w))
		}
		if a.data, err = withAliases(data, a.in.Aliases); err != nil {
			return err
		}
	}
	return nil
}

// aliasesKey is the data key an actor's other names are kept under (a list of strings). It is not a
// template field: recognition finds it with actor/filter/list ($all), and data is not in full-text
// search (owner's decision).
const aliasesKey = "aliases"

func withAliases(data json.RawMessage, aliases []string) (json.RawMessage, error) {
	if len(aliases) == 0 {
		return data, nil
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	clean := make([]string, 0, len(aliases))
	for _, a := range aliases {
		clean = append(clean, strings.TrimSpace(a))
	}
	values[aliasesKey], _ = json.Marshal(clean)
	return json.Marshal(values)
}

type actorBrief struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	FormID int64  `json:"formId"`
}

// resolveExisting checks every existing actor is there and visible to the caller.
func (p *ontologyPlan) resolveExisting(ctx context.Context, c *Client) error {
	var named []*planActor
	var params []any
	for _, a := range p.actors {
		if a.in.Existing != "" {
			named = append(named, a)
			params = append(params, map[string]any{"accId": c.workspace(), "actorId": a.in.Existing, "fields": []string{"id", "title", "formId"}})
		}
	}
	if len(named) == 0 {
		return nil
	}
	results, err := c.eachItems(ctx, "actor/get", params)
	if err != nil {
		return err
	}
	for i, a := range named {
		var got actorBrief
		if results[i].err != nil || json.Unmarshal(results[i].raw, &got) != nil || got.ID == "" {
			p.problem("actor %q: existing %s is not found or not visible to you", a.in.Key, a.in.Existing)
			continue
		}
		a.id, a.status, a.title, a.formID = got.ID, "existing", got.Title, got.FormID
	}
	return nil
}

// resolveSources turns each source into a chip at the end of the description: a file chip by its
// fileName, an actor chip, or a quote of a reaction (a comment).
func (p *ontologyPlan) resolveSources(ctx context.Context, c *Client) error {
	for _, a := range p.actors {
		if a.in.Source == "" {
			continue
		}
		kind, ref, ok := strings.Cut(a.in.Source, ":")
		ref = strings.TrimSpace(ref)
		if !ok || ref == "" || strings.ContainsAny(ref, "[]") {
			p.problem("actor %q: source is attachment:<fileName>, actor:<actorId> or reaction:<reactionId>", a.in.Key)
			continue
		}
		var chip string
		switch kind {
		case "attachment":
			chip = fmt.Sprintf("[file=%s]%s[/file]", ref, ref)
		case "actor", "reaction":
			got, err := call[map[string]any, struct {
				actorBrief
				Description string `json:"description"`
			}](ctx, c, "actor/get", map[string]any{"accId": c.workspace(), "actorId": ref, "fields": []string{"id", "title", "description"}})
			if err != nil {
				p.problem("actor %q: source %s is not found or not visible to you", a.in.Key, a.in.Source)
				continue
			}
			// A comment is an actor too: an actor chip is in the description's tag set, unlike
			// [quote] (a message chip of reactions and event cards only), and backlinks find it.
			label := got.Title
			if kind == "reaction" {
				text := got.Description
				if strings.TrimSpace(text) == "" {
					text = got.Title
				}
				label = "comment: " + excerpt(text, 80)
			}
			label = strings.NewReplacer("[", "(", "]", ")").Replace(strings.TrimSpace(label))
			if label == "" {
				label = kind
			}
			chip = fmt.Sprintf("[actor=%s]%s[/actor]", ref, label)
		default:
			p.problem("actor %q: source kind %q: attachment, actor or reaction", a.in.Key, kind)
			continue
		}
		if a.desc != "" {
			a.desc += "\n\n"
		}
		a.desc += "Source: " + chip
	}
	return nil
}

// resolveEdges resolves every edge type: parents are hierarchy edges, gantt is the Gantt
// dependency type, and the edges a transfer draws are refused.
func (p *ontologyPlan) resolveEdges(ctx context.Context, c *Client) error {
	types := map[string]int64{}
	typeID := func(name string) (int64, bool) {
		if id, ok := types[name]; ok {
			return id, id > 0
		}
		t, err := call[edgeTypeRequest, edgeTypeWire](ctx, c, "edge_type/get", edgeTypeRequest{AccID: c.workspace(), Name: name})
		types[name] = t.ID
		if err != nil || t.ID < 1 {
			types[name] = 0
			return 0, false
		}
		return t.ID, true
	}
	for _, a := range p.actors {
		if a.in.Parent == "" {
			continue
		}
		id, ok := typeID("hierarchy")
		if !ok {
			return errors.New("the hierarchy edge type is not readable")
		}
		p.edges = append(p.edges, planEdge{from: a.in.Parent, to: a.in.Key, typeID: id})
	}
	for _, e := range p.in.Edges {
		name := e.Type
		switch name {
		case "", "parent":
			name = "hierarchy"
		case "gantt":
			name = ganttEdgeType
		}
		if ledgerDrawnEdge(name) {
			p.problem("edge %s -> %s: a %s edge is drawn by a transfer, never created: put the movement into transfers (from and to legs) and the edge appears by itself", e.From, e.To, name)
			continue
		}
		id, ok := typeID(name)
		if !ok {
			p.problem("edge %s -> %s: edge type %q is unknown (edge_types_list; hierarchy, gantt, interaction, directedInteraction)", e.From, e.To, e.Type)
			continue
		}
		p.edges = append(p.edges, planEdge{from: e.From, to: e.To, name: e.Name, typeID: id})
	}
	return nil
}

var markup = regexp.MustCompile(`\[[^\]]*\]`)

// excerpt is a text's first words without its markup, at most n characters.
func excerpt(text string, n int) string {
	plain := strings.Join(strings.Fields(markup.ReplaceAllString(text, " ")), " ")
	if r := []rune(plain); len(r) > n {
		return strings.TrimSpace(string(r[:n-1])) + "…"
	}
	return plain
}
