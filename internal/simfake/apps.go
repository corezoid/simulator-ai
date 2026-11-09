package simfake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// MaxSourceBytes is the longest file source the fake stores, as the tools' schemas allow.
const MaxSourceBytes = 65536

// appNode is a folder or a file of a develop tree.
type appNode struct {
	ID, Parent          int64
	Title, Type, Source string
	Folder              bool
}

type release struct {
	ID, Number int64
	Status     string
}

// app is a Smart Form: its develop tree, what production serves and its releases.
type app struct {
	ID, Ref   string
	Dev, Prod int64
	Root      int64
	Nodes     []*appNode
	Served    map[string]string // production files by path, as of the last release
	Releases  []*release
}

func hashOf(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func (a *app) node(id int64) *appNode {
	for _, n := range a.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (a *app) path(n *appNode) string {
	if n.ID == a.Root {
		return ""
	}
	return strings.TrimPrefix(a.path(a.node(n.Parent))+"/"+n.Title, "/")
}

// files are the develop files by path.
func (a *app) files() map[string]*appNode {
	out := map[string]*appNode{}
	for _, n := range a.Nodes {
		if !n.Folder {
			out[a.path(n)] = n
		}
	}
	return out
}

func (s *Server) add(a *app, parent int64, title, kind, source string, folder bool) *appNode {
	s.Seq++
	n := &appNode{ID: s.Seq, Parent: parent, Title: title, Type: kind, Source: source, Folder: folder}
	a.Nodes = append(a.Nodes, n)
	return n
}

type appParams struct {
	ActorID string `json:"actorId"`
}

var appMethods = map[string]func(*Server, json.RawMessage) (any, *rpcError){
	"application/create":   (*Server).appCreate,
	"application/env/list": (*Server).appEnvs,
	"application/struct":   (*Server).appStruct,
	"app_folder/create":    (*Server).appFolders,
	"app_file/create":      (*Server).appFilesCreate,
	"app_file/get":         (*Server).appFileGet,
	"app_file/update":      (*Server).appFilesUpdate,
	"release/create":       (*Server).releaseCreate,
	"release/get":          (*Server).releaseGet,
	"route/key/get":        (*Server).routeKey,
	"page/render":          (*Server).pageRender,
}

func (s *Server) app(params json.RawMessage) (*app, *rpcError) {
	var p appParams
	_ = json.Unmarshal(params, &p)
	if a := s.apps[p.ActorID]; a != nil {
		return a, nil
	}
	return nil, fail("notFound", "Smart Form not found")
}

func envsOf(a *app) []any {
	return []any{map[string]any{"id": a.Dev, "actorId": a.ID, "title": "develop"}, map[string]any{"id": a.Prod, "actorId": a.ID, "title": "production", "readonly": true}}
}

// appCreate seeds what sim-api seeds: a pages/index page, its locale and viewModel, styles/index.
func (s *Server) appCreate(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Ref string `json:"ref"`
	}
	_ = json.Unmarshal(params, &p)
	for _, other := range s.apps {
		if other.Ref == p.Ref {
			return nil, fail("invalidParams", "duplicate: ref")
		}
	}
	a := &app{ID: s.id("app"), Ref: p.Ref}
	s.Seq += 2
	a.Dev, a.Prod = s.Seq-1, s.Seq
	a.Root = s.add(a, 0, "root", "", "", true).ID
	pages := s.add(a, a.Root, "pages", "", "", true)
	index := s.add(a, pages.ID, "index", "", "", true)
	s.add(a, index.ID, "config", "application/json", `{"schemaVersion":"2.0","id":"index","tree":{"id":"root","type":"Stack","children":[]}}`, false)
	s.add(a, index.ID, "locale", "application/json", `{}`, false)
	s.add(a, index.ID, "viewModel", "application/json", `{}`, false)
	styles := s.add(a, a.Root, "styles", "", "", true)
	s.add(a, styles.ID, "index", "text/less", "", false)
	s.apps[a.ID] = a
	return map[string]any{"application": map[string]any{"id": a.ID}, "envs": envsOf(a)}, nil
}

func (s *Server) appEnvs(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	return map[string]any{"canModify": true, "envs": envsOf(a)}, nil
}

func (s *Server) appStruct(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		Fields []string `json:"fields"`
	}
	_ = json.Unmarshal(params, &p)
	sources := false
	for _, f := range p.Fields {
		sources = sources || f == "source"
	}
	var build func(n *appNode) map[string]any
	build = func(n *appNode) map[string]any {
		if !n.Folder {
			out := map[string]any{"id": n.ID, "objType": "file", "title": n.Title, "type": n.Type, "sourceHash": hashOf(n.Source)}
			if sources {
				out["source"] = n.Source
			}
			return out
		}
		children := []any{}
		for _, child := range a.Nodes {
			if child.Parent == n.ID && child.ID != n.ID {
				children = append(children, build(child))
			}
		}
		return map[string]any{"id": n.ID, "objType": "folder", "title": n.Title, "children": children}
	}
	return map[string]any{"tree": build(a.node(a.Root))}, nil
}

func (s *Server) appFolders(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		Items []struct {
			FolderID int64  `json:"folderId"`
			Title    string `json:"title"`
		} `json:"items"`
	}
	_ = json.Unmarshal(params, &p)
	folders := []any{}
	for _, it := range p.Items {
		if parent := a.node(it.FolderID); parent == nil || !parent.Folder {
			return nil, fail("notFound", "folder not found")
		}
	}
	for _, it := range p.Items {
		n := s.add(a, it.FolderID, it.Title, "", "", true)
		folders = append(folders, map[string]any{"id": n.ID, "folderId": n.Parent, "objType": "folder", "title": n.Title})
	}
	return map[string]any{"folders": folders}, nil
}

func fileJSON(n *appNode) map[string]any {
	return map[string]any{"id": n.ID, "folderId": n.Parent, "objType": "file", "title": n.Title, "type": n.Type, "source": n.Source, "sourceHash": hashOf(n.Source)}
}

// checkSource refuses what sim-api refuses at save in the terms the fake knows: a source over the
// limit, and a page config that is not a JSON object with a tree.
func (a *app) checkSource(parent int64, title, source string) *rpcError {
	if len(source) > MaxSourceBytes {
		return fail("invalidParams", "source is too long")
	}
	folder := a.node(parent)
	if title != "config" || folder == nil || !strings.HasPrefix(a.path(folder), "pages/") {
		return nil
	}
	var page struct {
		Tree map[string]any `json:"tree"`
	}
	if json.Unmarshal([]byte(source), &page) != nil || page.Tree == nil {
		return fail("invalidParams", fmt.Sprintf("invalid CDU source in page '%s'", folder.Title))
	}
	return nil
}

func (s *Server) appFilesCreate(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		Items []struct {
			FolderID            int64 `json:"folderId"`
			Title, Type, Source string
		} `json:"items"`
	}
	_ = json.Unmarshal(params, &p)
	for _, it := range p.Items {
		if parent := a.node(it.FolderID); parent == nil || !parent.Folder {
			return nil, fail("notFound", "folder not found")
		}
		if err = a.checkSource(it.FolderID, it.Title, it.Source); err != nil {
			return nil, err
		}
	}
	files := []any{}
	for _, it := range p.Items {
		files = append(files, fileJSON(s.add(a, it.FolderID, it.Title, it.Type, it.Source, false)))
	}
	return map[string]any{"files": files}, nil
}

func (s *Server) appFileGet(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		FileID int64 `json:"fileId"`
	}
	_ = json.Unmarshal(params, &p)
	n := a.node(p.FileID)
	if n == nil || n.Folder {
		return nil, fail("notFound", "file not found")
	}
	return fileJSON(n), nil
}

// appFilesUpdate applies all items or none; an ifMatchHash that is not the file's hash refuses the
// batch with the current state of every mismatching file, as sim-api does.
func (s *Server) appFilesUpdate(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		Items []struct {
			ID          int64   `json:"id"`
			Title       string  `json:"title"`
			Type        string  `json:"type"`
			Source      *string `json:"source"`
			IfMatchHash string  `json:"ifMatchHash"`
		} `json:"items"`
	}
	_ = json.Unmarshal(params, &p)
	var stale []any
	for _, it := range p.Items {
		n := a.node(it.ID)
		if n == nil || n.Folder {
			return nil, fail("notFound", "file not found")
		}
		if it.IfMatchHash != "" && it.IfMatchHash != hashOf(n.Source) {
			stale = append(stale, map[string]any{"id": n.ID, "title": n.Title, "source": n.Source, "sourceHash": hashOf(n.Source)})
		}
		if it.Source != nil {
			if err = a.checkSource(n.Parent, it.Title, *it.Source); err != nil {
				return nil, err
			}
		}
	}
	if len(stale) > 0 {
		e := &rpcError{Code: 1, Message: "the file changed"}
		e.Data.Reason, e.Data.ReasonParams = "appFileSourceMismatch", map[string]any{"files": stale}
		return nil, e
	}
	files := []any{}
	for _, it := range p.Items {
		n := a.node(it.ID)
		n.Title, n.Type = it.Title, it.Type
		if it.Source != nil {
			n.Source = *it.Source
		}
		files = append(files, fileJSON(n))
	}
	return map[string]any{"files": files}, nil
}

func (s *Server) releaseCreate(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	a.Served = map[string]string{}
	for path, n := range a.files() {
		a.Served[path] = n.Source
	}
	for _, r := range a.Releases {
		r.Status = "archived"
	}
	s.Seq++
	r := &release{ID: s.Seq, Number: int64(len(a.Releases) + 1), Status: "active"}
	a.Releases = append(a.Releases, r)
	return releaseJSON(a, r), nil
}

func releaseJSON(a *app, r *release) map[string]any {
	return map[string]any{"id": r.ID, "actorId": a.ID, "envId": a.Prod, "sourceEnvId": a.Dev, "releaseNumber": r.Number, "status": r.Status}
}

func (s *Server) releaseGet(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		ReleaseID int64 `json:"releaseId"`
	}
	_ = json.Unmarshal(params, &p)
	for _, r := range a.Releases {
		if r.ID == p.ReleaseID {
			return releaseJSON(a, r), nil
		}
	}
	return nil, fail("notFound", "release not found")
}

func (s *Server) routeKey(params json.RawMessage) (any, *rpcError) {
	a, err := s.app(params)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"appKey": a.Ref}
	if n := len(a.Releases); n > 0 {
		out["activeReleaseId"] = a.Releases[n-1].ID
	}
	return out, nil
}

// pageRender answers a page of develop or of what production serves: its config with every $ref
// replaced by its definition, its viewModel and locale.
func (s *Server) pageRender(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Ref, EnvTitle, Page string
	}
	_ = json.Unmarshal(params, &p)
	var a *app
	for _, other := range s.apps {
		if other.Ref == p.Ref {
			a = other
		}
	}
	if a == nil {
		return nil, fail("notFound", "Smart Form not found")
	}
	files := a.Served
	if p.EnvTitle != "production" {
		files = map[string]string{}
		for path, n := range a.files() {
			files[path] = n.Source
		}
	}
	if p.Page == "" {
		p.Page = "index"
	}
	var page struct {
		SchemaVersion string `json:"schemaVersion"`
		Tree          any    `json:"tree"`
		Bindings      any    `json:"bindings"`
	}
	source, ok := files["pages/"+p.Page+"/config"]
	if !ok || json.Unmarshal([]byte(source), &page) != nil {
		return nil, fail("notFound", "page not found: "+p.Page)
	}
	var bad *rpcError
	var inline func(v any) any
	inline = func(v any) any {
		switch t := v.(type) {
		case []any:
			for i, item := range t {
				t[i] = inline(item)
			}
		case map[string]any:
			if ref, ok := t["$ref"].(string); ok {
				var def map[string]any
				if json.Unmarshal([]byte(files[strings.TrimPrefix(ref, "#/")]), &def) != nil || def == nil {
					bad = fail("invalidParams", "missing definition "+ref)
					return v
				}
				for k, own := range t {
					if k != "$ref" {
						def[k] = own
					}
				}
				return inline(def)
			}
			if children, ok := t["children"]; ok {
				t["children"] = inline(children)
			}
		}
		return v
	}
	tree := inline(page.Tree)
	if bad != nil {
		return nil, bad
	}
	out := map[string]any{"schemaVersion": page.SchemaVersion, "pageId": p.Page, "tree": tree, "bindings": page.Bindings}
	for _, name := range []string{"viewModel", "locale"} {
		out[name] = json.RawMessage("{}")
		if src := files["pages/"+p.Page+"/"+name]; json.Valid([]byte(src)) {
			out[name] = json.RawMessage(src)
		}
	}
	return out, nil
}

// EditFile changes a develop file behind the tools' back, as a person saving in the Smart Form
// Editor does; it reports whether the file exists.
func (s *Server) EditFile(appID, path string, edit func(source string) string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.apps[appID]
	if a == nil {
		return false
	}
	n := a.files()[path]
	if n == nil {
		return false
	}
	n.Source = edit(n.Source)
	return true
}

// AppFiles are the sources by path of a Smart Form's develop tree, or of what production serves.
func (s *Server) AppFiles(appID string, production bool) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	a := s.apps[appID]
	if a == nil {
		return out
	}
	if production {
		for path, source := range a.Served {
			out[path] = source
		}
		return out
	}
	for path, n := range a.files() {
		out[path] = n.Source
	}
	return out
}
