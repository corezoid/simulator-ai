package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

const strokeItem = `{"type":"object","properties":{"kind":{"type":"string","enum":["line","rect","ellipse"]},"points":{"type":"array","items":` + point + `},"center":` + point + `,"w":{"type":"number"},"h":{"type":"number"},"color":{"type":"string"},"width":{"type":"number"},"style":{"type":"string","enum":["solid","dashed","dotted"]},"smooth":{"type":"boolean"},"closed":{"type":"boolean"},"arrowStart":{"type":"boolean"},"arrowEnd":{"type":"boolean"}},"additionalProperties":false}`

// addConsoleDraw registers the Pencil in words: strokes become the picture a confirmed Pencil
// session is.
func addConsoleDraw(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "layer_draw", "Draw on a layer as the Graph Editor Pencil does: the strokes become ONE picture actor (the layer's default form; formId when it has none) placed where they lie, off the grid. Graph units (x right, y down, 50 per cell), relative to cell's centre when set. Stroke: kind line (points; smooth, closed, arrowStart/arrowEnd), rect or ellipse (center, w, h; outline only); color #RRGGBB (default #667085), width 1-20 (default 3), style solid|dashed|dotted. No fill, eraser or text. At most 2000 x 2000 units.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"strokes":{"type":"array","minItems":1,"maxItems":100,"items":`+strokeItem+`},"cell":`+cell+`,"formId":{"type":"integer","minimum":1},"title":{"type":"string","maxLength":1000}},"required":["layerId","strokes"],"additionalProperties":false}`, true, c.DrawOnLayer)
}
