package publicapi

import (
	"context"
	"errors"
	"strings"
)

// MemberWorkspace is one workspace of the caller (public workspace/list).
type MemberWorkspace struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

// WorkspacePage is one page of the caller's workspaces and the one in use.
type WorkspacePage struct {
	List    []MemberWorkspace `json:"list"`
	Current string            `json:"current,omitempty"`
}

type workspaceListRequest struct {
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset,omitempty"`
}

// ListWorkspaces reads one page of the workspaces the credential belongs to.
func (c *Client) ListWorkspaces(ctx context.Context, query string, limit, offset int) (WorkspacePage, error) {
	if c.DelegatedReadEnabled() {
		return WorkspacePage{}, errors.New("method unavailable in delegated-read profile")
	}
	if limit < 1 || limit > 100 || offset < 0 || len(query) > 255 {
		return WorkspacePage{}, errors.New("limit 1..100, offset >=0 and query up to 255 bytes")
	}
	page, err := call[workspaceListRequest, WorkspacePage](ctx, c, "workspace/list", workspaceListRequest{Query: strings.TrimSpace(query), Limit: limit, Offset: offset})
	if err != nil {
		return WorkspacePage{}, err
	}
	if page.List == nil || len(page.List) > limit {
		return WorkspacePage{}, errors.New("invalid workspace page")
	}
	page.Current = c.workspace()
	return page, nil
}

// maxWorkspaceScan bounds the membership check of workspace_set (pages of 100).
const maxWorkspaceScan = 20

// SetWorkspace selects one of the signed-in person's workspaces, by id or exact name, and
// remembers it for this origin. A host-pinned workspace or a host credential cannot be changed.
func (c *Client) SetWorkspace(ctx context.Context, id, name string) (MemberWorkspace, error) {
	if c.CredentialMode() != "login" {
		return MemberWorkspace{}, errors.New("the workspace comes from the host configuration (SIMULATOR_AI_WORKSPACE) with this credential; change it there and restart")
	}
	if c.WorkspacePinned() {
		return MemberWorkspace{}, errors.New("SIMULATOR_AI_WORKSPACE pins the workspace; unset it to choose one with workspace_set")
	}
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if (id == "") == (name == "") || len(id) > 255 || len(name) > 255 {
		return MemberWorkspace{}, errors.New("pass exactly one of workspaceId or name")
	}
	var named []MemberWorkspace
	for i := 0; i < maxWorkspaceScan; i++ {
		page, err := c.ListWorkspaces(ctx, name, 100, i*100)
		if err != nil {
			return MemberWorkspace{}, err
		}
		for _, w := range page.List {
			if id != "" && w.ID == id {
				return w, c.config.Account.SetWorkspace(w.ID)
			}
			if name != "" && strings.EqualFold(w.Name, name) {
				named = append(named, w)
			}
		}
		if len(page.List) < 100 {
			break
		}
	}
	switch {
	case len(named) == 1:
		return named[0], c.config.Account.SetWorkspace(named[0].ID)
	case len(named) > 1:
		return MemberWorkspace{}, errors.New("several workspaces have this name: pass workspaceId from workspaces_list")
	}
	return MemberWorkspace{}, errors.New("not one of your workspaces: list them with workspaces_list")
}
