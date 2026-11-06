package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// people: the access, reaction, user and file tools (console-full) on an actor the credential owns:
// its rules and its empty access-request waiting room, a request for it refused because the caller
// already sees it, a comment marked read, and a text file uploaded onto it and read back.
func people(c *client, stamp string) error {
	var actor id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke people " + stamp}, &actor); err != nil {
		return err
	}
	var ignored json.RawMessage
	for _, step := range []struct {
		tool string
		args map[string]any
	}{
		{"users_search", map[string]any{"limit": 1}},
		{"workspaces_list", map[string]any{"limit": 1}},
		{"access_rules_get", map[string]any{"objType": "actor", "objId": actor.ID}},
		{"access_requests_list", map[string]any{"actorId": actor.ID}},
		{"reaction_create", map[string]any{"rootActorId": actor.ID, "type": "comment", "description": "smoke comment " + stamp, "notify": false}},
		{"reactions_list", map[string]any{"rootActorId": actor.ID, "view": "flat", "limit": 5}},
		{"reactions_mark_read", map[string]any{"rootActorId": actor.ID, "time": time.Now().Unix()}},
	} {
		if err := c.tool(step.tool, step.args, &ignored); err != nil {
			return err
		}
	}
	// The owner sees the actor, so sim-api refuses a request for it: the call reached the method.
	if err := c.tool("access_request_create", map[string]any{"actorId": actor.ID}, &ignored); err == nil || !strings.Contains(err.Error(), "reason=notAllowed") || !strings.Contains(err.Error(), "already has access") {
		return fmt.Errorf("access_request_create on an own actor: want the already-has-access refusal, got %w", err)
	}
	text := "smoke file " + stamp
	var file struct {
		ID         int64  `json:"id"`
		FileName   string `json:"fileName"`
		AttachedTo string `json:"attachedTo"`
	}
	if err := c.tool("attachment_upload", map[string]any{"fileName": "smoke-" + stamp + ".txt", "content": base64.StdEncoding.EncodeToString([]byte(text)), "actorId": actor.ID}, &file); err != nil {
		return err
	}
	if file.AttachedTo != actor.ID {
		return fmt.Errorf("attachment %d was not attached to %s", file.ID, actor.ID)
	}
	var files struct {
		List []struct {
			ID int64 `json:"id"`
		} `json:"list"`
	}
	if err := c.tool("attachments_by_actor_list", map[string]any{"actorId": actor.ID}, &files); err != nil {
		return err
	}
	if len(files.List) == 0 || files.List[0].ID != file.ID {
		return fmt.Errorf("actor %s lists %v, want attachment %d", actor.ID, files.List, file.ID)
	}
	var read struct {
		Text string `json:"text"`
	}
	if err := c.tool("attachment_read", map[string]any{"fileName": file.FileName}, &read); err != nil {
		// A stand whose antivirus is still scanning answers "not finished": the route was reached.
		if !strings.Contains(err.Error(), "scan has not finished") {
			return err
		}
		fmt.Println("SKIP attachment_read: the scan has not finished yet")
	} else if read.Text != text {
		return fmt.Errorf("attachment_read returned %q, want %q", read.Text, text)
	}
	fmt.Println("OK people actor", actor.ID, "attachment", file.ID)
	return nil
}
