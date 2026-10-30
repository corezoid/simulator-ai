package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/corezoid/simulator-ai/internal/version"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const maxResponseBytes = 4 << 20

var actorFields = []string{"id", "title", "description", "formId", "status", "ref"}

// Client exposes only the three supported reads. No public arbitrary RPC proxy exists.
type Client struct {
	config Config
	http   *http.Client
	nextID atomic.Uint64
}

func New(c Config) (*Client, error) {
	if e := c.validate(); e != nil {
		return nil, e
	}
	return &Client{config: c, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// APIError preserves codes/reasons, never an untrusted response body or credential.
type APIError struct {
	Code       int
	Reason     string
	HTTPStatus int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("public API request failed (code=%d reason=%s http=%d)", e.Code, e.Reason, e.HTTPStatus)
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Reason string `json:"reason"`
	} `json:"data"`
}
type envelope struct {
	Version string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}
type request[P any] struct {
	Version string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  P      `json:"params"`
}

func read[P, R any](ctx context.Context, c *Client, method string, params P) (R, error) {
	var result R
	switch method {
	case "actor/get", "form/get", "graph/layer/actors/list":
	default:
		return result, errors.New("unsupported public method")
	}
	// Source: sim-api rpc_handler.Request.ID is a string. Preserve its exact wire type.
	id := strconv.FormatUint(c.nextID.Add(1), 10)
	b, e := json.Marshal(request[P]{"2.0", id, method, params})
	if e != nil {
		return result, errors.New("cannot encode public request")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.config.Origin, "/")+"/api/2.0/rpc", bytes.NewReader(b))
	if e != nil {
		return result, errors.New("invalid request")
	}
	req.Header.Set("Authorization", "ApiKey "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Platform", "simulator-ai")
	req.Header.Set("X-App-Version", version.Current)
	resp, e := c.http.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New("public API transport unavailable; read may be retried")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, &APIError{HTTPStatus: resp.StatusCode, Reason: "http_error"}
	}
	b, e = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if e != nil {
		return result, errors.New("cannot read API response")
	}
	if len(b) > maxResponseBytes {
		return result, errors.New("public API response exceeds 4 MiB limit")
	}
	var env envelope
	if e = json.Unmarshal(b, &env); e != nil || env.Version != "2.0" || env.ID != id {
		return result, errors.New("invalid JSON-RPC response")
	}
	if env.Error != nil {
		if len(env.Result) > 0 {
			return result, errors.New("ambiguous JSON-RPC response")
		}
		reason := "remote_error"
		switch env.Error.Data.Reason {
		case "notAuthorized", "accessDenied", "notFound", "noFoundOrNoAccess", "notAllowed", "notAvailable", "invalidParams":
			reason = env.Error.Data.Reason
		}
		return result, &APIError{Code: env.Error.Code, Reason: reason}
	}
	if len(env.Result) == 0 || bytes.Equal(bytes.TrimSpace(env.Result), []byte("null")) {
		return result, errors.New("missing public API result")
	}
	if e = json.Unmarshal(env.Result, &result); e != nil {
		return result, errors.New("public API result shape mismatch")
	}
	return result, nil
}
func (c *Client) GetActor(ctx context.Context, id string) (Actor, error) {
	if strings.TrimSpace(id) == "" || len(id) > 255 {
		return Actor{}, errors.New("actorId required")
	}
	a, e := read[ActorRequest, Actor](ctx, c, "actor/get", ActorRequest{AccID: c.config.Workspace, ActorID: id, Fields: actorFields})
	if e == nil && (a.ID == nil || *a.ID != id) {
		return Actor{}, errors.New("actor result identity mismatch")
	}
	return a, e
}
func (c *Client) GetForm(ctx context.Context, id int64) (Form, error) {
	if id < 1 {
		return Form{}, errors.New("positive formId required")
	}
	f, e := read[FormRequest, Form](ctx, c, "form/get", FormRequest{AccID: c.config.Workspace, FormID: id})
	if e == nil && (f.ID == nil || *f.ID != id) {
		return Form{}, errors.New("form result identity mismatch")
	}
	return f, e
}
func (c *Client) ListLayerActors(ctx context.Context, id string, limit, offset int64) (LayerResult, error) {
	if strings.TrimSpace(id) == "" || len(id) > 255 || limit < 1 || limit > 50 || offset < 0 {
		return LayerResult{}, errors.New("layerId required, limit 1..50 and offset >=0")
	}
	page, e := read[LayerRequest, LayerResult](ctx, c, "graph/layer/actors/list", LayerRequest{AccID: c.config.Workspace, ActorID: id, Limit: limit, Offset: offset, Fields: actorFields})
	if e != nil {
		return page, e
	}
	if page.List == nil || int64(len(page.List)) > limit {
		return LayerResult{}, errors.New("invalid layer page")
	}
	for i := range page.List {
		n := &page.List[i]
		if n.LaID == nil || *n.LaID < 1 || n.ID == nil || strings.TrimSpace(*n.ID) == "" {
			return LayerResult{}, errors.New("invalid placement identity")
		}
		if n.AccessDenied != nil && *n.AccessDenied {
			n.Actor = nil
		}
		if n.Actor != nil && (n.Actor.ID == nil || *n.Actor.ID != *n.ID) {
			return LayerResult{}, errors.New("placement actor identity mismatch")
		}
	}
	return page, nil
}
