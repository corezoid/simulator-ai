package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleLayerSettings registers the typed write of a layer's own settings (its data keys).
func addConsoleLayerSettings(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "layer_settings_update", "Change a layer's own settings (its data; only sent keys change): defaultFormId (template of new objects; 0 clears), edgeCurveStyle (default link shape), anisotropy, outerEdges, initialCell (where it opens), accounts (\"Accounts on the graph\": balances drawn on its actors, by name; the WHOLE list, [] hides), accountsPeriod (their window, Unix seconds; {} clears), maxVisibleChildren (\"show at most N children\": 0 off, or 1-200, the editor offers 10, 25, 50; a wider fan folds its bare leaves past N for every viewer). Skill simulator-ai-graph, Layer settings.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"defaultFormId":{"type":"integer","minimum":0},"edgeCurveStyle":{"type":"string","enum":["curved","rounded","roundedDownward","straight"]},"anisotropy":{"type":"boolean"},"outerEdges":{"type":"boolean"},"initialCell":`+cell+`,"accounts":{"type":"array","maxItems":32,"items":{"type":"object","properties":{"accountName":{"type":"string","minLength":1},"currency":{"type":"string","minLength":1}},"required":["accountName","currency"],"additionalProperties":false}},"accountsPeriod":{"type":"object","properties":{"startDate":{"type":"integer"},"endDate":{"type":"integer"},"timeZoneOffset":{"type":"integer"}},"additionalProperties":false},"maxVisibleChildren":{"type":"integer","minimum":0,"maximum":200}},"required":["layerId"],"additionalProperties":false}`, true, c.UpdateLayerSettings)
}
