package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizationConfig(t *testing.T) {
	base := func(auth string) Config {
		return Config{Origin: "http://127.0.0.1:9100", Workspace: "ws", Profile: "console", Authorization: auth}
	}
	for _, auth := range []string{"Bearer abc", "Simulator a.b.c", "ApiKey k1"} {
		c := &Client{config: base(auth)}
		if got, e := c.authorization(context.Background()); base(auth).validate() != nil || e != nil || got != auth {
			t.Fatal(auth, e)
		}
	}
	for _, auth := range []string{"Basic abc", "Bearer", "Bearer a b", "bearer abc", "Bearer "} {
		if base(auth).validate() == nil {
			t.Fatal("accepted", auth)
		}
	}
	both := base("Bearer abc")
	both.APIKey = "k"
	if both.validate() == nil {
		t.Fatal("accepted both an authorization and an API key")
	}
	delegated := base("Bearer abc")
	delegated.Profile = "delegated-read"
	if delegated.validate() == nil {
		t.Fatal("accepted an authorization for delegated-read")
	}
	if e := (Config{Origin: "http://127.0.0.1:9100", Workspace: "ws", Profile: "console", APIKey: "k"}).validate(); e != nil {
		t.Fatal("API-key console rejected:", e)
	}
}

func TestCreateActor(t *testing.T) {
	var got struct {
		Method string `json:"method"`
		ID     string `json:"id"`
		Params struct {
			AccID  string          `json:"accId"`
			FormID int64           `json:"formId"`
			Title  string          `json:"title"`
			Data   json.RawMessage `json:"data"`
			Fields []string        `json:"fields"`
		} `json:"params"`
	}
	var header string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"` + got.ID + `","result":{"id":"a1","title":"Demo","formId":42}}`))
	}))
	t.Cleanup(s.Close)
	c, e := New(Config{Origin: s.URL, Workspace: "ws", Profile: "console", Authorization: "Bearer user-token"})
	if e != nil {
		t.Fatal(e)
	}
	a, e := c.CreateActor(context.Background(), ActorCreateInput{FormID: 42, Title: "Demo"})
	if e != nil || a.ID == nil || *a.ID != "a1" {
		t.Fatal(a, e)
	}
	if header != "Bearer user-token" || got.Method != "actor/create" || got.Params.AccID != "ws" || got.Params.FormID != 42 ||
		got.Params.Title != "Demo" || string(got.Params.Data) != "{}" || strings.Join(got.Params.Fields, ",") != strings.Join(actorFields, ",") {
		t.Fatalf("request %+v header %q", got, header)
	}
	for _, bad := range []ActorCreateInput{{Title: "x"}, {FormID: 1}, {FormID: 1, Title: "x", Data: json.RawMessage(`[1]`)}} {
		if _, e := c.CreateActor(context.Background(), bad); e == nil {
			t.Fatal("accepted", bad)
		}
	}
	c.config.Profile = "read"
	if _, e := c.CreateActor(context.Background(), ActorCreateInput{FormID: 1, Title: "x"}); e == nil {
		t.Fatal("created outside the console profile")
	}
}
