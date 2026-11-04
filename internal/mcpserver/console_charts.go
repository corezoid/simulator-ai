package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

const (
	chartSettings = `"chartType":{"type":"string","enum":["line","bar","stackedBar","pie","doughnut","funnel","table","polarArea","radar"]},` +
		`"counterType":{"type":"string","enum":["amount","count"]},` +
		`"range":{"type":"string","enum":["allTime","lastMinute","last10Minutes","lastHour","today","yesterday","lastWeek","lastWeekExcludeToday","last14Days","last14DaysExcludeToday","lastMonth","lastMonthExcludeToday","previousMonth","lastYear","lastYearExcludeToday","realTime","lineRealTime"]},` +
		`"orderValue":{"type":"string","enum":["default","desc","asc"]}`
	chartFilter = `"filterActorId":{"type":"string","minLength":36,"maxLength":36},"filterTitle":{"type":"string","minLength":1,"maxLength":1000},` +
		`"sourceFormId":{"type":"integer","minimum":1},"accountNameId":{"type":"string","minLength":36,"maxLength":36},"currencyId":{"type":"integer","minimum":1},"top":{"type":"integer","minimum":1,"maximum":100}`
	incomeType    = `{"type":"string","enum":["total","credit","debit"]}`
	chartAccounts = `"accounts":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"actorId":{"type":"string","minLength":36,"maxLength":36},"nameId":{"type":"string","minLength":36,"maxLength":36},"currencyId":{"type":"integer","minimum":1},"color":{"type":"string","maxLength":32},"incomeType":` + incomeType + `},"required":["actorId","nameId","currencyId"],"additionalProperties":false}}`
	chartModes    = "Two kinds: a manual chart lists accounts [{actorId, nameId (account name id), currencyId, color?, incomeType total|credit|debit}], one series each; an actorFilter chart shows the top actors of a template ranked by one account: pass sourceFormId, accountNameId and currencyId (a new ActorFilters actor is made, titled filterTitle) or filterActorId to reuse one, and top (default 20). "
	chartRules    = "chartType default line, counterType amount (values) or count (transactions), range default lastHour, orderValue default/desc/asc. line and stackedBar take no allTime and no sorting; realTime is stored as lineRealTime for them (and back for the others); warnings name each such change."
)

// addConsoleCharts registers chart_create and chart_update (legacy createChart and updateChart).
func addConsoleCharts(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "chart_create", "Create a chart on a layer: an actor of the Dashboards system template placed at position, opened as a 650 x 450 chart. "+chartModes+chartRules+" Returns {chartId, filterActorId, laId, warnings}; build_link entity chart links to it. Account inheritance from the layer's graph is not set up.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":36,"maxLength":36},"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000},`+chartSettings+`,`+chartFilter+`,`+chartAccounts+`,"position":`+point+`},"required":["layerId","title"],"additionalProperties":false}`, true, c.CreateChart)
	consoleTool(s, "chart_update", "Change a chart (chartId, an actor of the Dashboards template); what you leave out stays as stored, other keys of its source included. Settings as in chart_create. A manual chart takes new accounts, or incomeType for all its series; an actorFilter chart takes top and filterTitle, filterActorId, or changed sourceFormId / accountNameId / currencyId (a new ActorFilters actor is made, the ids left out taken from the current filter); accounts turn it manual, a filter turns it actorFilter. With laId and layerId the placement is redrawn as a chart: expand (default true), width and height (at least 650 x 450). kpiCard charts and custom ranges are edited in the web app. Returns the resulting {chartId, sourceType, filterActorId, chartType, counterType, range, orderValue, laId, warnings}; a warning names a filter the chart no longer uses.",
		`{"type":"object","properties":{"chartId":{"type":"string","minLength":36,"maxLength":36},`+chartSettings+`,`+chartFilter+`,"incomeType":`+incomeType+`,`+chartAccounts+`,"layerId":{"type":"string","minLength":36,"maxLength":36},"laId":{"type":"integer","minimum":1},"expand":{"type":"boolean"},"width":{"type":"number","minimum":650},"height":{"type":"number","minimum":450}},"required":["chartId"],"additionalProperties":false}`, true, c.UpdateChart)
}
