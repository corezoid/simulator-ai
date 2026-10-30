// Package mcpserver exposes only the R1.1 public read profile to a host agent.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const Version = "0.1.0-dev.2"

type ActorArgs struct {
	ActorID string `json:"actorId"`
}
type FormArgs struct {
	FormID int64 `json:"formId"`
}
type LayerArgs struct {
	LayerID string `json:"layerId"`
	Limit   *int64 `json:"limit,omitempty"`
	Offset  int64  `json:"offset,omitempty"`
}

func New(c *publicapi.Client) *server.MCPServer {
	s := server.NewMCPServer("simulator-ai", Version, server.WithToolCapabilities(false), server.WithInputSchemaValidation())
	s.AddTool(tool[publicapi.Actor]("actor_get", "Read actor metadata in the configured workspace. Returns id/title/description/formId/status/ref only; no data fields or related objects. Public actor/get; actors.readonly scope and actor View required.", `{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255}},"required":["actorId"],"additionalProperties":false}`), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decode[ActorArgs](r)
		if e != nil {
			return failure(e), nil
		}
		v, e := c.GetActor(ctx, a.ActorID)
		return result(v, e), nil
	})
	s.AddTool(tool[publicapi.Form]("form_get", "Read account template metadata in the configured workspace. Returns id/title/description/type/parentId only, NOT the form field schema or editor token. Public form/get; forms.readonly scope and template View required.", `{"type":"object","properties":{"formId":{"type":"integer","minimum":1}},"required":["formId"],"additionalProperties":false}`), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decode[FormArgs](r)
		if e != nil {
			return failure(e), nil
		}
		v, e := c.GetForm(ctx, a.FormID)
		return result(v, e), nil
	})
	s.AddTool(tool[publicapi.LayerResult]("layer_actors_list", "Read ONE page of node placements and permitted actor metadata. No edges or total. Layer View and per-actor ACL apply. Offset pagination is not a stable snapshot under concurrent edits. Default/maximum limit 50; short or empty pages do not prove completion: deleted actors are filtered after pagination. This tool cannot certify complete enumeration.", `{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"limit":{"type":"integer","minimum":1,"maximum":50},"offset":{"type":"integer","minimum":0}},"required":["layerId"],"additionalProperties":false}`), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decode[LayerArgs](r)
		if e != nil {
			return failure(e), nil
		}
		limit := int64(50)
		if a.Limit != nil {
			limit = *a.Limit
		}
		v, e := c.ListLayerActors(ctx, a.LayerID, limit, a.Offset)
		return result(v, e), nil
	})
	return s
}
func tool[T any](name, description, schema string) mcp.Tool {
	return mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(json.RawMessage(schema)), mcp.WithOutputSchema[T](), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
}
func decode[T any](r mcp.CallToolRequest) (T, error) {
	var a T
	b, e := json.Marshal(r.GetRawArguments())
	if e != nil || len(b) > 4096 {
		return a, errors.New("invalid or oversized tool arguments")
	}
	if len(b) == 0 || b[0] != '{' {
		return a, errors.New("tool arguments must be an object")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&a); e != nil {
		return a, errors.New("invalid tool arguments: use the declared input schema")
	}
	var extra json.RawMessage
	if d.Decode(&extra) != io.EOF {
		return a, errors.New("trailing tool arguments")
	}
	return a, nil
}
func failure(e error) *mcp.CallToolResult { return mcp.NewToolResultError(e.Error()) }
func result[T any](v T, e error) *mcp.CallToolResult {
	if e != nil {
		return failure(e)
	}
	return mcp.NewToolResultStructuredOnly(v)
}
