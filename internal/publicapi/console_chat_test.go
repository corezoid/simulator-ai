package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func chatBackend(t *testing.T, chats string) (*Client, *[]rpcCall) {
	return rpcBackend(t, func(method string) string {
		switch method {
		case "form/system/get":
			return `{"id":9,"title":"Events","type":"system"}`
		case "actor/filter/list":
			return chats
		case "actor/create":
			return `{"id":"chat-1","userId":12}`
		case "batch":
			return `{"list":[{"id":"0","result":{"id":5,"nick":"Olena"}},{"id":"1","result":{"id":12,"nick":"Ivan"}}]}`
		case "access_rule/actor/share":
			return `{"list":[]}`
		}
		return `{"id":"chat-1"}`
	})
}

func TestOpenChatReusesTheP2PChat(t *testing.T) {
	c, calls := chatBackend(t, `{"list":[{"id":"other","ref":"p2pConversation_5_77"},{"id":"chat-0","title":"Olena:p2p:Ivan","ref":"p2pConversation_5_12"}]}`)
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{12}})
	if err != nil || out.ChatID != "chat-0" || out.Created {
		t.Fatal(out, err)
	}
	filter, _ := json.Marshal((*calls)[1].Params)
	if len(*calls) != 2 || !strings.Contains(string(filter), `"members":[{"access":[],"userId":12}]`) || !strings.Contains(string(filter), `"stream":"p2p"`) {
		t.Fatal(methods(*calls))
	}
}

func TestOpenChatCreatesP2PAsTheWebUI(t *testing.T) {
	c, calls := chatBackend(t, `{"list":[]}`)
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}})
	if err != nil || out.ChatID != "chat-1" || !out.Created || out.Title != "Olena:p2p:Ivan" {
		t.Fatal(out, err)
	}
	got := strings.Join(methods(*calls), "\n")
	for _, want := range []string{
		`"data":{"chatType":"p2p"`,
		`actor/update {"accId":"ws","actorId":"chat-1","fields":["id"],"ref":"p2pConversation_5_12","title":"Olena:p2p:Ivan"}`,
		`access_rule/actor/share {"accId":"ws","actions":[{"action":"create","privs":{"modify":true,"remove":true,"view":true},"userId":5}],"actorId":"chat-1"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
}

func TestOpenChatGroupAndBadInput(t *testing.T) {
	c, calls := chatBackend(t, `{"list":[]}`)
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "group", Title: "Team", UserIDs: []int64{5, 7}})
	if err != nil || out.Title != "Team" || !out.Created {
		t.Fatal(out, err)
	}
	for _, m := range methods(*calls) {
		if strings.HasPrefix(m, "actor/filter/list") || strings.HasPrefix(m, "actor/update") {
			t.Error("a group chat is never reused or renamed:", m)
		}
	}
	for _, bad := range []ChatOpenInput{{Type: "p2p", UserIDs: []int64{5, 7}}, {Type: "p2p", UserIDs: []int64{5}, Title: "x"}, {Type: "group", UserIDs: []int64{5}}, {Type: "dm", UserIDs: []int64{5}}, {Type: "p2p", UserIDs: []int64{0}}} {
		if _, err := c.OpenChat(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestAttachToActor(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "edge_type/get":
			return `{"id":3,"name":"hierarchy"}`
		case "batch":
			return batchOf(`{"id":"0","result":{"id":"f1"}}`, `{"id":"1","result":{"id":"e1"}}`)
		}
		return `{"id":"x"}`
	})
	ctx := context.Background()
	if _, err := c.AttachToActor(ctx, ActorAttachInput{ActorID: "a1", Kind: "smartForm", TargetID: "app1"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := json.Marshal((*calls)[0].Params); (*calls)[0].Method != "actor/update" || !strings.Contains(string(p), `"appId":"app1"`) {
		t.Fatal(string(p))
	}
	out, err := c.AttachToActor(ctx, ActorAttachInput{ActorID: "a1", Kind: "form", FormID: 7, Title: "Order", Data: json.RawMessage(`{"qty":2}`)})
	if err != nil || out.TargetID != "f1" || out.EdgeID != "e1" {
		t.Fatal(out, err)
	}
	last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	if !strings.Contains(string(last), `"edgeTypeId":3,"forceDirection":true,"pinned":true,"source":"a1","target":"$0.id"},"useRefs":true`) {
		t.Fatal(string(last))
	}
	for _, bad := range []ActorAttachInput{{ActorID: "a1", Kind: "actor"}, {ActorID: "a1", Kind: "actor", TargetID: "a1"}, {ActorID: "a1", Kind: "form"}, {ActorID: "a1", Kind: "form", TargetID: "f", FormID: 7}, {ActorID: "a1", Kind: "smartForm", TargetID: "x", Title: "t"}, {ActorID: "a1", Kind: "reaction", TargetID: "r"}} {
		if _, err := c.AttachToActor(ctx, bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
