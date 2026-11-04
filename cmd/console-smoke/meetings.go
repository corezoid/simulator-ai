package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// meetings (console-full): a meeting (an Events actor with scheduleMeeting) given a guest link,
// read back, revoked and gone, and its transcript read, empty with no call running.
func meetings(c *client, stamp string) error {
	var forms struct {
		List []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"list"`
	}
	if err := c.tool("forms_list", map[string]any{"type": "system", "query": "Events"}, &forms); err != nil {
		return err
	}
	events := int64(0)
	for _, f := range forms.List {
		if f.Title == "Events" {
			events = f.ID
		}
	}
	if events == 0 {
		return fmt.Errorf("the workspace has no Events system template")
	}
	now := time.Now().Unix()
	var meeting id
	if err := c.tool("actor_create", map[string]any{"formId": events, "title": "AI smoke meeting " + stamp,
		"data": map[string]any{"scheduleMeeting": true, "startDate": now, "endDate": now + 3600}}, &meeting); err != nil {
		return err
	}
	var made struct {
		Hash string `json:"hash"`
		URL  string `json:"url"`
	}
	if err := c.tool("public_link_create", map[string]any{"actorId": meeting.ID, "ttl": 600}, &made); err != nil {
		return err
	}
	var read struct {
		Link *struct {
			Hash string `json:"hash"`
		} `json:"link"`
	}
	if err := c.tool("public_link_get", map[string]any{"actorId": meeting.ID}, &read); err != nil {
		return err
	}
	if made.Hash == "" || !strings.Contains(made.URL, made.Hash) || read.Link == nil || read.Link.Hash != made.Hash {
		return fmt.Errorf("public_link_create gave %+v, public_link_get %+v", made, read.Link)
	}
	var revoked struct {
		Revoked   bool `json:"revoked"`
		Withdrawn int  `json:"withdrawn"`
	}
	if err := c.tool("public_link_revoke", map[string]any{"actorId": meeting.ID}, &revoked); err != nil {
		return err
	}
	if err := c.tool("public_link_get", map[string]any{"actorId": meeting.ID}, &read); err != nil {
		return err
	}
	if !revoked.Revoked || revoked.Withdrawn != 0 || read.Link != nil {
		return fmt.Errorf("public_link_revoke answered %+v and the link reads %+v", revoked, read.Link)
	}
	var transcript struct {
		List []json.RawMessage `json:"list"`
	}
	if err := c.tool("meet_transcription_list", map[string]any{"actorId": meeting.ID}, &transcript); err != nil {
		if strings.Contains(err.Error(), "reason=notAvailable") {
			fmt.Println("SKIP meet_transcription_list: meetings are not configured on this stand")
		} else {
			return err
		}
	} else if len(transcript.List) != 0 {
		return fmt.Errorf("a meeting with no call has %d transcript lines", len(transcript.List))
	}
	fmt.Println("OK meetings meeting", meeting.ID)
	return nil
}
