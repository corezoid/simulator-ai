package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// A chat is an actor of the Events system template with data.chatType p2p or group; its
// participants are the actor's access rules and its messages are comment reactions. The web UI
// (ui-kit useOpenDirectChat) reuses a p2p chat by its exact ref p2pConversation_<min userId>_<max
// userId>, titled "<nick of min>:p2p:<nick of max>"; group chats are never reused.

type ChatOpenInput struct {
	Type    string  `json:"type"`            // p2p or group
	UserIDs []int64 `json:"userIds"`         // the other participants
	Title   string  `json:"title,omitempty"` // group only
	// SelfUserID is the requesting user, whose id the run's prompt names ("userId N — the user you
	// act for"). A Simulator JWT names the caller itself (selfUserID), which wins; with another
	// credential this is taken as given and a reused chat must list it among its participants.
	SelfUserID int64 `json:"selfUserId,omitempty"`
}

type ChatOpened struct {
	ChatID       string  `json:"chatId"`
	Title        string  `json:"title"`
	Type         string  `json:"type"`
	Created      bool    `json:"created"`
	Participants []int64 `json:"participants"`
}

// chatPrivs are what the web UI gives a chat participant.
var chatPrivs = json.RawMessage(`{"view":true,"modify":true,"remove":true}`)

// OpenChat reuses the p2p chat of exactly these two people, else creates the chat and shares it
// with its participants in one atomic batch. The creator owns it, so you are never added as a rule.
func (c *Client) OpenChat(ctx context.Context, in ChatOpenInput) (ChatOpened, error) {
	if e := consoleOnly(c); e != nil {
		return ChatOpened{}, e
	}
	switch {
	case in.Type == "p2p" && (len(in.UserIDs) != 1 || in.Title != ""):
		return ChatOpened{}, errors.New("a p2p chat takes exactly one other userId and no title (the title is the two nicks)")
	case in.Type == "group" && (len(in.UserIDs) < 1 || !validTitle(in.Title, false) || in.SelfUserID != 0):
		return ChatOpened{}, errors.New("a group chat takes a title and at least one other userId (no selfUserId)")
	case in.Type != "p2p" && in.Type != "group":
		return ChatOpened{}, errors.New("type is p2p or group")
	}
	for _, id := range in.UserIDs {
		if id < 1 {
			return ChatOpened{}, errors.New("userIds are positive user ids from users_search")
		}
	}
	events, err := c.systemFormID(ctx, "Events")
	if err != nil {
		return ChatOpened{}, err
	}
	if in.Type == "group" {
		out, _, err := c.createChat(ctx, events, in, in.Title, "")
		return out, err
	}
	self, trusted, err := c.selfUserID(ctx, in.SelfUserID)
	other := in.UserIDs[0]
	switch {
	case err != nil:
		return ChatOpened{}, err
	case self < 1 || self == other:
		return ChatOpened{}, errors.New("a p2p chat needs selfUserId: the userId of the user you act for, as the request names it, other than the participant's")
	}
	in.SelfUserID = self
	ref := fmt.Sprintf("p2pConversation_%d_%d", min(self, other), max(self, other))
	// reuse answers an existing chat. An admin sees chats of others, so an id the model gave (not
	// the credential's) must be one of the chat's people.
	reuse := func(found ChatOpened) (ChatOpened, error) {
		if trusted {
			return found, nil
		}
		member, err := c.isParticipant(ctx, found.ChatID, self)
		if err == nil && !member {
			err = fmt.Errorf("selfUserId %d is not a participant of chat %s: pass the userId of the user you act for", self, found.ChatID)
		}
		if err != nil {
			return ChatOpened{}, err
		}
		return found, nil
	}
	if found, err := c.chatByRef(ctx, events, ref, other); err != nil || found.ChatID != "" {
		if err != nil {
			return ChatOpened{}, err
		}
		return reuse(found)
	}
	title, err := c.p2pTitle(ctx, min(self, other), max(self, other))
	if err != nil {
		return ChatOpened{}, err
	}
	out, owner, err := c.createChat(ctx, events, in, title, ref)
	if err != nil {
		// The ref is unique within the template: a chat created meanwhile is the one to reuse.
		if found, again := c.chatByRef(ctx, events, ref, other); again == nil && found.ChatID != "" {
			return reuse(found)
		}
		return ChatOpened{}, err
	}
	if owner != self {
		return ChatOpened{}, fmt.Errorf("chat %s was created by user %d, not selfUserId %d, so its ref names someone else: delete it and pass the userId the request names", out.ChatID, owner, self)
	}
	return out, nil
}

// chatByRef reads the p2p chat with this exact ref; none is the zero result.
func (c *Client) chatByRef(ctx context.Context, events int64, ref string, other int64) (ChatOpened, error) {
	chat, err := call[map[string]any, struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}](ctx, c, "actor/by-ref/get", map[string]any{"accId": c.config.Workspace, "formId": events, "ref": ref, "fields": []string{"id", "title"}})
	var api *APIError
	switch {
	case errors.As(err, &api) && api.Reason == "notFound":
		return ChatOpened{}, nil
	case err != nil:
		return ChatOpened{}, fmt.Errorf("actor/by-ref/get: %w", err)
	}
	return ChatOpened{ChatID: chat.ID, Title: chat.Title, Type: "p2p", Participants: []int64{other}}, nil
}

// createChat creates the chat and shares it with the participants in one atomic batch; it answers
// the chat and its creator.
func (c *Client) createChat(ctx context.Context, events int64, in ChatOpenInput, title, ref string) (ChatOpened, int64, error) {
	now := time.Now().Unix()
	chat := map[string]any{"accId": c.config.Workspace, "formId": events, "title": title, "data": map[string]any{"chatType": in.Type, "startDate": now, "endDate": now, "scheduleMeeting": false}, "fields": []string{"id", "userId"}}
	if ref != "" {
		chat["ref"] = ref
	}
	actions := make([]accessAction, len(in.UserIDs))
	for i, id := range in.UserIDs {
		actions[i] = accessAction{Action: "create", UserID: id, Privs: chatPrivs}
	}
	results, err := c.atomicItems(ctx, []atomicItem{
		{Method: "actor/create", Params: chat},
		// The web UI tells the other side of a p2p chat it was opened; group invitations stay quiet.
		{Method: "access_rule/actor/share", Params: map[string]any{"accId": c.config.Workspace, "actorId": "$0.id", "actions": actions, "isNotify": in.Type == "p2p"}, UseRefs: true},
	})
	if err != nil {
		return ChatOpened{}, 0, err
	}
	var created struct {
		ID     string `json:"id"`
		UserID int64  `json:"userId"`
	}
	if json.Unmarshal(results[0], &created) != nil || !validID(created.ID) {
		return ChatOpened{}, 0, errors.New("batch committed but actor/create returned no chat id; read the chat back by its ref before retrying")
	}
	return ChatOpened{ChatID: created.ID, Title: title, Type: in.Type, Created: true, Participants: in.UserIDs}, created.UserID, nil
}

// p2pTitle is the web UI's p2p title from the two nicks, an id standing in for a nick it cannot read.
func (c *Client) p2pTitle(ctx context.Context, low, high int64) (string, error) {
	users, err := c.PassEach(ctx, "user/get", []map[string]json.RawMessage{rawParams(map[string]any{"userId": low}), rawParams(map[string]any{"userId": high})})
	if err != nil {
		return "", err
	}
	nicks := []string{strconv.FormatInt(low, 10), strconv.FormatInt(high, 10)}
	for i, r := range users {
		var u struct {
			Nick string `json:"nick"`
		}
		if r.Error == "" && json.Unmarshal(r.Result, &u) == nil && u.Nick != "" {
			nicks[i] = u.Nick
		}
	}
	return nicks[0] + ":p2p:" + nicks[1], nil
}
