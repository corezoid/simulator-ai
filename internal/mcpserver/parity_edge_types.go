package mcpserver

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// The walks (graph_traverse, graph_path_find) and graph_get answer edges with a numeric
// edgeTypeId only. A model asked about the Gantt chart then guessed what type 3 was (eval
// multihop-08). Their answers now carry edgeTypes [{id, name, role}] for the types they hold, read
// from ontology/get (one small read: the base types; a per-account money type is named by its
// weight). role: hierarchy, dependency (the Gantt link: source = predecessor), transactions (money),
// accountTree, system, custom.

type edgeTypeName struct {
	ID   int64  `json:"id"`
	Name string `json:"name,omitempty"`
	Role string `json:"role,omitempty"`
}

// namedEdgeTypes adds edgeTypes to an answer whose edges carry edgeTypeId. An edge type the base
// list lacks is a per-account one: with a weight it is a money edge (role transactions). A failed
// read leaves the answer as it is.
func namedEdgeTypes(ctx context.Context, c *publicapi.Client, v json.RawMessage) json.RawMessage {
	var doc jsonObj
	var page struct {
		Edges []struct {
			EdgeTypeID int64           `json:"edgeTypeId"`
			Weight     json.RawMessage `json:"weight"`
		} `json:"edges"`
	}
	if json.Unmarshal(v, &doc) != nil || json.Unmarshal(v, &page) != nil || len(page.Edges) == 0 {
		return v
	}
	var ids []int64
	money := map[int64]bool{}
	for _, e := range page.Edges {
		if e.EdgeTypeID > 0 && !slices.Contains(ids, e.EdgeTypeID) {
			ids = append(ids, e.EdgeTypeID)
		}
		if w := strings.Trim(string(e.Weight), `"`); w != "" && w != "null" {
			money[e.EdgeTypeID] = true
		}
	}
	if len(ids) == 0 {
		return v
	}
	raw, err := c.Pass(ctx, "ontology/get", map[string]json.RawMessage{"fields": json.RawMessage(`["edgeTypes"]`), "limit": json.RawMessage("1")})
	var known struct {
		EdgeTypes []edgeTypeName `json:"edgeTypes"`
	}
	if err != nil || json.Unmarshal(raw, &known) != nil || len(known.EdgeTypes) == 0 {
		return v // every workspace has hierarchy: an empty list is no digest
	}
	out := make([]edgeTypeName, 0, len(ids))
	for _, id := range ids {
		t := edgeTypeName{ID: id}
		if i := slices.IndexFunc(known.EdgeTypes, func(k edgeTypeName) bool { return k.ID == id }); i >= 0 {
			t = known.EdgeTypes[i]
		} else if money[id] {
			t.Role = "transactions"
		}
		out = append(out, t)
	}
	doc["edgeTypes"], _ = json.Marshal(out)
	named, err := json.Marshal(doc)
	if err != nil {
		return v
	}
	return named
}

// withEdgeTypes is the Around of the walks: the answer with its edge types named.
func withEdgeTypes(ctx context.Context, c *publicapi.Client, method string, args, _ map[string]json.RawMessage) (json.RawMessage, error) {
	v, err := c.Pass(ctx, method, args)
	if err != nil {
		return nil, err
	}
	return namedEdgeTypes(ctx, c, v), nil
}

// moneyEdgeGuard is the Around of edge_update and edge_endpoints_update: a money edge keeps its
// ends and its transfer (CheckEdgeRewrite).
func moneyEdgeGuard(ctx context.Context, c *publicapi.Client, method string, args, _ map[string]json.RawMessage) (json.RawMessage, error) {
	var edgeID string
	_ = json.Unmarshal(args["edgeId"], &edgeID)
	if err := c.CheckEdgeRewrite(ctx, edgeID, args, method == "edge/endpoints/update"); err != nil {
		return nil, err
	}
	return c.Pass(ctx, method, args)
}

// formCount is how many reached actors one template has.
type formCount struct {
	FormID    int64  `json:"formId"`
	FormTitle string `json:"formTitle,omitempty"`
	Count     int    `json:"count"`
}

// traverseAround is the Around of graph_traverse: its edge types named, and countsByForm — the
// reached actors (start actors and denied ones left out) per template, so "how many contracts" is
// read, not counted by the model (eval multihop-14 said 4 and listed 3).
func traverseAround(ctx context.Context, c *publicapi.Client, method string, args, local map[string]json.RawMessage) (json.RawMessage, error) {
	v, err := withEdgeTypes(ctx, c, method, args, local)
	if err != nil {
		return nil, err
	}
	return countsByForm(v), nil
}

func countsByForm(v json.RawMessage) json.RawMessage {
	var doc jsonObj
	var page struct {
		Nodes []struct {
			ID        string `json:"id"`
			FormID    int64  `json:"formId"`
			FormTitle string `json:"formTitle"`
			Depth     int    `json:"depth"`
			Denied    bool   `json:"denied"`
		} `json:"nodes"`
	}
	if json.Unmarshal(v, &doc) != nil || json.Unmarshal(v, &page) != nil {
		return v
	}
	var out []formCount
	seen := map[string]bool{}
	for _, n := range page.Nodes {
		if n.Depth == 0 || n.Denied || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		i := slices.IndexFunc(out, func(f formCount) bool { return f.FormID == n.FormID })
		if i < 0 {
			out, i = append(out, formCount{FormID: n.FormID, FormTitle: n.FormTitle}), len(out)
		}
		out[i].Count++
	}
	slices.SortStableFunc(out, func(a, b formCount) int { return b.Count - a.Count })
	if out == nil {
		out = []formCount{}
	}
	doc["countsByForm"], _ = json.Marshal(out)
	named, err := json.Marshal(doc)
	if err != nil {
		return v
	}
	return named
}
