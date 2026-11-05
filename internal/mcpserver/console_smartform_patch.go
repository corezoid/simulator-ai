package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// patchOpSchema is one RFC 6902 operation of smart_form_page_patch.
const patchOpSchema = `{"type":"object","properties":{"op":{"type":"string","enum":["add","replace","remove","move","copy","test"]},"path":{"type":"string","maxLength":2000,"description":"JSON Pointer, e.g. /tree/children/0/props/size; with nodeId relative to that node (/props/size, \"\" the node itself)"},"from":{"type":"string","maxLength":2000,"description":"Source pointer of move and copy"},"value":{"description":"The value of add, replace and test"}},"required":["op","path"],"additionalProperties":false}`

// addSmartFormPatch registers smart_form_page_patch: a change to part of a develop file instead
// of a rewrite, which on a big page costs the agent thousands of output tokens per try.
func addSmartFormPatch(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "smart_form_page_patch", "Change part of one Smart Form develop JSON file (default pages/index/config) by JSON Patch (RFC 6902) instead of rewriting it with smart_form_files_update: ops add, replace, remove, move, copy, test on JSON Pointers (/tree/children/0/props/size). nodeId makes every path relative to the tree node with that id (/props/size; \"\" is the node), so child indexes need no counting. All ops apply or none: a bad pointer or a failed test names the op index. It writes with ifMatchHash set to the hash it read, so a concurrent change is refused, not overwritten; pass ifMatchHash (the sourceHash you read) to also refuse one made since you read the file. sim-api's refusal of the page (unknown type or prop) is returned as is: fix the ops and retry. Returns fileId, path, the new sourceHash and the applied ops, not the source.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"path":{"type":"string","minLength":1,"maxLength":1000,"description":"Develop file path from smart_form_files_get; default pages/index/config"},"fileId":{"type":"integer","minimum":1,"description":"The file by id instead of path"},"nodeId":{"type":"string","minLength":1,"maxLength":255,"description":"id of the CDU tree node the op paths are relative to"},"ifMatchHash":{"type":"string","pattern":"^[0-9a-f]{64}$"},"ops":{"type":"array","minItems":1,"maxItems":50,"items":`+patchOpSchema+`}},"required":["appId","ops"],"additionalProperties":false}`, true, c.PatchSmartFormFile)
}
