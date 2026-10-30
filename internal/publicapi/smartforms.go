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

const MaxSmartFormInputBytes = 512 << 10

func (c *Client) SmartFormsEnabled() bool   { return c.config.Profile == "smart-forms" }
func (w *Workspace) setWorkspace(id string) { w.AccID = id }

type smartRequest interface{ setWorkspace(string) }

// VerifySmartForms refuses old servers which silently ignore new request fields.
// Deployment must still ensure all replicas behind the origin carry this contract.
func (c *Client) VerifySmartForms(ctx context.Context) error {
	if !c.SmartFormsEnabled() {
		return errors.New("Smart Forms profile is disabled")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.config.Origin, "/")+"/api/2.0/doc/public", nil)
	if err != nil {
		return errors.New("invalid public contract request")
	}
	// The public contract endpoint needs no credential. Never forward a key on redirects.
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("cannot verify Smart Forms backend contract; no write sent")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("Smart Forms public contract unavailable; no write sent")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(b) > maxResponseBytes {
		return errors.New("invalid or oversized public contract; no write sent")
	}
	return contracts.CheckCompatibility(b)
}
func smartCall[P smartRequest, R any](ctx context.Context, c *Client, method string, p P, write bool) (R, error) {
	var result R
	if !c.SmartFormsEnabled() {
		return result, errors.New("Smart Forms profile is disabled")
	}
	p.setWorkspace(c.config.Workspace)
	b, err := json.Marshal(p)
	if err != nil || len(b) > MaxSmartFormInputBytes {
		return result, errors.New("invalid or oversized Smart Form input")
	}
	// The wire envelope has host-only fields which the tool schema excludes.
	var arguments map[string]json.RawMessage
	if err = json.Unmarshal(b, &arguments); err != nil {
		return result, errors.New("invalid Smart Form input")
	}
	delete(arguments, "accId")
	if method == "application/create" {
		delete(arguments, "cduVersion")
	}
	b, err = json.Marshal(arguments)
	if err != nil {
		return result, errors.New("invalid Smart Form input")
	}
	if err = contracts.ValidateInput(method, b); err != nil {
		return result, err
	}
	if write {
		if err = c.VerifySmartForms(ctx); err != nil {
			return result, err
		}
	}
	result, err = call[P, R](ctx, c, method, p)
	if err == nil {
		err = validateSmartResult(result)
	}
	if err == nil && write {
		err = validateWriteBinding(p, result)
	}
	if err != nil && write {
		return result, &WriteError{Cause: err}
	}
	return result, err
}

// WriteError deliberately preserves bounded API reasons without response bodies.
// Even HTTP errors or malformed success responses can follow a committed write.
type WriteError struct{ Cause error }

func (e *WriteError) Error() string {
	return e.Cause.Error() + "; preserve operationId and original payload; recover the receipt before any retry or new key"
}
func (e *WriteError) Unwrap() error { return e.Cause }

func (c *Client) CreateApplication(ctx context.Context, p CreateApplicationInput) (CreatedApplication, error) {
	p.CduVersion = 2
	r, e := smartCall[*CreateApplicationInput, CreatedApplication](ctx, c, "application/create", &p, true)
	if e == nil && (r.Application.ID == nil || *r.Application.ID == "" || r.Application.Ref == nil || *r.Application.Ref != p.Ref || len(r.Envs) != 2) {
		e = &WriteError{Cause: errors.New("application result identity mismatch")}
	}
	return r, e
}
func (c *Client) GetApplicationOperation(ctx context.Context, p ApplicationOperationInput) (CreatedApplication, error) {
	return smartCall[*ApplicationOperationInput, CreatedApplication](ctx, c, "application/operation/get", &p, false)
}
func (c *Client) GetEnvironments(ctx context.Context, p AppInput) (Environments, error) {
	r, e := smartCall[*AppInput, Environments](ctx, c, "application/env/list", &p, false)
	if e == nil {
		for _, env := range r.Envs {
			if env.ActorID != p.ActorID {
				return Environments{}, errors.New("environment actor identity mismatch")
			}
		}
	}
	return r, e
}
func (c *Client) GetSnapshot(ctx context.Context, p SnapshotInput) (Snapshot, error) {
	r, e := smartCall[*SnapshotInput, Snapshot](ctx, c, "application/snapshot/get", &p, false)
	if e == nil && (r.Tree.ID < 1 || r.Tree.ObjType != "folder" || len(r.EnvironmentHash) != 64) {
		return Snapshot{}, errors.New("invalid content snapshot")
	}
	return r, e
}
func (c *Client) CreateFolders(ctx context.Context, p CreateFoldersInput) (Folders, error) {
	return smartCall[*CreateFoldersInput, Folders](ctx, c, "app_folder/create", &p, true)
}
func (c *Client) GetFolderOperation(ctx context.Context, p AppOperationInput) (Folders, error) {
	return smartCall[*AppOperationInput, Folders](ctx, c, "app_folder/operation/get", &p, false)
}
func (c *Client) CreateFiles(ctx context.Context, p CreateFilesInput) (Files, error) {
	return smartCall[*CreateFilesInput, Files](ctx, c, "app_file/create", &p, true)
}
func (c *Client) UpdateFiles(ctx context.Context, p UpdateFilesInput) (Files, error) {
	seen := map[int64]bool{}
	for _, v := range p.EnvironmentPreconditions {
		if seen[v.EnvID] {
			return Files{}, errors.New("duplicate environment precondition")
		}
		seen[v.EnvID] = true
	}
	return smartCall[*UpdateFilesInput, Files](ctx, c, "app_file/update", &p, true)
}
func (c *Client) GetFileOperation(ctx context.Context, p FileOperationInput) (Files, error) {
	return smartCall[*FileOperationInput, Files](ctx, c, "app_file/operation/get", &p, false)
}
func (c *Client) CreateRelease(ctx context.Context, p CreateReleaseInput) (Release, error) {
	if p.SourceEnvID == p.TargetEnvID {
		return Release{}, errors.New("source and target environments must differ")
	}
	r, e := smartCall[*CreateReleaseInput, Release](ctx, c, "release/create", &p, true)
	if e == nil && (r.ID < 1 || r.ActorID != p.ActorID || r.EnvID != p.TargetEnvID || r.SourceEnvID != p.SourceEnvID) {
		e = &WriteError{Cause: errors.New("release result identity mismatch")}
	}
	return r, e
}
func (c *Client) GetReleaseOperation(ctx context.Context, p AppOperationInput) (Release, error) {
	r, e := smartCall[*AppOperationInput, Release](ctx, c, "release/operation/get", &p, false)
	if e == nil && (r.ID < 1 || r.ActorID != p.ActorID) {
		return Release{}, errors.New("release result identity mismatch")
	}
	return r, e
}
func (c *Client) GetRelease(ctx context.Context, p ReleaseInput) (Release, error) {
	r, e := smartCall[*ReleaseInput, Release](ctx, c, "release/get", &p, false)
	if e == nil && (r.ID != p.ReleaseID || r.ActorID != p.ActorID) {
		return Release{}, errors.New("release result identity mismatch")
	}
	return r, e
}
func (c *Client) GetRouteKey(ctx context.Context, p AppInput) (RouteKey, error) {
	return smartCall[*AppInput, RouteKey](ctx, c, "route/key/get", &p, false)
}
