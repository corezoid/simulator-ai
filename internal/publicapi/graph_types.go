package publicapi

import "encoding/json"

// GraphCatalogInput follows public form/flowchart-block/search; workspace is host-injected.
type GraphCatalogInput struct {
	BlockIDs   []string `json:"blockIds,omitempty"`
	Query      string   `json:"query,omitempty"`
	Collection string   `json:"collection,omitempty"`
	QuickOnly  bool     `json:"quickOnly,omitempty"`
	Limit      *int64   `json:"limit,omitempty"`
	Offset     int64    `json:"offset,omitempty"`
}

// GraphBlock follows the named catalog row. Form is the backend's open template JSON,
// not an arbitrary response bag; its contents never become agent instructions.
type GraphBlock struct {
	ID         int64           `json:"id"`
	ParentID   int64           `json:"parentId"`
	Title      string          `json:"title"`
	BlockID    string          `json:"blockId"`
	Collection string          `json:"collection,omitempty"`
	QuickBlock bool            `json:"quickBlock,omitempty"`
	UpdatedAt  int64           `json:"updatedAt"`
	Form       json.RawMessage `json:"form"`
}
type GraphCatalog struct {
	List    []GraphBlock `json:"list"`
	HasMore bool         `json:"hasMore"`
}
type graphCatalogWire struct {
	List    []GraphBlock `json:"list"`
	HasMore *bool        `json:"hasMore"`
}

// GraphSystemFormInput restricts public form/system/get to graph construction templates.
type GraphSystemFormInput struct {
	Title string `json:"title"`
}

// GraphSystemForm projects form/system/get, excluding owner data and editor capabilities.
type GraphSystemForm struct {
	ID    int64           `json:"id"`
	Title string          `json:"title"`
	Type  string          `json:"type"`
	Form  json.RawMessage `json:"form"`
}

// GraphEdgeTypesInput follows edge_type/list with a profile page bound of 100.
// Filter is the legacy MCP-only CSV projection, applied after full-row validation.
type GraphEdgeTypesInput struct {
	Limit    *int64 `json:"limit,omitempty"`
	Offset   int64  `json:"offset,omitempty"`
	IsSystem *bool  `json:"isSystem,omitempty"`
	Filter   string `json:"filter,omitempty"`
}

// GraphEdgeType projects the seven named public fields with legacy filterData
// selection. Pointers preserve an unselected field separately from false/zero.
type GraphEdgeType struct {
	ID        *int64            `json:"id,omitempty"`
	Name      *string           `json:"name,omitempty"`
	AccID     *string           `json:"accId,omitempty"`
	UserID    *GraphTypeCreator `json:"userId,omitempty"`
	IsSystem  *bool             `json:"isSystem,omitempty"`
	CreatedAt *int64            `json:"createdAt,omitempty"`
	IsTree    *bool             `json:"isTree,omitempty"`
}

// GraphTypeCreator restores the legacy SQL user_id null when the public backend
// omits an unrecorded creator. A nil outer field means excluded by filter instead.
type GraphTypeCreator struct{ Value *int64 }

func (c GraphTypeCreator) MarshalJSON() ([]byte, error) { return json.Marshal(c.Value) }

type GraphEdgeTypes struct {
	List []GraphEdgeType `json:"list"`
}

// graphEdgeTypeWire follows rpc_edge.edgeTypeRes. This client
// fetches full typed rows to validate identity before local field projection.
type graphEdgeTypeWire struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AccID     string `json:"accId"`
	UserID    *int64 `json:"userId"`
	IsSystem  *bool  `json:"isSystem"`
	CreatedAt *int64 `json:"createdAt"`
	IsTree    *bool  `json:"isTree"`
}

type graphEdgeTypesWire struct {
	List []graphEdgeTypeWire `json:"list"`
}
