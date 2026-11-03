package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/corezoid/simulator-ai/contracts"
)

func (c *Client) ListGraphEdgeTypes(ctx context.Context, p GraphEdgeTypesInput) (GraphEdgeTypes, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return GraphEdgeTypes{}, errors.New("invalid edge type input")
	}
	if err = contracts.ValidateGraphInput("edge_type/list", raw); err != nil {
		return GraphEdgeTypes{}, err
	}
	if p.Limit == nil {
		limit := int64(50)
		p.Limit = &limit
	}
	filter := p.Filter
	// Always validate full workspace/identity fields before returning the requested
	// projection; no model-supplied field selector widens the backend request.
	p.Filter = ""
	wire, err := graphCall[GraphEdgeTypesInput, graphEdgeTypesWire](ctx, c, "edge_type/list", p)
	if err != nil {
		return GraphEdgeTypes{}, err
	}
	if wire.List == nil || int64(len(wire.List)) > *p.Limit {
		return GraphEdgeTypes{}, errors.New("invalid edge type page")
	}
	selected := map[string]bool{}
	for _, name := range strings.Split(filter, ",") {
		selected[name] = true
	}
	result := GraphEdgeTypes{List: make([]GraphEdgeType, 0, len(wire.List))}
	previous := int64(0)
	for _, item := range wire.List {
		if item.ID <= previous || strings.TrimSpace(item.Name) == "" || item.AccID != c.config.Workspace || item.IsSystem == nil || item.IsTree == nil || item.CreatedAt == nil || (p.IsSystem != nil && *item.IsSystem != *p.IsSystem) {
			return GraphEdgeTypes{}, errors.New("invalid edge type identity, order or filter result")
		}
		previous = item.ID
		result.List = append(result.List, projectGraphEdgeType(item, filter == "", selected))
	}
	return result, nil
}

func projectGraphEdgeType(item graphEdgeTypeWire, all bool, selected map[string]bool) GraphEdgeType {
	var out GraphEdgeType
	if all || selected["id"] {
		out.ID = &item.ID
	}
	if all || selected["name"] {
		out.Name = &item.Name
	}
	if all || selected["accId"] {
		out.AccID = &item.AccID
	}
	if all || selected["userId"] {
		out.UserID = &GraphTypeCreator{Value: item.UserID}
	}
	if all || selected["isSystem"] {
		out.IsSystem = item.IsSystem
	}
	if all || selected["createdAt"] {
		out.CreatedAt = item.CreatedAt
	}
	if all || selected["isTree"] {
		out.IsTree = item.IsTree
	}
	return out
}
