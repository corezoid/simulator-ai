package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleHoles registers the hole closes: a placeholder is closed by an actor or an edge that
// takes its place, never by creating a new actor over it.
func addConsoleHoles(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "hole_close", "Close a hole: a placeholder actor on a layer (layer_get: hole true). The closer takes its placement and links; the hole stays (hole_reopen). Never create an actor over a hole instead. actorId: an existing closer; newActor: create it in the same atomic call (title; formId and data default to the hole's, whose non-empty data fields are its conditions). A closer that misses a condition is refused with each verdict; forceMerge closes a partial match, only on the user's yes. holeLaId: only that placement. A placeholder link: edge_hole_close.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"holeActorId":{"type":"string","minLength":1,"maxLength":255},"actorId":{"type":"string","minLength":1,"maxLength":255},"newActor":{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":1000},"formId":{"type":"integer","minimum":1},"description":{"type":"string","maxLength":10000},"color":{"type":"string","maxLength":32},"data":{"type":"object"}},"required":["title"],"additionalProperties":false},"holeLaId":{"type":"integer","minimum":1},"forceMerge":{"type":"boolean"}},"required":["layerId","holeActorId"],"additionalProperties":false}`, true, c.CloseHole)
	consoleTool(s, "edge_hole_close", "Close a placeholder link (layer_get: edge hole true) with a real edge between the same actors in the same direction: edgeId, or a new edge of the hole's type (name its label) made in the same atomic call. It takes the hole's placement; the hole stays (edge_hole_reopen).",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"holeEdgeId":{"type":"string","minLength":1,"maxLength":255},"edgeId":{"type":"string","minLength":1,"maxLength":255},"name":{"type":"string","maxLength":1000}},"required":["layerId","holeEdgeId"],"additionalProperties":false}`, true, c.CloseEdgeHole)
}
