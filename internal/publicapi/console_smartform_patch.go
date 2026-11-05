package publicapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// defaultPatchPath is the file smart_form_page_patch changes when neither path nor fileId is given.
const defaultPatchPath = "pages/index/config"

// SmartFormPatchInput changes one develop JSON file by RFC 6902 operations instead of rewriting it.
type SmartFormPatchInput struct {
	AppID string `json:"appId"`
	// Path (default pages/index/config) or FileID names the file.
	Path   string `json:"path,omitempty"`
	FileID int64  `json:"fileId,omitempty"`
	// NodeID makes every op path (and from) relative to the CDU tree node with that id.
	NodeID      string    `json:"nodeId,omitempty"`
	IfMatchHash string    `json:"ifMatchHash,omitempty"`
	Ops         []PatchOp `json:"ops"`
}

// SmartFormPatched is what a patch wrote: never the source, only where to find it and its new hash.
type SmartFormPatched struct {
	AppID      string `json:"appId"`
	FileID     int64  `json:"fileId"`
	Path       string `json:"path,omitempty"`
	SourceHash string `json:"sourceHash"`
	// Applied are the ops as run, with the absolute pointers a nodeId resolved to.
	Applied   []string `json:"applied"`
	Unchanged bool     `json:"unchanged,omitempty"`
}

type appFileGetRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
	FileID  int64  `json:"fileId"`
}

// appFile reads one develop file with its source and current sourceHash.
func (c *Client) appFile(ctx context.Context, appID string, id int64) (File, error) {
	f, e := call[appFileGetRequest, File](ctx, c, "app_file/get", appFileGetRequest{AccID: c.config.Workspace, ActorID: appID, FileID: id})
	if e == nil && f.ID != id {
		e = errors.New("app_file/get returned another file")
	}
	return f, e
}

// PatchSmartFormFile reads one develop JSON file, applies the ops (all or nothing) and writes it
// back with ifMatchHash set to the hash it read, so a concurrent change is refused, not overwritten.
func (c *Client) PatchSmartFormFile(ctx context.Context, in SmartFormPatchInput) (SmartFormPatched, error) {
	if e := consoleOnly(c); e != nil {
		return SmartFormPatched{}, e
	}
	if !validID(in.AppID) || len(in.Ops) < 1 || len(in.Ops) > 50 {
		return SmartFormPatched{}, errors.New("appId and 1..50 ops required")
	}
	if in.FileID > 0 && in.Path != "" {
		return SmartFormPatched{}, errors.New("name the file by path or by fileId, not both")
	}
	in.Path = strings.Trim(in.Path, "/")
	if in.FileID < 1 {
		if in.Path == "" {
			in.Path = defaultPatchPath
		}
		id, e := c.filePathID(ctx, in.AppID, in.Path)
		if e != nil {
			return SmartFormPatched{}, e
		}
		in.FileID = id
	}
	f, e := c.appFile(ctx, in.AppID, in.FileID)
	if e != nil {
		return SmartFormPatched{}, e
	}
	if in.IfMatchHash != "" && in.IfMatchHash != f.SourceHash {
		return SmartFormPatched{}, fmt.Errorf("the file changed since you read it: its sourceHash is now %s; read it again before patching", f.SourceHash)
	}
	source, applied, changed, e := patchSource(f.Source, in.NodeID, in.Ops)
	if e != nil {
		return SmartFormPatched{}, e
	}
	out := SmartFormPatched{AppID: in.AppID, FileID: f.ID, Path: in.Path, SourceHash: f.SourceHash, Applied: applied}
	if !changed {
		out.Unchanged = true
		return out, nil
	}
	w, e := c.UpdateSmartFormFiles(ctx, SmartFormFilesUpdateInput{AppID: in.AppID,
		Items: []SmartFormFileUpdate{{ID: f.ID, Title: f.Title, Type: f.Type, Source: &source, IfMatchHash: f.SourceHash}}})
	if e != nil {
		return SmartFormPatched{}, e
	}
	out.SourceHash = w.Files[0].SourceHash
	return out, nil
}

// filePathID finds a develop file's id by its slash path.
func (c *Client) filePathID(ctx context.Context, appID, path string) (int64, error) {
	files, e := c.GetSmartFormFiles(ctx, SmartFormFilesInput{AppID: appID})
	if e != nil {
		return 0, e
	}
	var paths []string
	for _, f := range files.Files {
		if f.ObjType != "file" {
			continue
		}
		if f.Path == path {
			return f.ID, nil
		}
		paths = append(paths, f.Path)
	}
	return 0, fmt.Errorf("no develop file %s; files: %s", path, strings.Join(paths, ", "))
}

// patchSource applies ops to a JSON source and writes it back in the source's own indentation;
// with nodeID every op path is relative to that CDU tree node. changed is false when the document
// is the same JSON as before (only test ops, or a value replaced by an equal one).
func patchSource(source, nodeID string, ops []PatchOp) (string, []string, bool, error) {
	doc, e := decodeDoc([]byte(source))
	if e != nil {
		return "", nil, false, fmt.Errorf("the file is not JSON (%v): patch a page config, locale or viewModel, or rewrite it with smart_form_files_update", e)
	}
	before := cloneDoc(doc)
	if nodeID != "" {
		base, e := findNode(doc, nodeID)
		if e != nil {
			return "", nil, false, e
		}
		rel := ops
		ops = make([]PatchOp, len(rel))
		for i, op := range rel {
			if op.Path != "" && op.Path[0] != '/' {
				return "", nil, false, fmt.Errorf("op %d: path %q must start with / (or be empty for the node itself)", i, op.Path)
			}
			ops[i] = op
			ops[i].Path = base + op.Path
			if op.Op == "move" || op.Op == "copy" {
				ops[i].From = base + op.From
			}
		}
	}
	doc, e = applyPatch(doc, ops)
	if e != nil {
		return "", nil, false, e
	}
	b, e := encodeDoc(doc, docIndent(source))
	if e != nil {
		return "", nil, false, e
	}
	applied := make([]string, len(ops))
	for i, op := range ops {
		applied[i] = op.Op + " " + op.Path
		if op.Op == "move" || op.Op == "copy" {
			applied[i] = op.Op + " " + op.From + " -> " + op.Path
		}
	}
	return string(b), applied, !equalDoc(before, doc), nil
}

// findNode returns the JSON Pointer of the one node with this id in the page's tree, walking
// children lists and named-slot children.
func findNode(doc any, id string) (string, error) {
	page, ok := doc.(*object)
	tree, has := any(nil), false
	if ok {
		tree, has = page.get("tree")
	}
	if !has {
		return "", errors.New("nodeId needs a page config: the file has no tree")
	}
	var found, ids []string
	var walk func(n any, path []string)
	walk = func(n any, path []string) {
		node, ok := n.(*object)
		if !ok {
			return
		}
		if v, ok := node.get("id"); ok {
			if s, ok := v.(string); ok {
				ids = append(ids, s)
				if s == id {
					found = append(found, pointerOf(path))
				}
			}
		}
		children, _ := node.get("children")
		switch c := children.(type) {
		case []any:
			for i, child := range c {
				walk(child, append(append([]string{}, path...), "children", fmt.Sprint(i)))
			}
		case *object:
			for _, slot := range c.keys {
				list, _ := c.vals[slot].([]any)
				for i, child := range list {
					walk(child, append(append([]string{}, path...), "children", slot, fmt.Sprint(i)))
				}
			}
		}
	}
	walk(tree, []string{"tree"})
	switch {
	case len(found) == 1:
		return found[0], nil
	case len(found) > 1:
		return "", fmt.Errorf("nodeId %q names %d nodes (%s): patch by absolute path", id, len(found), strings.Join(found, ", "))
	}
	if len(ids) > 40 {
		ids = append(ids[:40], "…")
	}
	return "", fmt.Errorf("no node with id %q in the tree; node ids: %s", id, strings.Join(ids, ", "))
}
