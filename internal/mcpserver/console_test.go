package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

func TestConsoleProfile(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.Method != "actor/create" || r.Header.Get("Authorization") != "Simulator user-jwt" {
			t.Errorf("unexpected %s with %q", q.Method, r.Header.Get("Authorization"))
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":"a1","title":"Demo","formId":42}}`, q.ID)
	}))
	defer api.Close()
	c, e := publicapi.New(publicapi.Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator user-jwt"})
	if e != nil {
		t.Fatal(e)
	}
	s := New(c)
	send := func(body string) string {
		reply, e := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(body)))
		if e != nil {
			t.Fatal(e)
		}
		return string(reply)
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	var list struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if e := json.Unmarshal([]byte(send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)), &list); e != nil {
		t.Fatal(e)
	}
	var names []string
	for _, tool := range list.Result.Tools {
		names = append(names, tool.Name)
	}
	if strings.Join(names, ",") != "actor_create,actor_get" {
		t.Fatal("console tools:", names)
	}
	reply := send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"actor_create","arguments":{"formId":42,"title":"Demo","data":{}}}}`)
	if !strings.Contains(reply, `\"id\":\"a1\"`) && !strings.Contains(reply, `"id":"a1"`) || strings.Contains(reply, `"isError":true`) {
		t.Fatal(reply)
	}
}
