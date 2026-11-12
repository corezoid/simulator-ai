package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var smartFormRef = regexp.MustCompile(`^[0-9a-zA-Z_-]{5,255}$`)

// SmartFormCreateInput creates a CDU v2 Smart Form (application/create with cduVersion 2).
type SmartFormCreateInput struct {
	Ref         string `json:"ref"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	SharedWith  string `json:"sharedWith,omitempty"`
}

type smartFormCreateRequest struct {
	AccID       string `json:"accId"`
	Ref         string `json:"ref"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	SharedWith  string `json:"sharedWith,omitempty"`
	CduVersion  int64  `json:"cduVersion"`
}

// SmartForm is a created Smart Form: edit develop, then release it into production.
type SmartForm struct {
	AppID           string `json:"appId"`
	Ref             string `json:"ref"`
	DevelopEnvID    int64  `json:"developEnvId"`
	ProductionEnvID int64  `json:"productionEnvId"`
}

type SmartFormInput struct {
	AppID string `json:"appId"`
}

type SmartFormFilesInput struct {
	AppID      string `json:"appId"`
	WithSource bool   `json:"withSource,omitempty"`
	// Paths narrows the answer to these files and folders (a folder with what is under it), each
	// file with its source.
	Paths []string `json:"paths,omitempty"`
	// Outline answers pages and definitions as one line per node, this many levels deep, instead of
	// their sources.
	Outline int `json:"outline,omitempty"`
	// NodeID reads one node of the one file paths names (a node id, or a JSON Pointer starting with
	// /); Offset reads that file's source from a byte offset on.
	NodeID string `json:"nodeId,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// SmartFormEntry is one folder or file of the develop tree, addressed by its slash path
// ("pages/index/config"); the root folder is "/".
type SmartFormEntry struct {
	ID         int64   `json:"id"`
	Path       string  `json:"path"`
	ObjType    string  `json:"objType"`
	Type       string  `json:"type,omitempty"`
	SourceHash string  `json:"sourceHash,omitempty"`
	Source     *string `json:"source,omitempty"`
	// Next is the offset the rest of a cut source is read from.
	Next    int             `json:"next,omitempty"`
	Outline string          `json:"outline,omitempty"`
	Node    json.RawMessage `json:"node,omitempty"`
}

type SmartFormFiles struct {
	AppID string `json:"appId"`
	// The environments' ids, for the release, trash and history tools.
	DevelopEnvID    int64            `json:"developEnvId"`
	ProductionEnvID int64            `json:"productionEnvId"`
	Files           []SmartFormEntry `json:"files"`
	// Missing are requested paths that name nothing in the develop tree.
	Missing []string `json:"missing,omitempty"`
	// Note says what the answer left out to stay within maxReadBytes, and how to read it.
	Note string `json:"note,omitempty"`
}

// SmartFormFileCreate creates a file in a develop folder; source is stored as text.
type SmartFormFileCreate struct {
	FolderID int64  `json:"folderId"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Source   string `json:"source"`
}

type SmartFormFilesCreateInput struct {
	AppID string                `json:"appId"`
	Items []SmartFormFileCreate `json:"items"`
}

// SmartFormFileUpdate replaces a file's source; title and type are always written.
type SmartFormFileUpdate struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Type        string  `json:"type"`
	Source      *string `json:"source,omitempty"`
	IfMatchHash string  `json:"ifMatchHash,omitempty"`
}

type SmartFormFilesUpdateInput struct {
	AppID string                `json:"appId"`
	Items []SmartFormFileUpdate `json:"items"`
}

type appFilesRequest[T any] struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
	Items   []T    `json:"items"`
}

// SmartFormFile is a written file with the sourceHash for a later ifMatchHash.
type SmartFormFile struct {
	ID         int64  `json:"id"`
	FolderID   int64  `json:"folderId"`
	Title      string `json:"title"`
	SourceHash string `json:"sourceHash"`
}

type SmartFormWritten struct {
	AppID string          `json:"appId"`
	Files []SmartFormFile `json:"files"`
	// Pending is what a written page still lacks to send what is typed (a Form without its
	// submit yet): saved, but smart_form_release refuses the page until it is wired.
	Pending []string `json:"pending,omitempty"`
}

type SmartFormReleaseInput struct {
	AppID     string `json:"appId"`
	ReleaseID int64  `json:"releaseId,omitempty"`
}

// SmartFormRelease is the result contract the Console backend reads: Ready is true only when the
// release is the active production content, so appId can be opened inline (appSettings.autorun).
// Marker ("appId: <appId>", only when ready) is the line the agent ends its reply with: the
// Hermes run stream carries no tool results, so sim-api finds the form in the final Markdown.
type SmartFormRelease struct {
	AppID         string `json:"appId"`
	ReleaseID     int64  `json:"releaseId"`
	ReleaseNumber int64  `json:"releaseNumber"`
	Status        string `json:"status"`
	Ready         bool   `json:"ready"`
	Marker        string `json:"marker,omitempty"`
}

type appRequest struct {
	AccID   string `json:"accId"`
	ActorID string `json:"actorId"`
}
type structRequest struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	EnvID   int64    `json:"envId"`
	Fields  []string `json:"fields"`
}
type structWire struct {
	Tree ContentNode `json:"tree"`
}
type releaseCreateRequest struct {
	AccID       string `json:"accId"`
	ActorID     string `json:"actorId"`
	SourceEnvID int64  `json:"sourceEnvId"`
	TargetEnvID int64  `json:"targetEnvId"`
}
type releaseGetRequest struct {
	AccID     string `json:"accId"`
	ActorID   string `json:"actorId"`
	ReleaseID int64  `json:"releaseId"`
}

func smartFormEnvs(envs []Environment) (develop, production int64) {
	for _, env := range envs {
		switch env.Title {
		case "develop":
			develop = env.ID
		case "production":
			production = env.ID
		}
	}
	return develop, production
}

func (c *Client) envs(ctx context.Context, appID string) (int64, int64, error) {
	if !validID(appID) {
		return 0, 0, errors.New("appId required")
	}
	r, e := call[appRequest, Environments](ctx, c, "application/env/list", appRequest{AccID: c.workspace(), ActorID: appID})
	if e != nil {
		return 0, 0, e
	}
	dev, prod := smartFormEnvs(r.Envs)
	if dev < 1 || prod < 1 {
		return 0, 0, errors.New("smart form has no develop and production environments")
	}
	return dev, prod, nil
}

// CreateSmartForm creates a CDU v2 Smart Form with its seeded develop tree (pages/index/config).
func (c *Client) CreateSmartForm(ctx context.Context, in SmartFormCreateInput) (SmartForm, error) {
	if e := consoleOnly(c); e != nil {
		return SmartForm{}, e
	}
	if !smartFormRef.MatchString(in.Ref) || in.Title == "" || utf8.RuneCountInString(in.Title) > 255 || utf8.RuneCountInString(in.Description) > 10000 {
		return SmartForm{}, errors.New("ref ^[0-9a-zA-Z_-]{5,255}$ and a 1..255 character title required")
	}
	r, e := call[smartFormCreateRequest, CreatedApplication](ctx, c, "application/create", smartFormCreateRequest{AccID: c.workspace(), Ref: in.Ref,
		Title: in.Title, Description: in.Description, SharedWith: in.SharedWith, CduVersion: 2})
	if e != nil {
		return SmartForm{}, e
	}
	dev, prod := smartFormEnvs(r.Envs)
	if r.Application.ID == nil || *r.Application.ID == "" || dev < 1 || prod < 1 {
		return SmartForm{}, errors.New("application/create returned no Smart Form identity")
	}
	return SmartForm{AppID: *r.Application.ID, Ref: in.Ref, DevelopEnvID: dev, ProductionEnvID: prod}, nil
}

func written(appID string, files Files, want int, pending []string) (SmartFormWritten, error) {
	if len(files.Files) != want {
		return SmartFormWritten{}, errors.New("file write returned a different number of files")
	}
	out := SmartFormWritten{AppID: appID, Files: make([]SmartFormFile, len(files.Files)), Pending: pending}
	for i, f := range files.Files {
		out.Files[i] = SmartFormFile{ID: f.ID, FolderID: f.FolderID, Title: f.Title, SourceHash: f.SourceHash}
	}
	return out, nil
}

// CreateSmartFormFiles creates develop files in one transaction; page configs are CDU-checked.
func (c *Client) CreateSmartFormFiles(ctx context.Context, in SmartFormFilesCreateInput) (SmartFormWritten, error) {
	if e := consoleOnly(c); e != nil {
		return SmartFormWritten{}, e
	}
	if !validID(in.AppID) || len(in.Items) < 1 || len(in.Items) > 20 {
		return SmartFormWritten{}, errors.New("appId and 1..20 files required")
	}
	pending, err := wireConfigSources(in.Items, c.workspace(), in.AppID, func(it *SmartFormFileCreate) *string { return configSource(it.Title, &it.Source) })
	if err != nil {
		return SmartFormWritten{}, err
	}
	r, e := call[appFilesRequest[SmartFormFileCreate], Files](ctx, c, "app_file/create", appFilesRequest[SmartFormFileCreate]{AccID: c.workspace(), ActorID: in.AppID, Items: in.Items})
	if e != nil {
		return SmartFormWritten{}, e
	}
	return written(in.AppID, r, len(in.Items), pending)
}

// wireConfigSources runs wireSubmits on every page config source of items, in place, and returns
// what the pages still lack, each prefixed by its item index.
func wireConfigSources[T any](items []T, workspace, appID string, source func(*T) *string) ([]string, error) {
	var pending []string
	for i := range items {
		if s := source(&items[i]); s != nil {
			wired, left, err := wireSubmits(*s, workspace, appID)
			if err != nil {
				return nil, fmt.Errorf("file %d: %w", i, err)
			}
			*s = wired
			for _, p := range left {
				pending = append(pending, fmt.Sprintf("file %d: %s", i, p))
			}
		}
	}
	return pending, nil
}

// configSource is the source of a page config file, nil for another file or no source.
func configSource(title string, source *string) *string {
	if title != "config" {
		return nil
	}
	return source
}

// UpdateSmartFormFiles changes develop files in one transaction.
func (c *Client) UpdateSmartFormFiles(ctx context.Context, in SmartFormFilesUpdateInput) (SmartFormWritten, error) {
	if e := consoleOnly(c); e != nil {
		return SmartFormWritten{}, e
	}
	if !validID(in.AppID) || len(in.Items) < 1 || len(in.Items) > 20 {
		return SmartFormWritten{}, errors.New("appId and 1..20 files required")
	}
	for i, it := range in.Items {
		// Without the hash the write is last-write-wins over whatever a person saved in the editor.
		if it.Source != nil && it.IfMatchHash == "" {
			return SmartFormWritten{}, fmt.Errorf("item %d: ifMatchHash required to overwrite a file: send the sourceHash smart_form_files_get lists for it (it changes with every write)", i)
		}
	}
	pending, err := wireConfigSources(in.Items, c.workspace(), in.AppID, func(it *SmartFormFileUpdate) *string { return configSource(it.Title, it.Source) })
	if err != nil {
		return SmartFormWritten{}, err
	}
	r, e := call[appFilesRequest[SmartFormFileUpdate], Files](ctx, c, "app_file/update", appFilesRequest[SmartFormFileUpdate]{AccID: c.workspace(), ActorID: in.AppID, Items: in.Items})
	if e != nil {
		return SmartFormWritten{}, staleWrite(e, in.Items)
	}
	return written(in.AppID, r, len(in.Items), pending)
}

// checkPagesSend refuses a release while a develop page config would send nothing (a Form
// without its submit, inputs beside an action-less button): a write lets a page be built in parts,
// a release ships it as it is. It reads the page configs one by one: every source of a large form
// in one answer would pass the response limit.
func (c *Client) checkPagesSend(ctx context.Context, appID string, dev int64) error {
	files, e := c.developTree(ctx, appID, dev)
	if e != nil {
		return e
	}
	var problems []string
	for _, f := range files {
		if f.ObjType != "file" || !isPageConfig(f.Path) {
			continue
		}
		file, e := c.appFile(ctx, appID, f.ID)
		if e != nil {
			return e
		}
		for _, p := range unsentPage(file.Source) {
			problems = append(problems, f.Path+": "+p)
		}
	}
	if len(problems) > 0 {
		return errors.New("not released, a page would send nothing: " + strings.Join(problems, "; ") + ". Add the form.submit button (or the action sending the values), or create the data template and a ready page with smart_form_data_form_create")
	}
	return nil
}

// isPageConfig says path is pages/<page>/config (a page may sit in nested folders).
func isPageConfig(path string) bool {
	return strings.HasPrefix(path, "pages/") && strings.HasSuffix(path, "/config")
}

func released(appID string, r Release) SmartFormRelease {
	out := SmartFormRelease{AppID: appID, ReleaseID: r.ID, ReleaseNumber: r.ReleaseNumber, Status: r.Status,
		Ready: r.Status == "active" && (r.PrunedByRetention == nil || !*r.PrunedByRetention)}
	if out.Ready {
		out.Marker = "appId: " + appID
	}
	return out
}

// ReleaseSmartForm deploys develop into production; production then serves this release.
func (c *Client) ReleaseSmartForm(ctx context.Context, in SmartFormInput) (SmartFormRelease, error) {
	if e := consoleOnly(c); e != nil {
		return SmartFormRelease{}, e
	}
	dev, prod, e := c.envs(ctx, in.AppID)
	if e != nil {
		return SmartFormRelease{}, e
	}
	if e = c.checkPagesSend(ctx, in.AppID, dev); e != nil {
		return SmartFormRelease{}, e
	}
	r, e := call[releaseCreateRequest, Release](ctx, c, "release/create", releaseCreateRequest{AccID: c.workspace(), ActorID: in.AppID, SourceEnvID: dev, TargetEnvID: prod})
	if e != nil {
		return SmartFormRelease{}, e
	}
	if r.ID < 1 || r.ActorID != in.AppID || r.EnvID != prod {
		return SmartFormRelease{}, errors.New("release result identity mismatch")
	}
	return released(in.AppID, r), nil
}

type routeKeyWire struct {
	ActiveReleaseID int64 `json:"activeReleaseId"`
}

// GetSmartFormRelease reads the current state of one release; without releaseId, the release
// production serves now (route/key/get), so a form released before this run can be embedded.
func (c *Client) GetSmartFormRelease(ctx context.Context, in SmartFormReleaseInput) (SmartFormRelease, error) {
	if e := consoleOnly(c); e != nil {
		return SmartFormRelease{}, e
	}
	if in.ReleaseID < 0 {
		return SmartFormRelease{}, errors.New("appId and a positive releaseId (or none for the active one) required")
	}
	_, prod, e := c.envs(ctx, in.AppID)
	if e != nil {
		return SmartFormRelease{}, e
	}
	if in.ReleaseID == 0 {
		k, e := call[appRequest, routeKeyWire](ctx, c, "route/key/get", appRequest{AccID: c.workspace(), ActorID: in.AppID})
		if e != nil {
			return SmartFormRelease{}, e
		}
		if k.ActiveReleaseID < 1 {
			return SmartFormRelease{}, errors.New("the Smart Form has no active production release: release it with smart_form_release")
		}
		in.ReleaseID = k.ActiveReleaseID
	}
	r, e := call[releaseGetRequest, Release](ctx, c, "release/get", releaseGetRequest{AccID: c.workspace(), ActorID: in.AppID, ReleaseID: in.ReleaseID})
	if e != nil {
		return SmartFormRelease{}, e
	}
	if r.ID != in.ReleaseID || r.ActorID != in.AppID {
		return SmartFormRelease{}, errors.New("release result identity mismatch")
	}
	if r.EnvID != prod {
		return SmartFormRelease{}, errors.New("release is not a production release")
	}
	return released(in.AppID, r), nil
}
