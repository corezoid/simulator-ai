package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addGraphCreates(s *server.MCPServer, c *publicapi.Client) {
	addGraphCreate(s, "graph_actor_create", "actor/create", "Create an actor (including graph/layer/native block) under an actual accessible form. Mandatory operationId and explicit title/data. Requires actors.management and current template/context-layer authorization. No ACL overrides. Native block data uses the real child form field keys and parsed view; do not invent identifiers. Does not place the actor. Recover unknown outcomes with graph_actor_operation_get before retrying.", true, c.CreateGraphActor)
	addGraphCreate(s, "graph_actor_operation_get", "actor/operation/get", "Recover this principal/workspace actor creation receipt with current Modify. Result is the original snapshot, not current content. Missing can mean in-flight; expired never permits a new key. Does not recreate deleted actors.", false, c.GetGraphActorOperation)
	addGraphCreate(s, "graph_edge_create", "edge/create", "Create or reuse a natural-key edge. Mandatory operationId; actors.management and current endpoint/linked actor View apply. Existing edge name/style may be unchanged; a reverse hierarchy edge can be returned unless forceDirection is true. Inspect resolved source/target before placing. Tree constraints still apply. Recover with graph_edge_operation_get.", true, c.CreateGraphEdge)
	addGraphCreate(s, "graph_edge_operation_get", "edge/operation/get", "Recover original resolved edge identity after response loss. Current View on original/current endpoints and linked actors applies. Historical result may differ from a later human edit; do not rewrite it. Missing can be in-flight; expired never permits rekeying.", false, c.GetGraphEdgeOperation)
	addGraphCreate(s, "graph_placements_create", "graph/layer/actors/manage", "Create 1-100 node/edge placements atomically on a layer with operationId. Only action=create. Current layer Modify and reference View apply. Nodes need explicit graph position; edges need both exact existing placement IDs. Native layerSettings use real catalog shape/blockId/frame, not a plain actor rectangle. Backend may snap nongraphic positions. Returns original IDs/positions on replay; no deletes or updates. Recover via graph_placements_operation_get before retrying unknown outcomes.", true, c.CreateGraphPlacements)
	addGraphCreate(s, "graph_placements_operation_get", "graph/layer/operation/get", "Recover original placement IDs/positions for the exact layer/key/principal. Current layer Modify and reference access apply. Later human moves survive; this historical result must not overwrite them. Deleted/replaced placements are not recreated. Missing can be in-flight; expired forbids a new key.", false, c.GetGraphPlacementOperation)
}
func addGraphCreate[P, R any](s *server.MCPServer, name, method, description string, write bool, run func(context.Context, P) (R, error)) {
	schema, err := contracts.GraphCreateInputSchema(method)
	if err != nil {
		panic(err)
	}
	tool := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(schema), mcp.WithOutputSchema[R](), mcp.WithReadOnlyHintAnnotation(!write), mcp.WithDestructiveHintAnnotation(write), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := json.Marshal(request.GetRawArguments())
		if err == nil {
			err = contracts.ValidateGraphCreateInput(method, raw)
		}
		if err != nil {
			return failure(err), nil
		}
		p, err := decodeLimit[P](request, publicapi.MaxGraphCreateInputBytes)
		if err != nil {
			return failure(err), nil
		}
		resultValue, err := run(ctx, p)
		return result(resultValue, err), nil
	})
}
