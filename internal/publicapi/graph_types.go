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

// GraphEdgeTypesInput follows edge_type/list with a stricter profile page bound of 100.
type GraphEdgeTypesInput struct {
	Limit  *int64 `json:"limit,omitempty"`
	Offset int64  `json:"offset,omitempty"`
}

// GraphEdgeType projects edge_type/list. Presence pointers distinguish missing flags from false.
type GraphEdgeType struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsSystem *bool  `json:"isSystem"`
	IsTree   *bool  `json:"isTree"`
}
type GraphEdgeTypes struct {
	List []GraphEdgeType `json:"list"`
}
