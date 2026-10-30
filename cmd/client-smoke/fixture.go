package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

type fixture struct {
	server           *httptest.Server
	reads            atomic.Int32
	unexpected       atomic.Int32
	tokenCountProbes atomic.Int32
}

func startFixture() *fixture {
	f := &fixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Claude plugin details estimates inventory tokens. Reject its non-generative
		// count_tokens probes locally; never send them or the inventory to a provider.
		if r.Method == "POST" && r.URL.Path == "/disabled-model/v1/messages/count_tokens" {
			f.tokenCountProbes.Add(1)
			_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, 1<<20))
			http.Error(w, "token counting unavailable in scripted smoke", http.StatusNotImplemented)
			return
		}
		if r.URL.Path != "/api/2.0/rpc" || r.Method != "POST" || r.Header.Get("Authorization") != "ApiKey synthetic-client-key" {
			f.unexpected.Add(1)
			http.Error(w, "unexpected request", 400)
			return
		}
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
			Params struct {
				AccID   string   `json:"accId"`
				ActorID string   `json:"actorId"`
				Fields  []string `json:"fields"`
			} `json:"params"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil || request.Method != "actor/get" || request.Params.AccID != "synthetic-workspace" || request.Params.ActorID != "synthetic-actor" || len(request.Params.Fields) != 6 {
			f.unexpected.Add(1)
			http.Error(w, "unexpected request", 400)
			return
		}
		f.reads.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"id":"synthetic-actor","title":"Client smoke fixture"}}`, request.ID)
	}))
	return f
}
