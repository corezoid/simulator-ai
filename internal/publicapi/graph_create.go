package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/corezoid/simulator-ai/contracts"
)

const MaxGraphCreateInputBytes = 512 << 10

func (c *Client) GraphCreatesEnabled() bool { return c.config.Profile == "graph-create" }

// Preflight before EVERY write, without a credential; no cached promise about other replicas.
func (c *Client) verifyGraphCreates(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.config.Origin, "/")+"/api/2.0/doc/public", nil)
	if err != nil {
		return errors.New("invalid public contract request")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("cannot verify graph creation contract; no write sent")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New("graph creation public contract unavailable; no write sent")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return errors.New("invalid or oversized public contract; no write sent")
	}
	return contracts.CheckGraphCreateCompatibility(raw)
}

func graphCreateCall[P, R any](ctx context.Context, c *Client, method string, p P, write bool) (R, error) {
	var zero R
	if !c.GraphCreatesEnabled() {
		return zero, errors.New("graph-create profile is disabled")
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > MaxGraphCreateInputBytes {
		return zero, errors.New("invalid or oversized graph creation input")
	}
	if err = contracts.ValidateGraphCreateInput(method, raw); err != nil {
		return zero, err
	}
	if write {
		if err = c.verifyGraphCreates(ctx); err != nil {
			return zero, err
		}
	}
	result, err := call[graphRequest[P], R](ctx, c, method, graphRequest[P]{AccID: c.config.Workspace, Input: p})
	if err != nil && write {
		return zero, &WriteError{Cause: err}
	}
	return result, err
}

func (c *Client) CreateGraphActor(ctx context.Context, p GraphActorInput) (Actor, error) {
	r, err := graphCreateCall[GraphActorInput, Actor](ctx, c, "actor/create", p, true)
	if err == nil {
		if err = validateGraphActor(r); err == nil && *r.FormID != p.FormID {
			err = errors.New("created actor template mismatch")
		}
		if err != nil {
			return Actor{}, &WriteError{Cause: err}
		}
	}
	return r, err
}
func (c *Client) GetGraphActorOperation(ctx context.Context, p Operation) (Actor, error) {
	r, err := graphCreateCall[Operation, Actor](ctx, c, "actor/operation/get", p, false)
	if err == nil {
		err = validateGraphActor(r)
	}
	return r, err
}
func (c *Client) CreateGraphEdge(ctx context.Context, p GraphEdgeInput) (GraphCreatedEdge, error) {
	r, err := graphCreateCall[GraphEdgeInput, GraphCreatedEdge](ctx, c, "edge/create", p, true)
	if err == nil {
		if err = validateGraphEdge(r); err == nil && (r.EdgeTypeID != p.EdgeTypeID || !sameEdgeEnds(p, r) || graphStringValue(r.LinkedActorID) != p.LinkedActorID) {
			err = errors.New("created edge identity mismatch")
		}
		if err != nil {
			return GraphCreatedEdge{}, &WriteError{Cause: err}
		}
	}
	return r, err
}
func (c *Client) GetGraphEdgeOperation(ctx context.Context, p Operation) (GraphCreatedEdge, error) {
	r, err := graphCreateCall[Operation, GraphCreatedEdge](ctx, c, "edge/operation/get", p, false)
	if err == nil {
		err = validateGraphEdge(r)
	}
	return r, err
}
func (c *Client) CreateGraphPlacements(ctx context.Context, p GraphPlacementsInput) (GraphCreatedPlacements, error) {
	r, err := graphCreateCall[GraphPlacementsInput, GraphCreatedPlacements](ctx, c, "graph/layer/actors/manage", p, true)
	if err == nil {
		if err = validateGraphPlacements(r); err == nil {
			err = bindGraphPlacements(p, r)
		}
		if err != nil {
			return GraphCreatedPlacements{}, &WriteError{Cause: err}
		}
	}
	return r, err
}
func (c *Client) GetGraphPlacementOperation(ctx context.Context, p GraphPlacementOperationInput) (GraphCreatedPlacements, error) {
	r, err := graphCreateCall[GraphPlacementOperationInput, GraphCreatedPlacements](ctx, c, "graph/layer/operation/get", p, false)
	if err == nil {
		err = validateGraphPlacements(r)
	}
	return r, err
}
