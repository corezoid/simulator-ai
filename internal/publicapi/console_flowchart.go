package publicapi

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// FlowchartBlocksInput narrows the native flowchart block catalog by title/blockId substring or
// exact blockIds.
type FlowchartBlocksInput struct {
	Query    string   `json:"query,omitempty"`
	BlockIDs []string `json:"blockIds,omitempty"`
	Offset   int64    `json:"offset,omitempty"`
}

type flowchartSearchRequest struct {
	AccID    string   `json:"accId"`
	Query    string   `json:"query,omitempty"`
	BlockIDs []string `json:"blockIds,omitempty"`
	Limit    int64    `json:"limit"`
	Offset   int64    `json:"offset,omitempty"`
}

// ListFlowchartBlocks reads the workspace's native flowchart block definitions through public
// form/flowchart-block/search: sorted by title, duplicate kinds resolved to the lowest form id.
func (c *Client) ListFlowchartBlocks(ctx context.Context, in FlowchartBlocksInput) (GraphCatalog, error) {
	if e := consoleOnly(c); e != nil {
		return GraphCatalog{}, e
	}
	if len(in.BlockIDs) > 100 || utf8.RuneCountInString(in.Query) > 200 || in.Offset < 0 {
		return GraphCatalog{}, errors.New("at most 100 blockIds and a query up to 200 characters")
	}
	wire, e := call[flowchartSearchRequest, graphCatalogWire](ctx, c, "form/flowchart-block/search", flowchartSearchRequest{AccID: c.config.Workspace,
		Query: strings.TrimSpace(in.Query), BlockIDs: in.BlockIDs, Limit: 100, Offset: in.Offset})
	if e != nil {
		return GraphCatalog{}, e
	}
	if wire.List == nil || wire.HasMore == nil || len(wire.List) > 100 {
		return GraphCatalog{}, errors.New("invalid native block catalog page")
	}
	return GraphCatalog{List: wire.List, HasMore: *wire.HasMore}, nil
}
