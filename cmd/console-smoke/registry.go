package main

import (
	"fmt"
	"os"
	"strings"
)

// registry: a verified skill saved through actor_create on the Skills template, found by a word of
// its body and read back by ref, then a link to it (the console core catalog).
func registry(c *client, stamp string) error {
	var forms struct {
		List []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"list"`
	}
	if err := c.tool("forms_list", map[string]any{"type": "system", "query": "Skills"}, &forms); err != nil {
		return err
	}
	skillsForm := int64(0)
	for _, f := range forms.List {
		if f.Title == "Skills" {
			skillsForm = f.ID
		}
	}
	if skillsForm == 0 {
		return fmt.Errorf("the workspace has no Skills system template")
	}
	ref, word := "ai-smoke-skill-"+strings.ReplaceAll(stamp, "_", "-"), "smokeword"+stamp
	body := "[md]\n# Smoke skill " + word + "\n## Steps\n1. actor_get the actor the user names.\n[/md]"
	var skill id
	if err := c.tool("actor_create", map[string]any{"formId": skillsForm, "title": "Smoke skill " + stamp, "ref": ref, "status": "verified", "description": body}, &skill); err != nil {
		return err
	}
	var found struct {
		List []struct {
			Ref    string `json:"ref"`
			Status string `json:"status"`
		} `json:"list"`
	}
	if err := c.tool("skills_find", map[string]any{"query": word}, &found); err != nil {
		return err
	}
	if len(found.List) != 1 || found.List[0].Ref != ref || found.List[0].Status != "verified" {
		return fmt.Errorf("skills_find %q returned %v, want the skill %s", word, found.List, ref)
	}
	var read struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := c.tool("skill_get", map[string]any{"ref": ref}, &read); err != nil {
		return err
	}
	if read.ID != skill.ID || read.Description != body {
		return fmt.Errorf("skill_get %s returned %s with another body", ref, read.ID)
	}
	var link struct {
		URL string `json:"url"`
	}
	if err := c.tool("build_link", map[string]any{"entity": "actor", "id": skill.ID}, &link); err != nil {
		return err
	}
	if want := "/actors_graph/" + os.Getenv("SIMULATOR_AI_WORKSPACE")[:8] + "/view/" + skill.ID; !strings.HasSuffix(link.URL, want) {
		return fmt.Errorf("build_link returned %s, want …%s", link.URL, want)
	}
	fmt.Println("OK registry skill", skill.ID, ref)
	return nil
}

// agents: the agent registry and the BBCode catalog (console-full): the members, a member's twin
// with its profile, and that twin found again by a text search of the System template.
func agents(c *client, _ string) error {
	var members struct {
		Members []struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"members"`
	}
	if err := c.tool("agents_find", map[string]any{"limit": 20}, &members); err != nil {
		return err
	}
	var twin struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		SystemObjType string `json:"systemObjType"`
		SystemObjID   string `json:"systemObjId"`
	}
	read := false
	for _, m := range members.Members {
		if m.Type != "user" {
			continue
		}
		err := c.tool("agent_get", map[string]any{"userId": m.ID}, &twin)
		if err != nil && strings.Contains(err.Error(), "has no twin actor") {
			continue
		}
		if err != nil {
			return err
		}
		if twin.SystemObjType != "user" || twin.SystemObjID != fmt.Sprint(m.ID) {
			return fmt.Errorf("agent_get userId %d returned actor %s of %s %s", m.ID, twin.ID, twin.SystemObjType, twin.SystemObjID)
		}
		read = true
		break
	}
	if !read {
		return fmt.Errorf("none of %d members has a twin actor to read", len(members.Members))
	}
	var found struct {
		Agents []struct {
			ID            string `json:"id"`
			SystemObjType string `json:"systemObjType"`
		} `json:"agents"`
	}
	if err := c.tool("agents_find", map[string]any{"query": twin.Title, "searchType": "text", "limit": 20}, &found); err != nil {
		return err
	}
	hit := false
	for _, a := range found.Agents {
		hit = hit || (a.ID == twin.ID && a.SystemObjType == "user")
	}
	if !hit {
		return fmt.Errorf("agents_find %q did not find the twin %s", twin.Title, twin.ID)
	}
	var tags struct {
		Tags []struct {
			Tag string `json:"tag"`
		} `json:"tags"`
	}
	if err := c.tool("bbcode_tags_get", map[string]any{}, &tags); err != nil {
		return err
	}
	if len(tags.Tags) < 10 {
		return fmt.Errorf("bbcode_tags_get returned %d tags", len(tags.Tags))
	}
	fmt.Println("OK agents twin", twin.ID)
	return nil
}
