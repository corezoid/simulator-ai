package mcpserver

import (
	"encoding/json"
	"strconv"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleTasks registers task_get and graph_file_upload around the export and import tools.
func addConsoleTasks(s *server.MCPServer, c *publicapi.Client) {
	addParity(s, c, taskTools)
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(`{"type":"object"}`)), "task_get",
		"Read a background task (an export, import, cloned graph or zip) of the workspace: status created or started means it is still running, so call task_get again a few seconds later; it ends completed (details), failed (error; an export of objects you may not read lists them) or canceled. A completed export or zip adds downloadUrl, the archive's link, which works without signing in: give it only to the user who asked. Only the person who started a task sees its archive. with [\"user\"] adds who started it. Public task/get.",
		`{"type":"object","properties":{"taskId":{"type":"integer","minimum":1},"with":{"type":"array","items":{"type":"string","enum":["user"]}}},"required":["taskId"],"additionalProperties":false}`, false, c.GetTask)
	consoleTool(s, "graph_file_upload", "Upload a .graph archive (up to "+strconv.Itoa(publicapi.MaxUploadBytes>>10)+" KiB) for graph_import_create: content is its bytes in standard base64, fileName ends in .graph (default import.graph). Stored attached to nothing; returns {fileName, title, size}, fileName being what graph_import_create takes. To re-import an export of this workspace, pass its details.file.fileName instead.",
		`{"type":"object","properties":{"fileName":{"type":"string","minLength":7,"maxLength":255},"content":{"type":"string","minLength":4,"maxLength":`+strconv.Itoa((publicapi.MaxUploadBytes+2)/3*4)+`}},"required":["content"],"additionalProperties":false}`, true, c.UploadGraphFile)
}
