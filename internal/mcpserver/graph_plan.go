package mcpserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/corezoid/simulator-ai/internal/graphplan"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Local plan schema; domain metadata is fetched using the pinned public catalog contract.
const graphPlanSchema = `{"type":"object","additionalProperties":false,"required":["nodes","edges"],"properties":{
 "nodes":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","additionalProperties":false,"required":["key","blockId","title"],"properties":{"key":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"},"blockId":{"type":"string","minLength":1,"maxLength":128},"title":{"type":"string","minLength":1,"maxLength":1000}}}},
 "edges":{"type":"array","maxItems":100,"items":{"type":"object","additionalProperties":false,"required":["key","source","target"],"properties":{"key":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"},"source":{"type":"string"},"target":{"type":"string"},"name":{"type":"string","maxLength":1000}}}}
}}`

func addGraphPlan(s *server.MCPServer, c *publicapi.Client) {
	tool := mcp.NewTool("graph_flowchart_plan", mcp.WithDescription("Build a deterministic create-only native flowchart plan from ordered symbolic nodes/edges. Reads exact blockIds through public catalog in the host workspace; no writes, operation keys, actor IDs or permissions are created. Returns actual root/child form IDs, prefixed native data, validated shape/frame, non-overlapping node geometry and unchanged directed edges. Cycles are retained, with returnEdges excluded only from layout ranking. No self-loops/parallel same-direction pairs. Save the exact plan before writes; resolve actor/placement IDs from receipts, never use symbolic keys as IDs. Catalog revisions/layout are not CAS. No edge routing or edit/reload acceptance is implied."), mcp.WithRawInputSchema(json.RawMessage(graphPlanSchema)), mcp.WithOutputSchema[graphplan.Plan](), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, err := decodeLimit[graphplan.Input](request, 32<<10)
		if err != nil {
			return failure(err), nil
		}
		if err = graphplan.Validate(p); err != nil {
			return failure(err), nil
		}
		ids := []string{}
		seen := map[string]bool{}
		for _, n := range p.Nodes {
			if !seen[n.BlockID] {
				ids = append(ids, n.BlockID)
				seen[n.BlockID] = true
			}
		}
		catalog, err := c.SearchGraphBlocks(ctx, publicapi.GraphCatalogInput{BlockIDs: ids})
		if err != nil {
			return failure(err), nil
		}
		plan, err := graphplan.Build(p, catalog.List)
		if err != nil {
			return failure(err), nil
		}
		raw, err := json.Marshal(plan)
		if err != nil || len(raw) > 1<<20 {
			return failure(errors.New("native plan exceeds 1 MiB; use a smaller plan")), nil
		}
		return result(plan, nil), nil
	})
}
