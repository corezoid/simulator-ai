package mcpserver

import (
	"context"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const chartDataDescription = "Read what a chart (dashboard actor) shows, computed by the server from its saved configuration: one series per account " +
	"(actorId, nameId, currencyId, incomeType, accountType) with value. A period is from and to together (Unix ms), bucketed by interval " +
	"(default day; at most 2 hours for minute, 7 days for hour, 2 years otherwise) in timezoneOffset minutes (Kyiv summer is -180). " +
	"line and stackedBar charts need the period and return data[] buckets (the movement in each; empty buckets are absent); their value is " +
	"only the base the series starts from: 0, or the balance at from with balanceOnDate. Every other chart type returns the period's amount " +
	"(transaction count with counterType count) as value, or the current value without a period. with: series fills data[] for every chart " +
	"type (needs from and to); total adds total {value, data[]}, every series summed as a whole and per bucket (an amount chart mixing " +
	"currencies is refused with invalidParams). actorIds keeps only those actors' series. A long period at a fine interval is a large " +
	"answer: prefer with total, a coarser interval or actorIds. Series the caller may not see are left out. Needs view on the chart (and on " +
	"its ActorFilters actor); notFound when chartId is not a chart."

// addDashboard registers chart_data_get, the read of public dashboard/get.
func addDashboard(s *server.MCPServer, c *publicapi.Client) {
	input, output, err := contracts.DashboardSchemas()
	if err != nil {
		panic(err)
	}
	t := mcp.NewTool("chart_data_get", mcp.WithDescription(chartDataDescription), mcp.WithRawInputSchema(input), mcp.WithRawOutputSchema(output),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 100 actorIds of 36-byte ids do not fit the default 4 KiB argument bound.
		in, err := decodeLimit[publicapi.DashboardInput](r, 16<<10)
		if err != nil {
			return failure(err), nil
		}
		v, err := c.GetDashboard(ctx, in)
		return result(v, err), nil
	})
}
