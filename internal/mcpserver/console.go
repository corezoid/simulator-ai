package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/graphplan"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// maxConsoleInput keeps one tool call, and so its request, under sim-api's 1 MiB JSON-RPC body
// limit; Smart Form file batches are the largest inputs.
const maxConsoleInput = 960 << 10

const (
	point        = `{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"}},"required":["x","y"],"additionalProperties":false}`
	nativeLayout = `{"type":"object","properties":{"width":{"type":"number"},"height":{"type":"number"},"blockId":{"type":"string"},"shape":{"type":"object"},"textFrame":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"},"w":{"type":"number"},"h":{"type":"number"}},"required":["x","y","w","h"],"additionalProperties":false}},"required":["width","height","blockId","shape","textFrame"],"additionalProperties":false}`
	// blocksOutput is written by hand: reflection would type the open data/shape JSON as byte arrays.
	blocksOutput = `{"type":"object","properties":{"list":{"type":"array","items":{"type":"object","properties":{"formId":{"type":"integer"},"title":{"type":"string"},"blockId":{"type":"string"},"collection":{"type":"string"},"data":{"type":"object"},"layerSettings":` + nativeLayout + `},"required":["formId","title","blockId"]}},"hasMore":{"type":"boolean"}},"required":["list","hasMore"]}`
)

// addConsole registers the console catalog: plain public writes for a flowchart on a layer and a
// released Smart Form. actor_get is registered by New.
func addConsole(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "actor_create", "Create one actor from an account template (form) in the configured workspace. Returns id/title/description/formId/status/ref. data holds form field values by field key; omit it for none. For a flowchart block pass formId and data from flowchart_blocks_list. Does not place the actor on a layer.",
		`{"type":"object","properties":{"formId":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000},"color":{"type":"string","maxLength":32},"data":{"type":"object"}},"required":["formId","title"],"additionalProperties":false}`, true, c.CreateActor)
	consoleTool(s, "actor_update", "Change one actor: only the sent keys change; data is merged key by key into the current field values. Needs modify access. Returns id/title/description/formId/status/ref.",
		`{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255},"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000},"color":{"type":"string","maxLength":32},"data":{"type":"object"}},"required":["actorId"],"additionalProperties":false}`, true, c.UpdateActor)
	addConsoleGraph(s, c)
	addConsoleSmartForms(s, c)
}

func addConsoleGraph(s *server.MCPServer, c *publicapi.Client) {
	addConsoleTool(s, mcp.WithRawOutputSchema(json.RawMessage(blocksOutput)), "flowchart_blocks_list", "List the workspace's native flowchart block kinds (Start / Stop, Process, Decision, ...): formId, title, blockId. With blockIds each kind also returns data and layerSettings. To draw a flowchart use flowchart_layout, which returns the same per block plus its position. query filters by title or blockId substring; at most 100 kinds per call; hasMore means read the next offset.",
		`{"type":"object","properties":{"query":{"type":"string","maxLength":200},"offset":{"type":"integer","minimum":0},"blockIds":{"type":"array","items":{"type":"string","minLength":1,"maxLength":128},"minItems":1,"maxItems":100,"uniqueItems":true}},"additionalProperties":false}`, false,
		func(ctx context.Context, in publicapi.FlowchartBlocksInput) (graphplan.Blocks, error) {
			catalog, e := c.ListFlowchartBlocks(ctx, in)
			if e != nil {
				return graphplan.Blocks{}, e
			}
			return graphplan.ConsoleBlocks(catalog, len(in.BlockIDs) > 0), nil
		})
	addFlowchartLayout(s, c)
	consoleTool(s, "layer_create", "Create an empty layer (a canvas actor of the Layers system template). Returns the layer actor; its id is the layerId of the layer tools.",
		`{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000}},"required":["title"],"additionalProperties":false}`, true, c.CreateLayer)
	consoleTool(s, "edge_create", "Link two actors with an edge (source -> target) of the named type, default hierarchy. name is the label drawn on the edge. The direction is kept: an existing edge of this type from source to target is returned unchanged, a reverse one is not reused. Tree edge types refuse a second parent and cycles. Returns id/source/target/edgeTypeId; draw it with layer_placements_create.",
		`{"type":"object","properties":{"source":{"type":"string","minLength":1,"maxLength":255},"target":{"type":"string","minLength":1,"maxLength":255},"edgeType":{"type":"string","minLength":1,"maxLength":255},"name":{"type":"string","maxLength":1000},"curveStyle":{"type":"string","enum":["curved","rounded","roundedDownward","straight"]}},"required":["source","target"],"additionalProperties":false}`, true, c.CreateEdge)
	consoleTool(s, "layer_placements_create", "Place existing actors (nodes) and edges on a layer in one transaction, 1-100 items. A position is the centre of the block in graph units (x right, y down, 50-unit cells); take flowchart positions from flowchart_layout. A flowchart node (layerSettings set, verbatim from flowchart_layout: resizing is not supported) is drawn like Graph Editor does: its corner is moved onto the nearest grid line and its shape is stored in color (pass the actor's color). Edges need the laId of both node placements, so place nodes first, then edges in a second call. Returns nodes [{laId, actorId, position}] and edges [{laId, edgeId, laIdSource, laIdTarget}].",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"nodes":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255},"position":`+point+`,"layerSettings":`+nativeLayout+`,"color":{"type":"string","maxLength":32}},"required":["actorId","position"],"additionalProperties":false}},"edges":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"edgeId":{"type":"string","minLength":1,"maxLength":255},"laIdSource":{"type":"integer","minimum":1},"laIdTarget":{"type":"integer","minimum":1}},"required":["edgeId","laIdSource","laIdTarget"],"additionalProperties":false}}},"required":["layerId"],"additionalProperties":false}`, true, gridPlacements(c))
	consoleTool(s, "layer_positions_update", "Move node placements (laId) on a layer to new x/y graph units; all or none are applied, and a call fails when a target grid cell is taken. Edges follow their endpoints.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"items":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"laId":{"type":"integer","minimum":1},"x":{"type":"number"},"y":{"type":"number"}},"required":["laId","x","y"],"additionalProperties":false}}},"required":["layerId","items"],"additionalProperties":false}`, true, c.UpdatePositions)
	consoleTool(s, "layer_get", "Read what is on a layer: node placements [{laId, actorId, title, formId, position, blockId}] and edge placements [{laId, edgeId, laIdSource, laIdTarget, source, target, name}], most recent first. Default/maximum limit 200/500 per kind; hasMore means read the next offset.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"limit":{"type":"integer","minimum":1,"maximum":500},"offset":{"type":"integer","minimum":0}},"required":["layerId"],"additionalProperties":false}`, false, c.GetLayer)
}

func addConsoleSmartForms(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "smart_form_create", "Create a Smart Form (CDU v2). It starts with a develop tree holding a pages/index page (config with an empty Stack), locale, viewModel and styles/index. Returns appId and env ids; edit develop with smart_form_files_get/update/create, then smart_form_release.",
		`{"type":"object","properties":{"ref":{"type":"string","pattern":"^[0-9a-zA-Z_-]{5,255}$","description":"Short name unique among the workspace's Smart Forms"},"title":{"type":"string","minLength":1,"maxLength":255},"description":{"type":"string","maxLength":10000},"sharedWith":{"type":"string","enum":["userList","allWorkspaceUsers","allRegisteredUsers"],"description":"Who may open it; default userList (its access list). Widen only when the user asks"}},"required":["ref","title"],"additionalProperties":false}`, true, c.CreateSmartForm)
	consoleTool(s, "smart_form_files_get", "List the develop folders and files of a Smart Form by path (pages/index/config, locale, styles/index, ...) with id, type and sourceHash; withSource adds file sources. Use folder ids for new files and file ids for updates.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"withSource":{"type":"boolean"}},"required":["appId"],"additionalProperties":false}`, false, c.GetSmartFormFiles)
	consoleTool(s, "smart_form_files_update", "Replace develop files of a Smart Form in one transaction. title and type are always written: send the current ones. A pages/<page>/config source must be a valid CDU page config ({schemaVersion:\"2.0\", id, tree}); the error carries the server's message naming what is wrong. ifMatchHash (the sourceHash you read) refuses a stale overwrite. Returns ids and new sourceHash. One call must stay under 960 KiB of arguments: split larger batches.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"items":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{"id":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":255},"type":{"type":"string","minLength":1,"maxLength":255},"source":{"type":"string","maxLength":65536},"ifMatchHash":{"type":"string","pattern":"^[0-9a-f]{64}$"}},"required":["id","title","type"],"additionalProperties":false}}},"required":["appId","items"],"additionalProperties":false}`, true, c.UpdateSmartFormFiles)
	consoleTool(s, "smart_form_files_create", "Create develop files of a Smart Form in one transaction, e.g. a pages/<page>/locale next to the page config. folderId comes from smart_form_files_get. Titles are not unique: update an existing file instead of creating a second one. Returns ids and sourceHash. One call must stay under 960 KiB of arguments: split larger batches.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"items":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{"folderId":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":255},"type":{"type":"string","minLength":1,"maxLength":255},"source":{"type":"string","maxLength":65536}},"required":["folderId","title","type","source"],"additionalProperties":false}}},"required":["appId","items"],"additionalProperties":false}`, true, c.CreateSmartFormFiles)
	consoleTool(s, "smart_form_release", "Release the Smart Form: deploy develop into production, which then serves it. Returns {appId, releaseId, releaseNumber, status, ready, marker}; ready true means the form can be opened by appId. When ready, end your final answer with the marker line exactly as returned (appId: <appId>) so the reply embeds the form. A refused page config or route is reported and nothing is deployed.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255}},"required":["appId"],"additionalProperties":false}`, true, c.ReleaseSmartForm)
	consoleTool(s, "smart_form_release_get", "Read the current state of a Smart Form release: {appId, releaseId, releaseNumber, status, ready, marker}. ready is true while it is the active production release.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"releaseId":{"type":"integer","minimum":1}},"required":["appId","releaseId"],"additionalProperties":false}`, false, c.GetSmartFormRelease)
}

func consoleTool[P, R any](s *server.MCPServer, name, description, schema string, write bool, run func(context.Context, P) (R, error)) {
	addConsoleTool(s, mcp.WithOutputSchema[R](), name, description, schema, write, run)
}

func addConsoleTool[P, R any](s *server.MCPServer, output mcp.ToolOption, name, description, schema string, write bool, run func(context.Context, P) (R, error)) {
	t := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(json.RawMessage(schema)), output,
		mcp.WithReadOnlyHintAnnotation(!write), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(!write))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decodeLimit[P](r, maxConsoleInput)
		if e != nil {
			return failure(e), nil
		}
		v, e := run(ctx, a)
		return result(v, e), nil
	})
}
