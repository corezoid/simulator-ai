package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addReactionReads(s *server.MCPServer, c *publicapi.Client) {
	schema, err := contracts.ReactionStatsInputSchema()
	if err != nil {
		panic(err)
	}
	description := "Read reaction counts by type under actorId through public reaction/stats/get in the configured workspace. Requires actors.readonly and current root View. Hidden counts additionally require Execute; recipient restrictions still apply. Missing types stay absent; an empty object means no eligible reactions, never a permission/transport error. No per-author data, writes, retries or cached authorization. Legacy getReactionsStats equivalent with current privacy enforcement."
	if c.ReactionActorBound() {
		schema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
		description = "Read reaction counts for the host-bound actor; pass {} with no actorId. The host target cannot be changed by tool arguments. Requires current public API scope/root View; Execute and recipient restrictions apply independently. Missing types stay absent, {} means no eligible reactions, and access/transport failures are errors rather than zero. No writes or cached authorization."
	}
	t := mcp.NewTool("reaction_stats_get",
		mcp.WithDescription(description),
		mcp.WithRawInputSchema(schema), mcp.WithRawOutputSchema(json.RawMessage(contracts.ReactionStatsOutputSchema)),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(t, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decode[ActorArgs](request)
		if err != nil {
			return failure(err), nil
		}
		value, err := c.GetReactionStats(ctx, args.ActorID)
		return result(value, err), nil
	})
}
