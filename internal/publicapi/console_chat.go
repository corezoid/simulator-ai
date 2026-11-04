package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// A chat is an actor of the Events system template with data.chatType p2p or group; its
// participants are the actor's access rules and its messages are comment reactions. The web UI
// reuses a p2p chat by ref p2pConversation_<min userId>_<max userId>, titled
// "<nick of min>:p2p:<nick of max>"; group chats are never reused.

type ChatOpenInput struct {
	Type    string  `json:"type"`            // p2p or group
	UserIDs []int64 `json:"userIds"`         // the other participants
	Title   string  `json:"title,omitempty"` // group only
}

type ChatOpened struct {
	ChatID       string  `json:"chatId"`
	Title        string  `json:"title"`
	Type         string  `json:"type"`
	Created      bool    `json:"created"`
	Participants []int64 `json:"participants"`
}

var p2pRef = regexp.MustCompile(`^p2pConversation_(\d+)_(\d+)$`)

// chatPrivs are what the web UI gives a chat participant.
var chatPrivs = json.RawMessage(`{"view":true,"modify":true,"remove":true}`)

// OpenChat reuses the p2p chat with the person you share, else creates the chat and shares it with
// its participants. The creator owns it, so you are never added as a rule.
func (c *Client) OpenChat(ctx context.Context, in ChatOpenInput) (ChatOpened, error) {
	if e := consoleOnly(c); e != nil {
		return ChatOpened{}, e
	}
	switch {
	case in.Type == "p2p" && (len(in.UserIDs) != 1 || in.Title != ""):
		return ChatOpened{}, errors.New("a p2p chat takes exactly one other userId and no title (the title is the two nicks)")
	case in.Type == "group" && (len(in.UserIDs) < 1 || !validTitle(in.Title, false)):
		return ChatOpened{}, errors.New("a group chat takes a title and at least one other userId")
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
	if in.Type == "p2p" {
		if found, err := c.findP2P(ctx, events, in.UserIDs[0]); err != nil || found.ChatID != "" {
			return found, err
		}
	}
	title := in.Title
	if in.Type == "p2p" {
		title = "p2p" // replaced below, once the creator's id is known
	}
	now := time.Now().Unix()
	chat, err := call[map[string]any, struct {
		ID     string `json:"id"`
		UserID int64  `json:"userId"` // the creator: you
	}](ctx, c, "actor/create", map[string]any{"accId": c.config.Workspace, "formId": events, "title": title, "data": map[string]any{"chatType": in.Type, "startDate": now, "endDate": now, "scheduleMeeting": false}, "fields": []string{"id", "userId"}})
	if err != nil {
		return ChatOpened{}, fmt.Errorf("actor/create: %w", err)
	}
	if !validID(chat.ID) {
		return ChatOpened{}, errors.New("actor/create returned no chat id")
	}
	out := ChatOpened{ChatID: chat.ID, Title: title, Type: in.Type, Created: true, Participants: in.UserIDs}
	if in.Type == "p2p" {
		if out.Title, err = c.nameP2P(ctx, chat.ID, chat.UserID, in.UserIDs[0]); err != nil {
			return ChatOpened{}, fmt.Errorf("chat %s was created but not named for reuse: %w", chat.ID, err)
		}
	}
	rules := make([]AccessRule, len(in.UserIDs))
	for i, id := range in.UserIDs {
		rules[i] = AccessRule{UserID: id, Privs: chatPrivs}
	}
	if _, err := c.SaveAccessRules(ctx, AccessRulesSaveInput{AccessObject: AccessObject{ObjType: "actor", ObjID: chat.ID}, Rules: rules}); err != nil {
		return ChatOpened{}, fmt.Errorf("chat %s was created but not shared (share it with access_rules_save): %w", chat.ID, err)
	}
	return out, nil
}

// findP2P finds a p2p chat you see that the person is a member of and whose ref names them.
func (c *Client) findP2P(ctx context.Context, events, userID int64) (ChatOpened, error) {
	page, err := call[map[string]any, struct {
		List []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Ref   string `json:"ref"`
		} `json:"list"`
	}](ctx, c, "actor/filter/list", map[string]any{"accId": c.config.Workspace, "formId": events, "stream": "p2p", "members": []map[string]any{{"userId": userID, "access": []string{}}}, "fields": []string{"id", "title", "ref"}, "limit": 100})
	if err != nil {
		return ChatOpened{}, fmt.Errorf("actor/filter/list: %w", err)
	}
	want := strconv.FormatInt(userID, 10)
	for _, a := range page.List {
		if m := p2pRef.FindStringSubmatch(a.Ref); m != nil && (m[1] == want || m[2] == want) && m[1] != m[2] {
			return ChatOpened{ChatID: a.ID, Title: a.Title, Type: "p2p", Participants: []int64{userID}}, nil
		}
	}
	return ChatOpened{}, nil
}

// nameP2P gives a new p2p chat the web UI's ref and title, so the UI and the next chat_open reuse it.
func (c *Client) nameP2P(ctx context.Context, chatID string, self, other int64) (string, error) {
	if self < 1 || self == other {
		return "", errors.New("actor/create did not name a creator other than the participant")
	}
	low, high := min(self, other), max(self, other)
	users, err := c.PassEach(ctx, "user/get", []map[string]json.RawMessage{rawParams(map[string]any{"userId": low}), rawParams(map[string]any{"userId": high})})
	if err != nil {
		return "", err
	}
	nicks := make([]string, 2)
	for i, r := range users {
		var u struct {
			Nick string `json:"nick"`
		}
		if r.Error != "" || json.Unmarshal(r.Result, &u) != nil || u.Nick == "" {
			nicks[i] = strconv.FormatInt([]int64{low, high}[i], 10)
			continue
		}
		nicks[i] = u.Nick
	}
	title := nicks[0] + ":p2p:" + nicks[1]
	ref := fmt.Sprintf("p2pConversation_%d_%d", low, high)
	if _, err := call[map[string]any, named](ctx, c, "actor/update", map[string]any{"accId": c.config.Workspace, "actorId": chatID, "title": title, "ref": ref, "fields": []string{"id"}}); err != nil {
		return "", err
	}
	return title, nil
}
