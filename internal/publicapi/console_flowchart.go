package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

// FlowchartBlocksInput narrows the native flowchart block catalog by title substring or exact blockIds.
type FlowchartBlocksInput struct {
	Query    string   `json:"query,omitempty"`
	BlockIDs []string `json:"blockIds,omitempty"`
}

type workspaceRequest struct {
	AccID string `json:"accId"`
}
type formRequest struct {
	AccID  string `json:"accId"`
	FormID int64  `json:"formId"`
}

// systemFormRow follows public form/system/list rows; the template body is not included.
type systemFormRow struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	ParentID *int64 `json:"parentId"`
	Type     string `json:"type"`
}
type systemFormsWire struct {
	List []systemFormRow `json:"list"`
}

// formBodyWire follows public form/get: the child block form with its item values in form.
type formBodyWire struct {
	ID        int64           `json:"id"`
	ParentID  *int64          `json:"parentId"`
	Title     string          `json:"title"`
	UpdatedAt int64           `json:"updatedAt"`
	Form      json.RawMessage `json:"form"`
}

// blockItems are the catalog items of a FlowchartBlock child form (UI server/forms/flowchartBlocks.ts).
type blockItems struct {
	Sections []struct {
		Content []struct {
			ID    string          `json:"id"`
			Value json.RawMessage `json:"value"`
		} `json:"content"`
	} `json:"sections"`
}

// ListFlowchartBlocks reads the workspace's native flowchart block definitions: the children of the
// FlowchartBlock system template. Until public form/flowchart-block/search exists this takes
// form/system/list plus one form/get per matching child; all reads, no writes.
func (c *Client) ListFlowchartBlocks(ctx context.Context, in FlowchartBlocksInput) ([]GraphBlock, error) {
	if e := consoleOnly(c); e != nil {
		return nil, e
	}
	if len(in.BlockIDs) > 100 || utf8.RuneCountInString(in.Query) > 255 {
		return nil, errors.New("at most 100 blockIds and a query up to 255 characters")
	}
	forms, e := call[workspaceRequest, systemFormsWire](ctx, c, "form/system/list", workspaceRequest{AccID: c.config.Workspace})
	if e != nil {
		return nil, e
	}
	var root int64
	for _, f := range forms.List {
		if f.Title == "FlowchartBlock" && f.ParentID == nil {
			root = f.ID
		}
	}
	if root < 1 {
		return nil, errors.New("FlowchartBlock system template not found")
	}
	wanted := map[string]bool{}
	for _, id := range in.BlockIDs {
		wanted[id] = true
	}
	// Lowest form id first: like sim-api, the first definition of a duplicated blockId wins.
	sort.Slice(forms.List, func(i, j int) bool { return forms.List[i].ID < forms.List[j].ID })
	query := strings.ToLower(in.Query)
	blocks := []GraphBlock{}
	seen := map[string]bool{}
	for _, f := range forms.List {
		if f.ParentID == nil || *f.ParentID != root || !strings.Contains(strings.ToLower(f.Title), query) {
			continue
		}
		body, e := call[formRequest, formBodyWire](ctx, c, "form/get", formRequest{AccID: c.config.Workspace, FormID: f.ID})
		if e != nil {
			return nil, e
		}
		b, ok := graphBlock(body, root)
		if !ok || seen[b.BlockID] || (len(wanted) > 0 && !wanted[b.BlockID]) {
			continue
		}
		seen[b.BlockID] = true
		blocks = append(blocks, b)
		if len(wanted) > 0 && len(blocks) == len(wanted) {
			break
		}
	}
	// Like sim-api: lowercase title, then id.
	sort.SliceStable(blocks, func(i, j int) bool {
		a, b := strings.ToLower(blocks[i].Title), strings.ToLower(blocks[j].Title)
		return a < b || (a == b && blocks[i].ID < blocks[j].ID)
	})
	return blocks, nil
}

func graphBlock(f formBodyWire, root int64) (GraphBlock, bool) {
	var items blockItems
	if f.ParentID == nil || *f.ParentID != root || !jsonObject(f.Form) || json.Unmarshal(f.Form, &items) != nil {
		return GraphBlock{}, false
	}
	b := GraphBlock{ID: f.ID, ParentID: root, Title: f.Title, UpdatedAt: f.UpdatedAt, Form: f.Form}
	for _, s := range items.Sections {
		for _, item := range s.Content {
			switch item.ID {
			case "blockId":
				_ = json.Unmarshal(item.Value, &b.BlockID)
			case "collection":
				_ = json.Unmarshal(item.Value, &b.Collection)
			}
		}
	}
	return b, strings.TrimSpace(b.BlockID) != ""
}
