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
	if id, trusted, err := c.selfUserID(ctx, 0); err != nil || !trusted || id != 12 {
		t.Fatal(id, trusted, err)
	}
	if _, _, err := c.selfUserID(ctx, 7); err == nil || !strings.Contains(err.Error(), "you are user 12") {
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
