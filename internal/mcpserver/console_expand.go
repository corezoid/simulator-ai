package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleExpand registers opening a node in place (multi-layers), with the anisotropy make-room
// the Graph Editor does around it.
func addConsoleExpand(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "layer_node_expand", "Open a node (laId) in place as the Graph Editor does: an actor shows its own layer, a Dashboards actor its chart, a Scripts actor its Smart Form, in a box of cells around it (13x9, Smart Form 11x9; width/height resize). expand false closes it. Layer anisotropy on: the rest of the layer moves out of the box (moved), back on close; off: the box covers its neighbours.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"laId":{"type":"integer","minimum":1},"expand":{"type":"boolean"},"width":{"type":"integer","minimum":5,"maximum":60},"height":{"type":"integer","minimum":4,"maximum":60}},"required":["layerId","laId"],"additionalProperties":false}`, true, c.ExpandNode)
}
