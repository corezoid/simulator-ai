package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addLayerStats(s *server.MCPServer, c *publicapi.Client) {
	input, output, err := contracts.LayerStatsSchemas()
	if err != nil {
		panic(err)
	}
	description := "Read six recipient-eligible layer counters through public graph/layer/stats/get. actorId is the layer actor; configured workspace is host-owned. Requires actors.readonly and current layer View. Counts placements, including repeated and geometry-only placements; recipient restrictions still apply. Backend may repair shared counters but returns fresh primary counts, never shared cache authority. No model writes or realtime event; retries recheck access. Counts and separate pages are not a snapshot. Errors are never zero."
	if c.LayerStatsActorBound() {
		input = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
		description += " This process is host-bound: call with {} and no actorId; the configured layer cannot be changed."
	}
	t := mcp.NewTool("graph_layer_stats_get", mcp.WithDescription(description), mcp.WithRawInputSchema(input), mcp.WithRawOutputSchema(output), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(t, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decode[ActorArgs](request)
		if err != nil {
			return failure(err), nil
		}
		value, err := c.GetLayerStats(ctx, args.ActorID)
		return result(value, err), nil
	})
}
