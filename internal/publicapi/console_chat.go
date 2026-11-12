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
	// Notified says the other side of a new p2p chat was told about it.
	Notified bool `json:"notified,omitempty"`
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
		out, _, err := c.createChat(ctx, events, in, in.Title, "", true, false)
		return out, err
	}
	who, err := c.selfUserID(ctx, in.SelfUserID)
	self, other := who.ID, in.UserIDs[0]
	switch {
	case err != nil:
		return ChatOpened{}, err
	case self < 1 || self == other:
		return ChatOpened{}, errors.New("a p2p chat needs selfUserId: the userId of the user you act for, as the request names it, other than the participant's")
	}
	in.SelfUserID = self
	ref := fmt.Sprintf("p2pConversation_%d_%d", min(self, other), max(self, other))
	// reuse answers an existing chat. An admin sees chats of others, so an id the model gave (not
	// the credential's) must be one of the chat's people, with the credential's saId when it names one.
	reuse := func(found ChatOpened) (ChatOpened, error) {
		if who.Trusted {
			return found, nil
		}
		member, err := c.isParticipant(ctx, found.ChatID, who)
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
	title, err := c.p2pTitle(ctx, min(self, other), max(self, other), self)
	if err != nil {
		return ChatOpened{}, err
	}
	// The web UI tells the other side of a new p2p chat. A caller known from the credential shares
	// and notifies in the create batch. Otherwise the chat is created alone and shared after its
	// creator is checked: sim-api notifies only a new rule, and a wrong id must reach nobody.
	out, owner, err := c.createChat(ctx, events, in, title, ref, who.Trusted, who.Trusted)
	if err != nil {
		// The ref is unique within the template: a chat created meanwhile is the one to reuse.
		if found, again := c.chatByRef(ctx, events, ref, other); again == nil && found.ChatID != "" {
			return reuse(found)
		}
		return ChatOpened{}, err
	}
	if owner != self {
		// The ref names someone else: take the quiet, just-made chat back.
		if _, derr := call[map[string]any, json.RawMessage](ctx, c, "actor/delete", map[string]any{"accId": c.workspace(), "actorId": out.ChatID}); derr != nil {
			return ChatOpened{}, fmt.Errorf("selfUserId %d is not you (chat %s was created by user %d) and the chat could not be deleted (%w): delete it, then pass the userId the request names", self, out.ChatID, owner, derr)
		}
		return ChatOpened{}, fmt.Errorf("selfUserId %d is not you: the chat was created by user %d and removed again; pass the userId the request names", self, owner)
	}
	if !who.Trusted {
		if _, err := call[map[string]any, json.RawMessage](ctx, c, "access_rule/actor/share", map[string]any{"accId": c.workspace(), "actorId": out.ChatID, "actions": []accessAction{{Action: "create", UserID: other, Privs: chatPrivs}}, "isNotify": true}); err != nil {
			// A chat its other side cannot open is no chat: take it back, so the next call starts clean.
			if _, derr := call[map[string]any, json.RawMessage](ctx, c, "actor/delete", map[string]any{"accId": c.workspace(), "actorId": out.ChatID}); derr != nil {
				return ChatOpened{}, fmt.Errorf("chat %s was created but not shared with user %d (%w) nor deleted (%w): share it with access_rules_save or delete it", out.ChatID, other, err, derr)
			}
			return ChatOpened{}, fmt.Errorf("the chat could not be shared with user %d and was removed again: %w", other, err)
		}
	}
	out.Notified = true
	return out, nil
}

// chatByRef reads the p2p chat with this exact ref; none is the zero result.
func (c *Client) chatByRef(ctx context.Context, events int64, ref string, other int64) (ChatOpened, error) {
	chat, err := call[map[string]any, struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}](ctx, c, "actor/by-ref/get", map[string]any{"accId": c.workspace(), "formId": events, "ref": ref, "fields": []string{"id", "title"}})
	var api *APIError
	switch {
	case errors.As(err, &api) && api.Reason == "notFound":
		return ChatOpened{}, nil
	case err != nil:
		return ChatOpened{}, fmt.Errorf("actor/by-ref/get: %w", err)
	}
	return ChatOpened{ChatID: chat.ID, Title: chat.Title, Type: "p2p", Participants: []int64{other}}, nil
}

// createChat creates the chat and, with share, shares it with the participants in the same atomic
// batch (notify tells them); it answers the chat and its creator.
func (c *Client) createChat(ctx context.Context, events int64, in ChatOpenInput, title, ref string, share, notify bool) (ChatOpened, int64, error) {
	now := time.Now().Unix()
	chat := map[string]any{"accId": c.workspace(), "formId": events, "title": title, "data": map[string]any{"chatType": in.Type, "startDate": now, "endDate": now, "scheduleMeeting": false}, "fields": []string{"id", "userId"}}
	if ref != "" {
		chat["ref"] = ref
	}
	actions := make([]accessAction, len(in.UserIDs))
	for i, id := range in.UserIDs {
		actions[i] = accessAction{Action: "create", UserID: id, Privs: chatPrivs}
	}
	items := []atomicItem{{Method: "actor/create", Params: chat}}
	if share {
		items = append(items, atomicItem{Method: "access_rule/actor/share", Params: map[string]any{"accId": c.workspace(), "actorId": "$0.id", "actions": actions, "isNotify": notify}, UseRefs: true})
	}
	results, err := c.atomicItems(ctx, items)
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

// p2pTitle is the web UI's p2p title from the two nicks. user/get needs viewUsers: the caller's own
// nick then comes from the credential, and an id stands in for a nick nothing gives.
func (c *Client) p2pTitle(ctx context.Context, low, high, self int64) (string, error) {
	users, err := c.PassEach(ctx, "user/get", []map[string]json.RawMessage{rawParams(map[string]any{"userId": low}), rawParams(map[string]any{"userId": high})})
	if err != nil {
		return "", err
	}
	nicks := []string{strconv.FormatInt(low, 10), strconv.FormatInt(high, 10)}
	if user, ok := c.credentialUser(ctx); ok && user.Nick != "" {
		nicks[map[bool]int{true: 0, false: 1}[self == low]] = user.Nick
	}
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
