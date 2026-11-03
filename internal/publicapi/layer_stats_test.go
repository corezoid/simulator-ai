package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const layerCounterFixture = `{"nodes":7,"edges":8,"holes":1,"holesClosed":2,"edgeHoles":3,"edgeHolesClosed":4}`

func TestLayerStatsCounterProjection(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		bad          bool
	}{
		{"six distinct counters", layerCounterFixture, false},
		{"zero", `{"nodes":0,"edges":0,"holes":0,"holesClosed":0,"edgeHoles":0,"edgeHolesClosed":0}`, false},
		{"capability excluded", strings.TrimSuffix(layerCounterFixture, "}") + `,"credential":"canary"}`, false},
		{"missing counters", `{}`, true},
		{"null result", `null`, true},
		{"array", `[]`, true},
		{"null counter", strings.Replace(layerCounterFixture, `"nodes":7`, `"nodes":null`, 1), true},
		{"missing counter", strings.Replace(layerCounterFixture, `"nodes":7,`, "", 1), true},
		{"negative", strings.Replace(layerCounterFixture, `"nodes":7`, `"nodes":-1`, 1), true},
		{"fraction", strings.Replace(layerCounterFixture, `"nodes":7`, `"nodes":1.5`, 1), true},
		{"string", strings.Replace(layerCounterFixture, `"nodes":7`, `"nodes":"7"`, 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q request[layerStatsRequest]
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
				}
				if q.Method != "graph/layer/stats/get" || q.Params.AccID != "workspace" || q.Params.ActorID != "layer" || r.Header.Get("Authorization") != "ApiKey fixture" {
					t.Error("public host boundary escaped")
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, tc.result)
			}))
			defer backend.Close()
			client, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "graph-read"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetLayerStats(context.Background(), "layer")
			if (err != nil) != tc.bad {
				t.Fatalf("unexpected result/error: %+v %v", got, err)
			}
			if tc.bad {
				if got != (LayerStats{}) {
					t.Fatal("partial counters leaked")
				}
				return
			}
			var want LayerStats
			if err = json.Unmarshal([]byte(tc.result), &want); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("counter projection changed: %+v", got)
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), "canary") {
				t.Fatal("unexpected field leaked")
			}
		})
	}
}

func TestLayerStatsBindingFreshAuthorization(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q request[layerStatsRequest]
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if q.Params.ActorID != "host-layer" || q.Params.AccID != "workspace" {
			t.Error("binding escaped")
		}
		if calls == 1 {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, q.ID, layerCounterFixture)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":1,"message":"secret-canary","data":{"reason":"accessDenied"}}}`, q.ID)
	}))
	defer backend.Close()
	c, err := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: "graph-read", ActorID: "host-layer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.GetLayerStats(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"host-layer", "other"} {
		if _, err = c.GetLayerStats(context.Background(), id); err == nil {
			t.Fatal("retargeting accepted")
		}
	}
	got, err := c.GetLayerStats(context.Background(), "")
	if err == nil || got != (LayerStats{}) || strings.Contains(err.Error(), "canary") {
		t.Fatal("revoked read cached or error leaked")
	}
	if calls != 2 {
		t.Fatal("invalid call reached backend or read was retried")
	}
	for _, profile := range []string{"read", "reactions-read", "smart-forms"} {
		other, e := New(Config{Origin: backend.URL, Workspace: "workspace", APIKey: "fixture", Profile: profile})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = other.GetLayerStats(context.Background(), "layer"); e == nil {
			t.Fatal("stats escaped profile")
		}
	}
	if calls != 2 {
		t.Fatal("denied profile reached backend")
	}
}
