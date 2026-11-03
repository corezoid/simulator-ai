package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// addConsole registers the console write: one plain actor/create. actor_get is registered by New.
func addConsole(s *server.MCPServer, c *publicapi.Client) {
	t := mcp.NewTool("actor_create",
		mcp.WithDescription("Create one actor from an account template (form) in the configured workspace. Returns id/title/description/formId/status/ref. Public actor/create under the requesting user's own rights: actors.management scope and template access required. data holds form field values by field key; omit it for none. Does not place the actor on a layer."),
		mcp.WithRawInputSchema(json.RawMessage(`{"type":"object","properties":{"formId":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000},"data":{"type":"object"}},"required":["formId","title"],"additionalProperties":false}`)),
		mcp.WithOutputSchema[publicapi.Actor](), mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(false))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decodeLimit[publicapi.ActorCreateInput](r, 512<<10)
		if e != nil {
			return failure(e), nil
		}
		v, e := c.CreateActor(ctx, a)
		return result(v, e), nil
	})
}
