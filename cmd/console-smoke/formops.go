package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type formTree struct {
	DevelopEnvID int64 `json:"developEnvId"`
	Files        []struct {
		ID         int64   `json:"id"`
		Path       string  `json:"path"`
		Type       string  `json:"type"`
		SourceHash string  `json:"sourceHash"`
		Source     *string `json:"source"`
	} `json:"files"`
}

func (t formTree) entry(path string) (int64, string, string) {
	for _, f := range t.Files {
		if f.Path == path {
			source := ""
			if f.Source != nil {
				source = *f.Source
			}
			return f.ID, f.SourceHash, source
		}
	}
	return 0, "", ""
}

type releaseRow struct {
	ID            int64  `json:"id"`
	ReleaseID     int64  `json:"releaseId"`
	ReleaseNumber int64  `json:"releaseNumber"`
	Status        string `json:"status"`
}

// formops (console-full): a second page in a folder of its own, two releases compared, the first
// rolled back to and read as the active release without its id, the page's file history and a
// rollback of its edit, the trash, the page rendered as the runtime does, and no Corezoid project yet.
func formops(c *client, stamp string) error {
	ref := "ai_smoke_ops_" + stamp
	var app struct {
		AppID string `json:"appId"`
	}
	if err := c.tool("smart_form_create", map[string]any{"ref": ref, "title": "Smoke ops " + stamp}, &app); err != nil {
		return err
	}
	var tree formTree
	if err := c.tool("smart_form_files_get", map[string]any{"appId": app.AppID, "withSource": true}, &tree); err != nil {
		return err
	}
	pages, _, _ := tree.entry("pages")
	_, _, index := tree.entry("pages/index/config")
	if pages == 0 || index == "" || tree.DevelopEnvID == 0 {
		return fmt.Errorf("new Smart Form has no pages folder, index config or develop env: %+v", tree)
	}
	var folders struct {
		Folders []struct {
			ID int64 `json:"id"`
		} `json:"folders"`
	}
	if err := c.tool("smart_form_folders_create", map[string]any{"actorId": app.AppID, "items": []any{map[string]any{"folderId": pages, "title": "thanks"}}}, &folders); err != nil {
		return err
	}
	if len(folders.Folders) != 1 {
		return fmt.Errorf("smart_form_folders_create made %d folders", len(folders.Folders))
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(index), &doc); err != nil {
		return fmt.Errorf("the seeded page config is not JSON: %w", err)
	}
	doc["id"] = "thanks"
	page, _ := json.Marshal(doc)
	var made struct {
		Files []struct {
			ID         int64  `json:"id"`
			SourceHash string `json:"sourceHash"`
		} `json:"files"`
	}
	if err := c.tool("smart_form_files_create", map[string]any{"appId": app.AppID, "items": []any{map[string]any{"folderId": folders.Folders[0].ID, "title": "config", "type": "application/json", "source": string(page)}}}, &made); err != nil {
		return err
	}
	first, err := release(c, app.AppID)
	if err != nil {
		return err
	}
	doc["routes"] = []map[string]any{{"name": "thanks", "path": "/thanks"}}
	edited, _ := json.Marshal(doc)
	var ignored json.RawMessage
	if err = c.tool("smart_form_files_update", map[string]any{"appId": app.AppID, "items": []any{map[string]any{"id": made.Files[0].ID, "title": "config", "type": "application/json", "source": string(edited), "ifMatchHash": made.Files[0].SourceHash}}}, &ignored); err != nil {
		return err
	}
	second, err := release(c, app.AppID)
	if err != nil {
		return err
	}
	if err = formReleases(c, app.AppID, first, second); err != nil {
		return err
	}
	if err = formFileHistory(c, app.AppID, made.Files[0].ID, tree.DevelopEnvID); err != nil {
		return err
	}
	var render struct {
		PageID string `json:"pageId"`
	}
	if err = c.tool("smart_form_page_render", map[string]any{"ref": ref, "envTitle": "develop", "page": "thanks"}, &render); err != nil {
		return err
	}
	if render.PageID != "thanks" {
		return fmt.Errorf("smart_form_page_render returned page %q", render.PageID)
	}
	var link struct {
		Status string `json:"status"`
	}
	if err = c.tool("corezoid_project_get", map[string]any{"actorId": app.AppID}, &link); err != nil {
		return err
	}
	if link.Status != "none" {
		return fmt.Errorf("a new Smart Form has the Corezoid link status %q, want none", link.Status)
	}
	fmt.Println("OK formops appId", app.AppID, "releases", first, second)
	return nil
}

func release(c *client, appID string) (int64, error) {
	var r releaseRow
	if err := c.tool("smart_form_release", map[string]any{"appId": appID}, &r); err != nil {
		return 0, err
	}
	return r.ReleaseID, nil
}

// formReleases: the releases listed and compared, then a rollback to the first, read back as the
// active release without its id.
func formReleases(c *client, appID string, first, second int64) error {
	var list struct {
		Releases []releaseRow `json:"releases"`
	}
	if err := c.tool("smart_form_releases_list", map[string]any{"actorId": appID}, &list); err != nil {
		return err
	}
	if len(list.Releases) != 2 || list.Releases[0].ID != second {
		return fmt.Errorf("smart_form_releases_list returned %+v, want %d then %d", list.Releases, second, first)
	}
	var diff struct {
		Modified []json.RawMessage `json:"modified"`
	}
	if err := c.tool("smart_form_release_diff", map[string]any{"actorId": appID, "releaseId": second, "vs": first}, &diff); err != nil {
		return err
	}
	if len(diff.Modified) != 1 {
		return fmt.Errorf("the releases differ in %d files, want the edited config", len(diff.Modified))
	}
	var back releaseRow
	if err := c.tool("smart_form_release_rollback", map[string]any{"actorId": appID, "releaseId": first}, &back); err != nil {
		return err
	}
	var active releaseRow
	if err := c.tool("smart_form_release_get", map[string]any{"appId": appID}, &active); err != nil {
		return err
	}
	if active.ReleaseID != back.ID || active.Status != "active" {
		return fmt.Errorf("the active release is %+v, want the rollback %d", active, back.ID)
	}
	return nil
}

// formFileHistory: the edited page's versions, the one before the edit read and rolled back to,
// and the develop trash listed and asked to restore a file that is not in it.
func formFileHistory(c *client, appID string, fileID, developEnv int64) error {
	var versions struct {
		Versions []struct {
			ID        int64  `json:"id"`
			Operation string `json:"operation"`
		} `json:"versions"`
	}
	if err := c.tool("smart_form_file_versions_list", map[string]any{"actorId": appID, "fileId": fileID}, &versions); err != nil {
		return err
	}
	var before int64
	for _, v := range versions.Versions {
		if v.Operation == "update" && before == 0 {
			before = v.ID
		}
	}
	if before == 0 {
		return fmt.Errorf("file %d has no update version: %+v", fileID, versions.Versions)
	}
	var version struct {
		Source string `json:"source"`
	}
	if err := c.tool("smart_form_file_version_get", map[string]any{"actorId": appID, "fileId": fileID, "versionId": before}, &version); err != nil {
		return err
	}
	if strings.Contains(version.Source, `"routes"`) {
		return fmt.Errorf("version %d holds the edit, not the file before it", before)
	}
	var ignored json.RawMessage
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"smart_form_file_rollback", map[string]any{"actorId": appID, "fileId": fileID, "versionId": before}},
		{"smart_form_trash_list", map[string]any{"actorId": appID, "envId": developEnv}},
		{"smart_form_trash_restore", map[string]any{"actorId": appID, "items": []any{map[string]any{"id": fileID, "objType": "file"}}}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	return nil
}
