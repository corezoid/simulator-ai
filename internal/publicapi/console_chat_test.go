package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// chatBackend answers by-ref reads with byRef (notFound when empty) and creates chat-1 as user 12.
func chatBackend(t *testing.T, byRef string) (*Client, *[]rpcCall) {
	return rpcBackend(t, func(method string) string {
		switch method {
		case "form/system/get":
			return `{"id":9,"title":"Events","type":"system"}`
		case "actor/by-ref/get":
			if byRef == "" {
				return notFound
			}
			return byRef
		case "access_rule/actor/list": // the chat of 5 and 12
			return `{"list":[{"userId":12},{"userId":5}]}`
		case "batch": // the nick reads fall back to ids; the create answers as user 12
			return batchOf(`{"id":"0","result":{"id":"chat-1","userId":12}}`, `{"id":"1","result":{"list":[]}}`)
		}
		return `{}`
	})
}

func TestOpenChatReusesTheExactP2PChat(t *testing.T) {
	c, calls := chatBackend(t, `{"id":"chat-0","title":"Olena:p2p:Ivan"}`)
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12})
	if err != nil || out.ChatID != "chat-0" || out.Created {
		t.Fatal(out, err)
	}
	// The exact web UI key, never a search a workspace admin could match a stranger's chat with;
	// a selfUserId the model gave must be one of the chat's people.
	got := strings.Join(methods(*calls), "\n")
	if len(*calls) != 3 || !strings.Contains(got, `actor/by-ref/get {"accId":"ws","fields":["id","title"],"formId":9,"ref":"p2pConversation_5_12"}`) || !strings.Contains(got, `access_rule/actor/list {"accId":"ws","actorId":"chat-0"}`) {
		t.Fatal(got)
	}
	// An admin who sees the chat of 5 and 12 but passes 12 as itself is refused.
	if _, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{12}, SelfUserID: 5}); err != nil {
		t.Fatal("5 is a participant:", err)
	}
	if _, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 99}); err == nil || !strings.Contains(err.Error(), "not a participant") {
		t.Fatal("a stranger's chat was reused:", err)
	}
}

func TestOpenChatCreatesAndSharesAtomically(t *testing.T) {
	batches := 0
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "form/system/get":
			return `{"id":9,"title":"Events","type":"system"}`
		case "actor/by-ref/get":
			return notFound
		case "batch":
			if batches++; batches == 1 {
				return `{"list":[{"id":"0","result":{"id":5,"nick":"Olena"}},{"id":"1","result":{"id":12,"nick":"Ivan"}}]}`
			}
			return batchOf(`{"id":"0","result":{"id":"chat-1","userId":12}}`, `{"id":"1","result":{"list":[]}}`)
		}
		return `{}`
	})
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12})
	if err != nil || out.ChatID != "chat-1" || !out.Created || out.Title != "Olena:p2p:Ivan" {
		t.Fatal(out, err)
	}
	last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	for _, want := range []string{
		`"isAtomic":true`,
		`"method":"actor/create","params":{"accId":"ws","data":{"chatType":"p2p"`,
		`"ref":"p2pConversation_5_12","title":"Olena:p2p:Ivan"}`,
		`"method":"access_rule/actor/share","params":{"accId":"ws","actions":[{"action":"create","privs":{"modify":true,"remove":true,"view":true},"userId":5}],"actorId":"$0.id","isNotify":true},"useRefs":true`,
	} {
		if !strings.Contains(string(last), want) {
			t.Errorf("batch lacks %s: %s", want, last)
		}
	}
	for _, m := range methods(*calls) {
		if strings.HasPrefix(m, "actor/update") || strings.HasPrefix(m, "access_rule/actor/share") {
			t.Error("the chat is named and shared inside the atomic batch, not after it:", m)
		}
	}
}

// A selfUserId that is not the caller is caught: the chat's creator is someone else.
func TestOpenChatRefusesAForeignSelf(t *testing.T) {
	c, _ := chatBackend(t, "")
	if _, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 77}); err == nil || !strings.Contains(err.Error(), "not selfUserId 77") {
		t.Fatal(err)
	}
}

// A failed create (the ref taken meanwhile) reuses the chat the second read finds.
func TestOpenChatRaceReusesTheWinner(t *testing.T) {
	reads := 0
	c, _ := rpcBackend(t, func(method string) string {
		switch method {
		case "form/system/get":
			return `{"id":9,"title":"Events","type":"system"}`
		case "actor/by-ref/get":
			if reads++; reads == 1 {
				return notFound
			}
			return `{"id":"chat-w","title":"Olena:p2p:Ivan"}`
		case "access_rule/actor/list":
			return `{"list":[{"userId":12},{"userId":5}]}`
		case "batch":
			if reads == 1 {
				return `{"list":[{"id":"0","result":{"nick":"Olena"}},{"id":"1","result":{"nick":"Ivan"}}]}`
			}
		}
		return `{"isAtomic":true,"rolledBack":true,"list":[{"id":"0","error":{"code":1,"message":"ref is taken","data":{"reason":"invalidParams"}}},{"id":"1"}]}`
	})
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12})
	if err != nil || out.ChatID != "chat-w" || out.Created {
		t.Fatal(out, err)
	}
}

func TestOpenChatGroupAndBadInput(t *testing.T) {
	c, calls := chatBackend(t, "")
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "group", Title: "Team", UserIDs: []int64{5, 7}})
	if err != nil || out.Title != "Team" || !out.Created {
		t.Fatal(out, err)
	}
	for _, m := range methods(*calls) {
		if strings.HasPrefix(m, "actor/by-ref/get") || strings.Contains(m, `"ref"`) {
			t.Error("a group chat is never reused or keyed:", m)
		}
	}
	for _, bad := range []ChatOpenInput{
		{Type: "p2p", UserIDs: []int64{5}},
		{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 5},
		{Type: "p2p", UserIDs: []int64{5, 7}, SelfUserID: 12},
		{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12, Title: "x"},
		{Type: "group", UserIDs: []int64{5}},
		{Type: "group", Title: "T", UserIDs: []int64{5}, SelfUserID: 12},
		{Type: "dm", UserIDs: []int64{5}},
		{Type: "group", Title: "T", UserIDs: []int64{0}},
	} {
		if _, err := c.OpenChat(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
