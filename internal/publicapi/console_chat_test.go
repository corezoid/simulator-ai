package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// batchReply answers the last batch by its items: user/get with the nick of 5 or 12, actor/create
// as chat-1 made by user 12, a share with no rules; atomic when it was asked to be.
func batchReply(calls *[]rpcCall) string {
	raw, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	var req struct {
		IsAtomic bool `json:"isAtomic"`
		List     []struct {
			Method string `json:"method"`
			Params struct {
				UserID int64 `json:"userId"`
			} `json:"params"`
		} `json:"list"`
	}
	_ = json.Unmarshal(raw, &req)
	items := make([]string, len(req.List))
	for i, item := range req.List {
		result := `{"list":[]}`
		switch item.Method {
		case "user/get":
			if nick, ok := map[int64]string{5: "Olena", 12: "Ivan"}[item.Params.UserID]; ok {
				result = fmt.Sprintf(`{"id":%d,"nick":%q}`, item.Params.UserID, nick)
			} else {
				result = `{}`
			}
		case "actor/create":
			result = `{"id":"chat-1","userId":12}`
		}
		items[i] = fmt.Sprintf(`{"id":"%d","result":%s}`, i, result)
	}
	return fmt.Sprintf(`{"isAtomic":%v,"list":[%s]}`, req.IsAtomic, strings.Join(items, ","))
}

// chatBackend answers by-ref reads with byRef (notFound when empty) and creates chat-1 as user 12.
func chatBackend(t *testing.T, byRef string) (*Client, *[]rpcCall) {
	var calls *[]rpcCall
	c, calls := rpcBackend(t, func(method string) string {
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
		case "batch":
			return batchReply(calls)
		}
		return `{}`
	})
	return c, calls
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

func TestOpenChatCreatesThenSharesOnceChecked(t *testing.T) {
	c, calls := chatBackend(t, "")
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12})
	if err != nil || out.ChatID != "chat-1" || !out.Created || !out.Notified || out.Title != "Olena:p2p:Ivan" {
		t.Fatal(out, err)
	}
	// The id came from the model, not the credential: the batch only creates the chat, and the other
	// side gets its rule, notified, in one share after the chat's creator proved to be that id.
	got := methods(*calls)
	create, share := got[len(got)-2], got[len(got)-1]
	if !strings.HasPrefix(create, "batch") || !strings.Contains(create, `"isAtomic":true`) || !strings.Contains(create, `"ref":"p2pConversation_5_12","title":"Olena:p2p:Ivan"}`) || strings.Contains(create, "access_rule") {
		t.Fatal("the create batch:", create)
	}
	if share != `access_rule/actor/share {"accId":"ws","actions":[{"action":"create","privs":{"modify":true,"remove":true,"view":true},"userId":5}],"actorId":"chat-1","isNotify":true}` {
		t.Fatal("the share after the check:", share)
	}
	for _, m := range got[:len(got)-1] {
		if strings.Contains(m, "access_rule/actor/share") || strings.HasPrefix(m, "actor/update") {
			t.Error("shared or renamed before the check:", m)
		}
	}
}

// A share that fails after the check takes the chat back: no chat the other side cannot open.
func TestOpenChatShareFailureRemovesTheChat(t *testing.T) {
	var calls *[]rpcCall
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "form/system/get":
			return `{"id":9,"title":"Events","type":"system"}`
		case "actor/by-ref/get":
			return notFound
		case "batch":
			return batchReply(calls)
		case "access_rule/actor/share":
			return `"error":{"code":1,"message":"user 5 is not a member","data":{"reason":"invalidParams"}}`
		}
		return `{}`
	})
	if _, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12}); err == nil || !strings.Contains(err.Error(), "removed again") {
		t.Fatal(err)
	}
	if got := methods(*calls); got[len(got)-1] != `actor/delete {"accId":"ws","actorId":"chat-1"}` {
		t.Fatal(got)
	}
}

// A selfUserId that is not the caller is caught by the chat's creator: the quiet chat is deleted
// again and nobody is notified.
func TestOpenChatRefusesAForeignSelf(t *testing.T) {
	c, calls := chatBackend(t, "")
	if _, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 77}); err == nil || !strings.Contains(err.Error(), "selfUserId 77 is not you") {
		t.Fatal(err)
	}
	got := methods(*calls)
	if last := got[len(got)-1]; last != `actor/delete {"accId":"ws","actorId":"chat-1"}` {
		t.Fatal("the wrong chat was not taken back:", got)
	}
	for _, m := range got {
		if strings.Contains(m, `"isNotify":true`) {
			t.Error("a wrong selfUserId notified someone:", m)
		}
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
