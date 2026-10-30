package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type received struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Params struct {
		AccID   string   `json:"accId"`
		ActorID string   `json:"actorId"`
		FormID  int64    `json:"formId"`
		Fields  []string `json:"fields"`
		With    []string `json:"with"`
		Limit   int64    `json:"limit"`
		Offset  int64    `json:"offset"`
	} `json:"params"`
}

func testClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, e := New(Config{s.URL, "workspace-a", "synthetic-test-key"})
	if e != nil {
		t.Fatal(e)
	}
	return c, s
}
func TestTypedReadsAndProjection(t *testing.T) {
	calls := 0
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/2.0/rpc" || r.URL.RawQuery != "" {
			t.Error("wrong public endpoint")
		}
		if r.Header.Get("Authorization") != "ApiKey synthetic-test-key" {
			t.Error("wrong auth")
		}
		var q received
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		if q.Params.AccID != "workspace-a" || len(q.Params.With) != 0 {
			t.Error("workspace or related expansion drift")
		}
		var result string
		switch q.Method {
		case "actor/get":
			if len(q.Params.Fields) != 6 {
				t.Error("missing projection")
			}
			result = `{"id":"actor-a","title":"A","formId":7,"data":{"secret":"hidden"},"editToken":"secret-editor"}`
		case "form/get":
			result = `{"id":7,"title":"F","parentId":null,"form":{"private":"not-in-profile"},"editToken":"secret-editor"}`
		case "graph/layer/actors/list":
			if q.Params.Limit != 2 || q.Params.Offset != 4 || len(q.Params.Fields) != 6 {
				t.Error("wrong page")
			}
			result = `{"list":[{"laId":1,"id":"actor-a","actor":{"id":"actor-a","title":"A"}},{"laId":2,"id":"actor-private","accessDenied":true,"actor":{"description":"must-strip"}}]}`
		default:
			t.Errorf("unexpected method %s", q.Method)
			result = `{}`
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, q.ID, result)
	})
	a, e := c.GetActor(context.Background(), "actor-a")
	if e != nil {
		t.Fatal(e)
	}
	if a.Title == nil || *a.Title != "A" {
		t.Fatal("actor metadata missing")
	}
	f, e := c.GetForm(context.Background(), 7)
	if e != nil {
		t.Fatal(e)
	}
	if f.ParentID != nil {
		t.Fatal("null parent changed")
	}
	p, e := c.ListLayerActors(context.Background(), "layer-a", 2, 4)
	if e != nil {
		t.Fatal(e)
	}
	if p.List[1].Actor != nil {
		t.Fatal("denied actor leaked")
	}
	for _, v := range []any{a, f, p} {
		b, _ := json.Marshal(v)
		for _, secret := range []string{"hidden", "secret-editor", "not-in-profile", "must-strip"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("unrequested result leaked %s", secret)
			}
		}
	}
	if calls != 3 {
		t.Fatalf("expected 3 reads, got %d", calls)
	}
}
func TestNoRedirectCredentialForwarding(t *testing.T) {
	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++ }))
	defer target.Close()
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	_, e := c.GetActor(context.Background(), "a")
	if e == nil || forwarded != 0 {
		t.Fatalf("redirect followed or accepted: %v calls %d", e, forwarded)
	}
}
func TestMalformedErrorsAndBounds(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"wrong-id", `{"jsonrpc":"2.0","id":99,"result":{"id":"a"}}`, 200},
		{"wrong-version", `{"jsonrpc":"1.0","id":1,"result":{"id":"a"}}`, 200},
		{"missing-result", `{"jsonrpc":"2.0","id":1}`, 200},
		{"null-result", `{"jsonrpc":"2.0","id":1,"result":null}`, 200},
		{"identity-mismatch", `{"jsonrpc":"2.0","id":1,"result":{"id":"different"}}`, 200},
		{"business", `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"synthetic-test-key","data":{"reason":"accessDenied"}}}`, 200},
		{"unsafe-reason", `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"synthetic-test-key","data":{"reason":"synthetic-test-key"}}}`, 200},
		{"ambiguous", `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":1}}`, 200},
		{"bad-json", `synthetic-test-key`, 200},
		{"http-body", `synthetic-test-key`, 401},
		{"oversize", strings.Repeat("x", maxResponseBytes+1), 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			_, e := c.GetActor(context.Background(), "a")
			if e == nil {
				t.Fatal("invalid response accepted")
			}
			if strings.Contains(e.Error(), "synthetic-test-key") {
				t.Fatal("secret echoed in error")
			}
			if calls != 1 {
				t.Fatal("unexpected retry")
			}
			if tc.name == "business" {
				var apiErr *APIError
				if !errors.As(e, &apiErr) || apiErr.Code != 1 || apiErr.Reason != "accessDenied" {
					t.Fatal("typed business error lost")
				}
			}
		})
	}
}
func TestInvalidInputsAndNoArbitraryRPC(t *testing.T) {
	calls := 0
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	if _, e := c.GetActor(context.Background(), ""); e == nil {
		t.Fatal("empty actor")
	}
	if _, e := c.GetForm(context.Background(), 0); e == nil {
		t.Fatal("zero form")
	}
	for _, limit := range []int64{0, 51} {
		if _, e := c.ListLayerActors(context.Background(), "a", limit, 0); e == nil {
			t.Fatal("invalid limit")
		}
	}
	if _, e := c.ListLayerActors(context.Background(), "a", 1, -1); e == nil {
		t.Fatal("negative offset")
	}
	if _, e := read[ActorRequest, Actor](context.Background(), c, "actor/update", ActorRequest{}); e == nil {
		t.Fatal("arbitrary write allowed")
	}
	if calls != 0 {
		t.Fatal("invalid input reached API")
	}
}
func TestCancellation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, e := c.GetActor(ctx, "a")
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("cancellation lost: %v", e)
	}
}
func TestConfigRejectsUnsafeOrAmbiguousOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://", "http://example.com", "https://u:p@example.com", "https://example.com/path", "https://example.com?q=x", "https://example.com?", "https://example.com#x", "file:///tmp/foo"} {
		if _, e := New(Config{origin, "ws", "key"}); e == nil {
			t.Fatalf("accepted %s", origin)
		}
	}
	for _, origin := range []string{"https://example.com", "http://127.0.0.1:9100", "http://[::1]:9100"} {
		if _, e := New(Config{origin, "ws", "key"}); e != nil {
			t.Fatalf("rejected %s: %v", origin, e)
		}
	}
	for _, key := range []string{"", "a\r\nX: bad", "Bearer key"} {
		if _, e := New(Config{"https://example.com", "ws", key}); e == nil {
			t.Fatal("invalid key")
		}
	}
	if _, e := LoadConfig(func(string) string { return "" }); e == nil {
		t.Fatal("missing env accepted")
	}
}

// Deleted actor filtering happens after LIMIT/OFFSET in sim-api. An empty page
// must not prevent a caller from reading a later offset with live placements.
func TestEmptyLayerPageDoesNotEndLaterReads(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q received
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		result := `{"list":[]}`
		if q.Params.Offset == 2 {
			result = `{"list":[{"laId":3,"id":"live-actor","actor":{"id":"live-actor"}}]}`
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, q.ID, result)
	})
	first, e := c.ListLayerActors(context.Background(), "layer-a", 2, 0)
	if e != nil || len(first.List) != 0 {
		t.Fatalf("first page: %+v %v", first, e)
	}
	later, e := c.ListLayerActors(context.Background(), "layer-a", 2, 2)
	if e != nil || len(later.List) != 1 || (later.List[0].ID == nil || *later.List[0].ID != "live-actor") {
		t.Fatalf("later page: %+v %v", later, e)
	}
}

func TestInvalidPlacementResults(t *testing.T) {
	for _, body := range []string{
		`{"list":[null]}`, `{"list":[{}]}`,
		`{"list":[{"laId":0,"id":"a"}]}`,
		`{"list":[{"laId":1,"id":""}]}`,
		`{"list":[{"laId":1,"id":"a","actor":{}}]}`,
		`{"list":[{"laId":1,"id":"a","actor":{"id":"b"}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, body)
			})
			if _, e := c.ListLayerActors(context.Background(), "layer-a", 2, 0); e == nil {
				t.Fatal("malformed placement accepted")
			}
		})
	}
}
