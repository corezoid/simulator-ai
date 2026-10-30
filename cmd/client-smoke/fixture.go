package main

import (
	"encoding/json"
	"fmt"
	"github.com/corezoid/simulator-ai/contracts"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

type fixture struct {
	server           *httptest.Server
	reads            atomic.Int32
	creates          atomic.Int32
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
		if r.Method == "GET" && r.URL.Path == "/api/2.0/doc/public" {
			_, _ = w.Write(contracts.SmartForms)
			return
		}
		if r.URL.Path != "/api/2.0/rpc" || r.Method != "POST" || r.Header.Get("Authorization") != "ApiKey synthetic-client-key" {
			f.unexpected.Add(1)
			http.Error(w, "unexpected request", 400)
			return
		}
		var request struct {
			ID     string `json:"id"`
			Method string `json:"method"`
			Params struct {
				AccID       string   `json:"accId"`
				OperationID string   `json:"operationId"`
				Ref         string   `json:"ref"`
				Title       string   `json:"title"`
				CduVersion  int      `json:"cduVersion"`
				ActorID     string   `json:"actorId"`
				Fields      []string `json:"fields"`
			} `json:"params"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil || request.Params.AccID != "synthetic-workspace" {
			f.unexpected.Add(1)
			http.Error(w, "unexpected request", 400)
			return
		}
		if request.Method == "application/create" && request.Params.OperationID == "smoke-create-1" && request.Params.Ref == "smoke-app" && request.Params.Title == "Smoke app" && request.Params.CduVersion == 2 {
			f.creates.Add(1)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"application":{"id":"synthetic-app","ref":"smoke-app"},"envs":[{"id":1,"actorId":"synthetic-app","title":"develop","rootFolderId":10,"corezoidCredentials":{"apiSecret":"credential-canary"}},{"id":2,"actorId":"synthetic-app","title":"production","rootFolderId":20}]}}`, request.ID)
			return
		}
		if request.Method != "actor/get" || request.Params.ActorID != "synthetic-actor" || len(request.Params.Fields) != 6 {
			f.unexpected.Add(1)
			http.Error(w, "unexpected request", 400)
			return
		}
		f.reads.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"id":"synthetic-actor","title":"Client smoke fixture"}}`, request.ID)
	}))
	return f
}
