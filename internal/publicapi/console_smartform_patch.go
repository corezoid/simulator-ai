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
	// Pending is what the page still lacks to send what is typed (see SmartFormWritten).
	Pending []string `json:"pending,omitempty"`
}

type appFileGetRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
	FileID  int64  `json:"fileId"`
}

// appFile reads one develop file with its source and current sourceHash.
func (c *Client) appFile(ctx context.Context, appID string, id int64) (File, error) {
	f, e := call[appFileGetRequest, File](ctx, c, "app_file/get", appFileGetRequest{AccID: c.workspace(), ActorID: appID, FileID: id})
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
	out.SourceHash, out.Pending = w.Files[0].SourceHash, w.Pending
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
		return "", nil, false, fmt.Errorf("the file is not JSON (%w): patch a page config, locale or viewModel, or rewrite it with smart_form_files_update", e)
	}
	before := cloneDoc(doc)
	root := &doc
	applied := make([]string, len(ops))
	for i, op := range ops {
		if nodeID != "" {
			if op, e = nodeOp(*root, nodeID, op, i); e != nil {
				return "", nil, false, e
			}
		}
		if e = applyOp(root, op); e != nil {
			return "", nil, false, fmt.Errorf("op %d (%s %s): %w", i, op.Op, op.Path, e)
		}
		applied[i] = op.Op + " " + op.Path
		if op.Op == "move" || op.Op == "copy" {
			applied[i] = op.Op + " " + op.From + " -> " + op.Path
		}
	}
	b, e := encodeDoc(*root, docIndent(source))
	if e != nil {
		return "", nil, false, e
	}
	return string(b), applied, !equalDoc(before, *root), nil
}

// nodeOp makes an op relative to the node nodeID absolute. The node is found anew for every op,
// since an earlier one may have moved, replaced or removed it: then the op is refused rather than
// sent to whatever took the node's place. "" is the node itself, so add and replace "" replace it
// (an add at an array index would insert a sibling with the same id) and remove "" deletes it.
func nodeOp(doc any, nodeID string, op PatchOp, i int) (PatchOp, error) {
	base, e := findNode(doc, nodeID)
	if e != nil {
		if i > 0 {
			return op, fmt.Errorf("op %d: after the ops before it, %w", i, e)
		}
		return op, e
	}
	if e = instanceOp(doc, base, op, i); e != nil {
		return op, e
	}
	if op.Op == "remove" && op.Path == "" && overlayRoot(doc, base) {
		return op, fmt.Errorf("op %d: node %q is what an overlay.open shows: removing it leaves an empty overlay. Remove the overlay.open action instead, or replace the node (path \"\")", i, nodeID)
	}
	moves := op.Op == "move" || op.Op == "copy"
	for _, p := range []string{op.Path, op.From} {
		if p != "" && p[0] != '/' {
			return op, fmt.Errorf("op %d: pointer %q must start with / (or be empty for the node itself)", i, p)
		}
	}
	abs := op
	abs.Path = base + op.Path
	if moves {
		abs.From = base + op.From
	}
	if op.Path == "" {
		switch {
		case op.Op == "add":
			abs.Op = "replace"
		case moves:
			return op, fmt.Errorf("op %d: %s onto the node itself is not supported: replace it with the value", i, op.Op)
		}
	}
	return abs, nil
}

// overlayRoot says the node at base is the whole content of an overlay.open: the content itself,
// or the only node of a content list.
func overlayRoot(doc any, base string) bool {
	tokens, err := parsePointer(base)
	if err != nil || len(tokens) == 0 {
		return false
	}
	if tokens[len(tokens)-1] == "content" {
		return true
	}
	if len(tokens) < 2 || tokens[len(tokens)-2] != "content" {
		return false
	}
	list, _ := getAt(doc, tokens[:len(tokens)-1])
	items, ok := list.([]any)
	return ok && len(items) == 1
}
