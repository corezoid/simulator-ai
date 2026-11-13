package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const chartDataDescription = "Read a chart's (dashboard actor's) series computed by the server from its saved accounts or actor filter: one series per " +
	"account (actorId, nameId, currencyId, incomeType, accountType) with value. The period is yours, never the chart's saved range: from and to " +
	"together (Unix ms) from the request, bucketed by interval (default day; at most 2 hours for minute, 7 days for hour, 2 years otherwise) " +
	"in timezoneOffset minutes (JS sign: Kyiv summer is -180). line and stackedBar charts need the period and return data[] buckets (an " +
	"empty bucket is absent); their value is only the base the series starts from: 0, or the value at from with balanceOnDate. Every " +
	"other chart type returns the period's amount (transaction count with counterType count) as value, or the current value without a " +
	"period. with: series fills data[] for every chart type (needs from and to); total adds total {value, data[]}, every series summed as " +
	"a whole and per bucket, beside the series (an amount chart mixing currencies is refused with invalidParams). actorIds keeps only " +
	"those actors' series. An actorFilter chart's top actors are ranked by their current balance whatever the period. The answer is " +
	"bounded: narrow a large one with a shorter period, a coarser interval, actorIds or (non-line charts) without series. Series the " +
	"caller may not see are left out. Needs view on the chart (and its ActorFilters actor); notFound when chartId is not a chart."

// maxChartDataBytes bounds the answer: a structured result rides in the model context whole, and
// 100 series of 2 years by day would be about 3 MB.
const maxChartDataBytes = 256 << 10

// addDashboard registers chart_data_get, the read of public dashboard/get.
func addDashboard(s *server.MCPServer, c *publicapi.Client) {
	input, output, err := contracts.DashboardSchemas()
	if err != nil {
		panic(err)
	}
	t := mcp.NewTool("chart_data_get", mcp.WithDescription(chartDataDescription), mcp.WithRawInputSchema(input), mcp.WithRawOutputSchema(output),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 100 actorIds of up to 255 bytes do not fit the default 4 KiB argument bound.
		in, err := decodeLimit[publicapi.DashboardInput](r, 32<<10)
		if err != nil {
			return failure(err), nil
		}
		v, err := c.GetDashboard(ctx, in)
		if err != nil {
			return failure(err), nil
		}
		if b, err := json.Marshal(v); err != nil || len(b) > maxChartDataBytes {
			return failure(fmt.Errorf("chart data of %d series is %d KiB, more than %d KiB: use a shorter period, a coarser interval, actorIds or, on a non-line chart, no series", len(v.Data), len(b)>>10, maxChartDataBytes>>10)), nil
		}
		return result(v, nil), nil
	})
}
