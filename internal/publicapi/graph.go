package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/corezoid/simulator-ai/contracts"
)

func (c *Client) GraphReadsEnabled() bool {
	return c.config.Profile == "graph-read" || c.GraphCreatesEnabled()
}

type graphRequest[P any] struct {
	AccID string `json:"accId"`
	Input P      `json:"-"`
}

// MarshalJSON injects only the host workspace into a named, already validated DTO.
func (r graphRequest[P]) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(r.Input)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	fields["accId"], err = json.Marshal(r.AccID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func graphCall[P, R any](ctx context.Context, c *Client, method string, input P) (R, error) {
	var result R
	if !c.GraphReadsEnabled() {
		return result, errors.New("graph-read profile is disabled")
	}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > 32<<10 {
		return result, errors.New("invalid or oversized graph metadata input")
	}
	if err = contracts.ValidateGraphInput(method, raw); err != nil {
		return result, err
	}
	return call[graphRequest[P], R](ctx, c, method, graphRequest[P]{AccID: c.config.Workspace, Input: input})
}

func (c *Client) SearchGraphBlocks(ctx context.Context, p GraphCatalogInput) (GraphCatalog, error) {
	if p.BlockIDs != nil && len(p.BlockIDs) == 0 {
		return GraphCatalog{}, errors.New("blockIds must be omitted or nonempty")
	}
	limit := int64(48)
	if p.Limit != nil {
		limit = *p.Limit
	}
	wire, err := graphCall[GraphCatalogInput, graphCatalogWire](ctx, c, "form/flowchart-block/search", p)
	if err != nil {
		return GraphCatalog{}, err
	}
	maxItems := limit
	wanted := map[string]bool{}
	if len(p.BlockIDs) > 0 {
		for _, id := range p.BlockIDs {
			wanted[id] = true
		}
		maxItems = int64(len(wanted))
	}
	if wire.List == nil || wire.HasMore == nil || int64(len(wire.List)) > maxItems || (len(wanted) > 0 && *wire.HasMore) || (len(wire.List) == 0 && *wire.HasMore) {
		return GraphCatalog{}, errors.New("invalid native block catalog page")
	}
	seen := map[string]bool{}
	for _, item := range wire.List {
		if item.ID < 1 || item.ParentID < 1 || strings.TrimSpace(item.BlockID) == "" || !jsonObject(item.Form) || seen[item.BlockID] || (len(wanted) > 0 && !wanted[item.BlockID]) {
			return GraphCatalog{}, errors.New("invalid native block definition identity")
		}
		seen[item.BlockID] = true
	}
	return GraphCatalog{List: wire.List, HasMore: *wire.HasMore}, nil
}

func (c *Client) GetGraphSystemForm(ctx context.Context, p GraphSystemFormInput) (GraphSystemForm, error) {
	r, err := graphCall[GraphSystemFormInput, GraphSystemForm](ctx, c, "form/system/get", p)
	if err == nil && (r.ID < 1 || r.Title != p.Title || r.Type != "system" || !jsonObject(r.Form)) {
		return GraphSystemForm{}, errors.New("system form identity mismatch")
	}
	return r, err
}

func jsonObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 1 && raw[0] == '{' && json.Valid(raw)
}
