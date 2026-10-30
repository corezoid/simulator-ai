package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

type fixture struct {
	server     *httptest.Server
	reads      atomic.Int32
	unexpected atomic.Int32
}

func startFixture() *fixture {
	f := &fixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
