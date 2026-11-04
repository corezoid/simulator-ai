package publicapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jwtBackend is rpcBackend with a Simulator JWT credential naming SuperAdmin user sub.
func jwtBackend(t *testing.T, claims string, result func(method string) string) (*Client, *[]rpcCall) {
	t.Helper()
	var calls []rpcCall
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID string `json:"id"`
			rpcCall
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		calls = append(calls, q.rpcCall)
		if r := result(q.Method); strings.HasPrefix(r, `"error":`) {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,%s}`, q.ID, r)
		} else {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, r)
		}
	}))
	t.Cleanup(api.Close)
	token := "eyJhbGciOiJIUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".sig"
	c, err := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator " + token})
	if err != nil {
		t.Fatal(err)
	}
	return c, &calls
}

func members(method string) string {
	switch method {
	case "user/list":
		return `{"list":[{"id":7,"saId":4500,"nick":"Ivan"},{"id":12,"saId":45,"nick":"Ivan"}]}`
	case "form/system/get":
		return `{"id":9,"title":"Events","type":"system"}`
	case "actor/by-ref/get":
		return `{"id":"chat-0","title":"Olena:p2p:Ivan"}`
	}
	return `{}`
}

// The run's own token names the caller: its member is found by saId, and a selfUserId that differs
// is refused; the reused chat needs no participant check.
func TestSelfFromTheRunCredential(t *testing.T) {
	c, calls := jwtBackend(t, `{"sub":"45","nick":"Ivan","id":45}`, members)
	ctx := context.Background()
	if who, err := c.selfUserID(ctx, 0); err != nil || who != (caller{ID: 12, Trusted: true, SAID: 45}) {
		t.Fatal(who, err)
	}
	if _, err := c.selfUserID(ctx, 7); err == nil || !strings.Contains(err.Error(), "you are user 12") {
		t.Fatal("a selfUserId that is not the caller was taken:", err)
	}
	*calls = nil
	out, err := c.OpenChat(ctx, ChatOpenInput{Type: "p2p", UserIDs: []int64{5}})
	if err != nil || out.ChatID != "chat-0" {
		t.Fatal(out, err)
	}
	got := strings.Join(methods(*calls), "\n")
	if !strings.Contains(got, `user/list {"accId":"ws","limit":200,"offset":0,"query":"Ivan"}`) || !strings.Contains(got, `"ref":"p2pConversation_5_12"`) || strings.Contains(got, "access_rule/actor/list") {
		t.Fatal(got)
	}
}

func TestCredentialUserReadsOnlyASimulatorJWT(t *testing.T) {
	for auth, ok := range map[string]bool{
		"Simulator x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"45"}`)) + ".y": true,
		"Bearer x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"45"}`)) + ".y":    false,
		"Simulator run": false,
		"Simulator x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"me"}`)) + ".y": false,
	} {
		c, err := New(Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: auth})
		if err != nil {
			t.Fatal(err)
		}
		if _, got := c.credentialUser(); got != ok {
			t.Errorf("%.20s…: %v, want %v", auth, got, ok)
		}
	}
}

// An ordinary member lacks viewUsers, so user/list is refused: the caller is the selfUserId the
// request names, and a reused chat needs that user's rule to carry the token's saId.
func TestSelfWithoutViewUsers(t *testing.T) {
	rules := `{"list":[{"userId":12,"saId":45},{"userId":5,"saId":900}]}`
	c, calls := jwtBackend(t, `{"sub":"45","nick":"Ivan"}`, func(method string) string {
		switch method {
		case "user/list":
			return `"error":{"code":1,"message":"access denied: workspace permissions not allowed","data":{"reason":"accessDenied"}}`
		case "access_rule/actor/list":
			return rules
		}
		return members(method)
	})
	ctx := context.Background()
	if who, err := c.selfUserID(ctx, 12); err != nil || who != (caller{ID: 12, SAID: 45}) {
		t.Fatal(who, err)
	}
	out, err := c.OpenChat(ctx, ChatOpenInput{Type: "p2p", UserIDs: []int64{5}, SelfUserID: 12})
	if err != nil || out.ChatID != "chat-0" {
		t.Fatal(out, err)
	}
	if got := strings.Join(methods(*calls), "\n"); !strings.Contains(got, `access_rule/actor/list {"accId":"ws","actorId":"chat-0"}`) {
		t.Fatal(got)
	}
	// An admin who sees the chat of 5 and 12 and claims to be 5: 5's rule is not the token's saId.
	if _, err := c.OpenChat(ctx, ChatOpenInput{Type: "p2p", UserIDs: []int64{12}, SelfUserID: 5}); err == nil || !strings.Contains(err.Error(), "not a participant") {
		t.Fatal("a stranger's chat was reused:", err)
	}
	if _, err := c.OpenChat(ctx, ChatOpenInput{Type: "p2p", UserIDs: []int64{5}}); err == nil || !strings.Contains(err.Error(), "needs selfUserId") {
		t.Fatal("without viewUsers selfUserId is required:", err)
	}
}

// With the caller known from the credential the batch notifies at once, with no second share.
func TestOpenChatTrustedNotifiesInTheBatch(t *testing.T) {
	c, calls := jwtBackend(t, `{"sub":"45","nick":"Ivan"}`, func(method string) string {
		switch method {
		case "actor/by-ref/get":
			return notFound
		case "batch":
			return batchOf(`{"id":"0","result":{"id":"chat-1","userId":12}}`, `{"id":"1","result":{"list":[]}}`)
		}
		return members(method)
	})
	out, err := c.OpenChat(context.Background(), ChatOpenInput{Type: "p2p", UserIDs: []int64{5}})
	if err != nil || !out.Created || !out.Notified || out.Title != "5:p2p:Ivan" {
		t.Fatal(out, err)
	}
	got := methods(*calls)
	if last := got[len(got)-1]; !strings.HasPrefix(last, "batch") || !strings.Contains(last, `"isNotify":true},"useRefs":true`) {
		t.Fatal(got)
	}
}
