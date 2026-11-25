package mcpserver

import (
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// layerDigestOutput is written by hand: the reflected one would carry every nested struct.
const layerDigestOutput = `{"type":"object","properties":{"layerId":{"type":"string"},"title":{"type":"string"},"stats":{"type":"object"},"forms":{"type":"array"},"levels":{"type":"array"},"actors":{"type":"array"},"balances":{"type":"array"},"balanceSums":{"type":"array"},"transfers":{"type":"array"},"notes":{"type":"array"}},"required":["layerId","forms","levels","actors"]}`

// addConsoleLayerDigest registers layer_digest: a layer summarised for a person in one call.
func addConsoleLayerDigest(s *server.MCPServer, c *publicapi.Client) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(layerDigestOutput)), "layer_digest", "Summarise a layer in ONE call — use it FIRST for \"summarise this layer\", \"what is on it\", a report for a manager: stats (nodes, edges), forms (actors per template), levels (actors per hierarchy depth on the layer), actors [{id, title, form, level, parentId, data (filled fields)}], balances [{actorId, title, form, account, currency, balance}] — what each actor holds now (credit - debit), the answer for an actor's money — balanceSums (sumOfBalances per template and account; never add them across templates), transfers between its actors [{from, to, amount, account, currency, count}], notes for the person. Balances are the accounts the layer shows in its settings, else every non-zero fact one. Compact: 80 actors, 4 fields each, values cut; full: true lists all.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"full":{"type":"boolean"}},"required":["layerId"],"additionalProperties":false}`, false, c.LayerDigest)
}
