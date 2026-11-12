package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	// maxPathSources bounds the files one request reads a source of (one app_file/get each).
	maxPathSources = 20
	// maxReadBytes bounds the sources, outlines and nodes of one smart_form_files_get answer: the
	// Console agent's host cuts a tool answer over 50,000 characters, and what is cut cannot be read.
	maxReadBytes = 40_000
	// minReadBytes is what sources may take however long the file listing is.
	minReadBytes = 8_000
	// maxOutlineLevels is the deepest outline; a tree is rarely deeper.
	maxOutlineLevels = 20
)

// developTree lists the develop folders and files as slash paths, without sources.
func (c *Client) developTree(ctx context.Context, appID string, dev int64) ([]SmartFormEntry, error) {
	r, e := call[structRequest, structWire](ctx, c, "application/struct", structRequest{AccID: c.workspace(), ActorID: appID, EnvID: dev,
		Fields: []string{"id", "title", "objType", "type", "sourceHash", "children"}})
	if e != nil {
		return nil, e
	}
	if r.Tree.ID < 1 || r.Tree.ObjType != "folder" {
		return nil, errors.New("invalid Smart Form tree")
	}
	files := []SmartFormEntry{}
	var walk func(n ContentNode, path string)
	walk = func(n ContentNode, path string) {
		files = append(files, SmartFormEntry{ID: n.ID, Path: path, ObjType: n.ObjType, Type: n.Type, SourceHash: n.SourceHash})
		for _, child := range n.Children {
			walk(child, strings.TrimPrefix(path+"/"+child.Title, "//"))
		}
	}
	walk(r.Tree, "/")
	return files, nil
}

// GetSmartFormFiles lists the develop tree as slash paths. Sources come on request and within
// maxReadBytes: what does not fit is left out and named in Note, a page is read as an outline
// (Outline) and then node by node (NodeID), any other long file from an Offset on.
func (c *Client) GetSmartFormFiles(ctx context.Context, in SmartFormFilesInput) (SmartFormFiles, error) {
	if e := consoleOnly(c); e != nil {
		return SmartFormFiles{}, e
	}
	if in.Outline < 0 || in.Outline > maxOutlineLevels || in.Offset < 0 || len(in.NodeID) > 2000 {
		return SmartFormFiles{}, fmt.Errorf("outline 1..%d and offset >= 0 required", maxOutlineLevels)
	}
	if (in.NodeID != "" || in.Offset > 0) && len(in.Paths) != 1 {
		return SmartFormFiles{}, errors.New("nodeId and offset read one file: name it in paths")
	}
	dev, prod, e := c.envs(ctx, in.AppID)
	if e != nil {
		return SmartFormFiles{}, e
	}
	files, e := c.developTree(ctx, in.AppID, dev)
	if e != nil {
		return SmartFormFiles{}, e
	}
	out := SmartFormFiles{AppID: in.AppID, DevelopEnvID: dev, ProductionEnvID: prod, Files: files}
	if !in.WithSource && len(in.Paths) == 0 && in.Outline == 0 {
		return out, nil
	}
	named := map[string]bool{}
	if len(in.Paths) > 0 {
		out.Files, out.Missing, named = narrowFiles(files, in.Paths)
	}
	if in.NodeID != "" || in.Offset > 0 {
		if n := len(out.Files); n != 1 || out.Files[0].ObjType != "file" {
			return SmartFormFiles{}, errors.New("nodeId and offset read one file: paths names a folder or nothing")
		}
	}
	// The listing rides in the same answer: its bytes come off the budget (a floor keeps a form with
	// very many files readable by paths).
	listing, _ := json.Marshal(out.Files)
	budget := max(maxReadBytes-len(listing), minReadBytes)
	r := &fileReader{in: in, left: budget, budget: budget}
	for i := range out.Files {
		f := &out.Files[i]
		if f.ObjType != "file" || in.Outline > 0 && in.NodeID == "" && !named[f.Path] && !isPageConfig(f.Path) && !strings.HasPrefix(f.Path, "definitions/") {
			continue
		}
		if r.read == maxPathSources || r.left <= 0 {
			r.skipped++
			continue
		}
		file, e := c.appFile(ctx, in.AppID, f.ID)
		if e != nil {
			return SmartFormFiles{}, e
		}
		r.read++
		f.SourceHash = file.SourceHash
		if e = r.fill(f, file.Source); e != nil {
			return SmartFormFiles{}, e
		}
	}
	out.Note = r.note()
	return out, nil
}

// narrowFiles keeps the entries paths name (a folder with everything under it); named are the
// kept paths a request names itself, missing the requested paths that name nothing.
func narrowFiles(all []SmartFormEntry, paths []string) (kept []SmartFormEntry, missing []string, named map[string]bool) {
	kept, named = []SmartFormEntry{}, map[string]bool{}
	hit := map[string]bool{}
	for _, f := range all {
		keep := false
		for _, p := range paths {
			if p = strings.Trim(p, "/"); p != "" && (f.Path == p || strings.HasPrefix(f.Path, p+"/")) {
				hit[p] = true // every path that names it counts as found; the entry is kept once
				keep = true
				named[f.Path] = named[f.Path] || f.Path == p
			}
		}
		if keep {
			kept = append(kept, f)
		}
	}
	for _, p := range paths {
		if !hit[strings.Trim(p, "/")] {
			missing = append(missing, p)
		}
	}
	return kept, missing, named
}

// fileReader fills the entries of one answer within its byte budget.
type fileReader struct {
	in      SmartFormFilesInput
	left    int // bytes the answer may still take
	budget  int // left at the start: the whole answer's share for sources, outlines and nodes
	read    int // files read
	skipped int // files left without their source
	cut     bool
}

func (r *fileReader) fill(f *SmartFormEntry, source string) error {
	if r.in.NodeID == "" && r.in.Outline == 0 {
		return r.source(f, source)
	}
	doc, e := decodeDoc([]byte(source))
	if e != nil {
		if r.in.NodeID != "" {
			return fmt.Errorf("%s is not JSON: read it from an offset", f.Path)
		}
		return r.source(f, source)
	}
	var path []string
	if id := r.in.NodeID; id != "" {
		if id[0] != '/' {
			if id, e = findNode(doc, id); e != nil {
				return e
			}
		}
		if path, e = parsePointer(id); e == nil {
			doc, e = getAt(doc, path)
		}
		if e != nil {
			return e
		}
	}
	if r.in.Outline > 0 {
		f.Outline = outlineDoc(doc, path, r.in.Outline, r.left)
		r.left -= encodedLen(f.Outline)
		return nil
	}
	b, e := encodeDoc(doc, "")
	if e != nil {
		return e
	}
	if len(b) > r.left {
		return fmt.Errorf("node %s is %d characters, one answer holds %d: read it with outline 2 and this nodeId, then its child nodes", r.in.NodeID, len(b), r.left)
	}
	f.Node, r.left = b, r.left-len(b)
	return nil
}

// source gives the entry its source from the offset on. One that does not fit beside the sources
// already answered is left out (to be read alone); one too long alone is cut, with Next to go on.
func (r *fileReader) source(f *SmartFormEntry, source string) error {
	at := r.in.Offset
	if at > 0 {
		if at >= len(source) {
			return fmt.Errorf("offset %d is past the end of %s (%d bytes)", at, f.Path, len(source))
		}
		for at < len(source) && !utf8.RuneStart(source[at]) {
			at++ // an offset inside a character starts at the next one
		}
		if at == len(source) {
			return fmt.Errorf("offset %d is past the end of %s (%d bytes)", r.in.Offset, f.Path, len(source))
		}
		source = source[at:]
	}
	if encodedLen(source) > r.left {
		if r.left < r.budget {
			r.skipped++
			return nil
		}
		source = cutSource(source, r.left)
		f.Next, r.cut = at+len(source), true
	}
	f.Source, r.left = &source, r.left-encodedLen(source)
	return nil
}

// encodedLen is the length of s as a JSON string value, which is how an answer carries it.
func encodedLen(s string) int {
	b, _ := json.Marshal(s) // a string always encodes
	return len(b) - 2
}

// cutSource is the longest head of s, cut between characters, that encodes within limit bytes.
func cutSource(s string, limit int) string {
	lo, hi := 0, min(len(s), limit) // s[:lo] fits; nothing longer than hi can
	for lo < hi {
		n := (lo + hi + 1) / 2
		if encodedLen(s[:n]) <= limit {
			lo = n
		} else {
			hi = n - 1
		}
	}
	for lo > 0 && lo < len(s) && !utf8.RuneStart(s[lo]) {
		lo--
	}
	return s[:lo]
}

func (r *fileReader) note() string {
	var notes []string
	if r.skipped > 0 {
		notes = append(notes, fmt.Sprintf("%d files left without source (one answer holds %d characters and %d files): read them by paths, fewer at a time", r.skipped, r.budget, maxPathSources))
	}
	if r.cut {
		notes = append(notes, "source cut: read on with offset = next; read a page or definition with outline, then one node with nodeId, and change it with smart_form_page_patch")
	}
	return strings.Join(notes, "; ")
}

// maxStaleIDs bounds the node ids a refused overwrite names.
const maxStaleIDs = 12

// staleWrite turns the server's refusal of a write based on an older revision into a short error:
// the file, its hash now, and what the overwrite would have lost. The server's answer carries the
// whole current source of each file, which an error must not.
func staleWrite(e error, items []SmartFormFileUpdate) error {
	var api *APIError
	if !errors.As(e, &api) || len(api.stale) == 0 {
		return e
	}
	var parts []string
	for _, now := range api.stale {
		part := fmt.Sprintf("%s (id %d) changed since you read it: its sourceHash is now %s", now.Title, now.ID, now.SourceHash)
		for _, it := range items {
			if it.ID == now.ID && it.Source != nil {
				part += lostBy(now.Source, *it.Source)
			}
		}
		parts = append(parts, part)
	}
	return errors.New("not written: " + strings.Join(parts, "; ") + ". Read what changed (smart_form_files_get with outline or nodeId) and patch it with smart_form_page_patch, or send this hash to overwrite")
}

// lostBy says what the stored source has that the one sent lacks: node ids of a page or a
// definition, else only the sizes.
func lostBy(stored, sent string) string {
	has, page := sourceIDs(stored)
	sends, same := sourceIDs(sent)
	if !page || !same {
		return fmt.Sprintf(" (%d bytes now, %d sent)", len(stored), len(sent))
	}
	var lost []string
	for _, id := range has {
		if !slices.Contains(sends, id) {
			lost = append(lost, id)
		}
	}
	if len(lost) == 0 {
		return " (it has no node yours lacks; props may differ)"
	}
	more := ""
	if len(lost) > maxStaleIDs {
		more = fmt.Sprintf(" and %d more", len(lost)-maxStaleIDs)
		lost = lost[:maxStaleIDs]
	}
	return " (it now has nodes yours lacks: " + strings.Join(lost, ", ") + more + ")"
}

// sourceIDs is the node ids of a page config or definition source; ok is false for anything else.
func sourceIDs(source string) ([]string, bool) {
	doc, e := decodeDoc([]byte(source))
	if e != nil {
		return nil, false
	}
	f, ok := walkNodes(doc, "")
	return f.ids, ok
}
