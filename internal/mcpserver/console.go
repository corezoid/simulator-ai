package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/graphplan"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// consoleCoreTools is the plain console catalog: what the Console scenarios (an actor answer, a
// flowchart on a layer, a released Smart Form, a comment, a task or a chat shared with people, a file,
// a saved skill run, a link to what was made) use. Every schema rides in the model context on every
// turn, so the rest of the console tools are console-full only.
var consoleCoreTools = map[string]bool{
	"actor_get": true, "actor_create": true, "actors_create": true, "actor_update": true, "actors_search": true,
	"form_get": true, "forms_list": true, "graph_get": true,
	"flowchart_blocks_list": true, "flowchart_layout": true, "layer_create": true, "layer_get": true,
	"layer_placements_create": true, "layer_positions_update": true, "edge_create": true, "edges_create": true,
	"smart_form_components": true, "smart_form_data_form_create": true, "smart_form_create": true, "smart_form_files_get": true, "smart_form_files_update": true,
	"smart_form_files_create": true, "smart_form_folders_create": true, "smart_form_release": true, "smart_form_release_get": true,
	"actor_by_ref_get": true, "reactions_list": true, "reaction_create": true, "users_search": true, "access_rules_get": true, "access_rules_save": true,
	"attachment_upload": true, "attachment_read": true, "skills_find": true, "skill_get": true, "build_link": true,
	// Accounts on an actor by name ("open a Default USD account"), its rows and balances, and a move of
	// value: a transfer between actors' accounts, a transaction when one account is named.
	"account_create": true, "accounts_list": true, "transfer_create": true, "transaction_create": true,
	// Account Templates: the template itself and the accounts every actor of it has.
	"form_create": true, "form_update": true, "form_account_create": true,
	// A p2p or group chat in one call (its reuse ref needs the creator's id, which no tool gives).
	"chat_open": true,
	// The actor, form and Smart Form cards an actor shows.
	"actor_attach": true,
	// The Corezoid business logic offered after a Smart Form is released: a project of its own,
	// read until ready.
	"corezoid_project_create": true, "corezoid_project_get": true,
}

// keepConsoleCore drops every tool outside consoleCoreTools.
func keepConsoleCore(s *server.MCPServer) {
	var drop []string
	for name := range s.ListTools() {
		if !consoleCoreTools[name] {
			drop = append(drop, name)
		}
	}
	s.DeleteTools(drop...)
}

// idempotentTools are console writes whose repeat returns the same result and changes nothing more.
// chat_open is not: a group chat is new on every call.
var idempotentTools = map[string]bool{"account_pair_create": true, "account_create": true, "form_account_create": true}

// destructiveTools delete what they touch; clients may ask for confirmation on this hint.
var destructiveTools = map[string]bool{"edges_by_nodes_delete": true, "layer_prune_long_edges": true, "access_rules_save": true}

// maxConsoleInput keeps one tool call, and so its request, under sim-api's 1 MiB JSON-RPC body
// limit; Smart Form file batches are the largest inputs.
const maxConsoleInput = 960 << 10

const (
	actorItem    = `{"type":"object","properties":{"formId":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000},"color":{"type":"string","maxLength":32},"data":{"type":"object"},"hole":{"type":"boolean"}},"required":["formId","title"],"additionalProperties":false}`
	edgeItem     = `{"type":"object","properties":{"source":{"type":"string","minLength":1,"maxLength":255},"target":{"type":"string","minLength":1,"maxLength":255},"edgeType":{"type":"string","minLength":1,"maxLength":255},"name":{"type":"string","maxLength":1000},"curveStyle":{"type":"string","enum":["curved","rounded","roundedDownward","straight"]},"hole":{"type":"boolean"}},"required":["source","target"],"additionalProperties":false}`
	point        = `{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"}},"required":["x","y"],"additionalProperties":false}`
	nativeLayout = `{"type":"object","properties":{"width":{"type":"number"},"height":{"type":"number"},"blockId":{"type":"string"},"shape":{"type":"object"},"textFrame":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"},"w":{"type":"number"},"h":{"type":"number"}},"required":["x","y","w","h"],"additionalProperties":false}},"required":["width","height","blockId","shape","textFrame"],"additionalProperties":false}`
	// blocksOutput is written by hand: reflection would type the open data/shape JSON as byte arrays.
	blocksOutput = `{"type":"object","properties":{"list":{"type":"array","items":{"type":"object","properties":{"formId":{"type":"integer"},"title":{"type":"string"},"blockId":{"type":"string"},"collection":{"type":"string"},"data":{"type":"object"},"layerSettings":` + nativeLayout + `},"required":["formId","title","blockId"]}},"hasMore":{"type":"boolean"}},"required":["list","hasMore"]}`
)

// addConsole registers the console's own tools: a grid-aligned flowchart on a layer and a released
// Smart Form. New adds the actor, form and graph parity tools around them.
func addConsole(s *server.MCPServer, c *publicapi.Client) {
	addConsoleGraph(s, c)
	addConsoleSmartForms(s, c)
	addConsoleAccounts(s, c)
	addConsoleActors(s, c)
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
	consoleTool(s, "layer_create", "Create an empty layer (a canvas actor of the Layers system template). Returns the layer actor; its id is the layerId of the layer tools. Asked to visualize something in the chat, first ask how: a Smart Form showing a layer in a GraphEditor block (a flowchart, or plain actors on the layer), or plain text; never draw ASCII diagrams.",
		`{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":1000},"description":{"type":"string","maxLength":10000}},"required":["title"],"additionalProperties":false}`, true, c.CreateLayer)
	consoleTool(s, "edge_create", "Create ONE edge; for two or more use edges_create (one call). Link two actors with an edge (source -> target) of the named type, default hierarchy. name is the label drawn on the edge. The direction is kept: an existing edge of this type from source to target is returned unchanged, a reverse one is not reused. Tree edge types refuse a second parent and cycles. hole makes a placeholder edge (an edge to a hole actor need not be one). Returns id/source/target/edgeTypeId; draw it with layer_placements_create.",
		`{"type":"object","properties":{"source":{"type":"string","minLength":1,"maxLength":255},"target":{"type":"string","minLength":1,"maxLength":255},"edgeType":{"type":"string","minLength":1,"maxLength":255},"name":{"type":"string","maxLength":1000},"curveStyle":{"type":"string","enum":["curved","rounded","roundedDownward","straight"]},"hole":{"type":"boolean"}},"required":["source","target"],"additionalProperties":false}`, true, c.CreateEdge)
	consoleTool(s, "actors_create", "Create 2-100 actors in ONE call instead of repeated actor_create; use it for two or more. Each item as in actor_create: formId (templates may differ), title, optional description, data (field values; for a flowchart block formId, data and color from flowchart_layout), color (optional; without it a random colour of the editor palette), hole (a placeholder for an actor not known yet: its data fields are only the conditions that will close it). Atomic: all are created or none, and an error names the failed item's index. Returns actors [{id, title, formId, color, ...}] in input order. Does not place them on a layer.",
		`{"type":"object","properties":{"actors":{"type":"array","minItems":1,"maxItems":100,"items":`+actorItem+`}},"required":["actors"],"additionalProperties":false}`, true, c.CreateActors)
	consoleTool(s, "edges_create", "Create 2-100 edges in ONE call instead of repeated edge_create; use it for two or more. Each item as in edge_create: source -> target (direction kept), optional edgeType name (default hierarchy), name (the label drawn on the edge), curveStyle, hole. Atomic: all are created or none, and an error names the failed item's index. Returns edges [{id, source, target, edgeTypeId, name}] in input order; draw them with layer_placements_create.",
		`{"type":"object","properties":{"edges":{"type":"array","minItems":1,"maxItems":100,"items":`+edgeItem+`}},"required":["edges"],"additionalProperties":false}`, true, c.CreateEdges)
	consoleTool(s, "layer_placements_create", "Place existing actors (nodes) and edges on a layer in one transaction, 1-100 items. A position is the centre of the block in graph units (x right, y down, 50-unit cells); take flowchart positions from flowchart_layout. A flowchart node (layerSettings set, verbatim from flowchart_layout: resizing is not supported) is drawn like Graph Editor does: its corner is moved onto the nearest grid line and its shape is stored in color (the actor's own colour when you omit it). Edges need the laId of both node placements, so place nodes first, then edges in a second call. Returns nodes [{laId, actorId, position}] and edges [{laId, edgeId, laIdSource, laIdTarget}].",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"nodes":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255},"position":`+point+`,"layerSettings":`+nativeLayout+`,"color":{"type":"string","maxLength":32}},"required":["actorId","position"],"additionalProperties":false}},"edges":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"edgeId":{"type":"string","minLength":1,"maxLength":255},"laIdSource":{"type":"integer","minimum":1},"laIdTarget":{"type":"integer","minimum":1}},"required":["edgeId","laIdSource","laIdTarget"],"additionalProperties":false}}},"required":["layerId"],"additionalProperties":false}`, true, gridPlacements(c))
	consoleTool(s, "layer_positions_update", "Move node placements (laId) on a layer to new x/y graph units; all or none are applied, and a call fails when a target grid cell is taken. Edges follow their endpoints.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"items":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"laId":{"type":"integer","minimum":1},"x":{"type":"number"},"y":{"type":"number"}},"required":["laId","x","y"],"additionalProperties":false}}},"required":["layerId","items"],"additionalProperties":false}`, true, c.UpdatePositions)
	consoleTool(s, "layer_get", "Read what is on a layer: node placements [{laId, actorId, title, formId, position, blockId, hole, closedHole}] and edge placements [{laId, edgeId, laIdSource, laIdTarget, source, target, name, hole, closedHole}], most recent first; hole marks a placeholder, closedHole a placement that closed one. Default/maximum limit 200/500 per kind; hasMore means read the next offset.",
		`{"type":"object","properties":{"layerId":{"type":"string","minLength":1,"maxLength":255},"limit":{"type":"integer","minimum":1,"maximum":500},"offset":{"type":"integer","minimum":0}},"required":["layerId"],"additionalProperties":false}`, false, c.GetLayer)
}

func addConsoleSmartForms(s *server.MCPServer, c *publicapi.Client) {
	addParity(s, c, smartFormTools)
	consoleTool(s, "smart_form_data_form_create", "Create the data template a Smart Form saves into, and get its page: only when the user asked to keep what is submitted, or agreed when you asked; a form that only shows or calculates needs none. Create the Smart Form first and pass its appId: form/submit then lets whoever may open that form (its sharedWith) submit. fields [{name, label, type text|number|date|select, required?, options? for select}]: each becomes a field (name is its id; number is edit/float, date a calendar) of the template's one section and a bound input of the page. Returns formId and page: a ready pages/<page>/config source (a Form node, the inputs, and a submitLabel button that validates and submits via form/submit, creating one actor of the template per submit, titled by the first text field, else by the template title (255 characters), so give records a text field to tell them apart; empty fields left out; successMessage shown). Needs form/submit on the stand. Write page with smart_form_files_update as pages/index/config, then smart_form_release.",
		`{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":1000},"fields":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"object","properties":{"name":{"type":"string","pattern":"^[A-Za-z][A-Za-z0-9_]{0,63}$"},"label":{"type":"string","minLength":1,"maxLength":255},"type":{"type":"string","enum":["text","number","date","select"]},"required":{"type":"boolean"},"options":{"type":"array","maxItems":100,"items":{"type":"string","minLength":1,"maxLength":255}}},"required":["name","label"],"additionalProperties":false}},"submitLabel":{"type":"string","maxLength":100},"successMessage":{"type":"string","maxLength":200},"appId":{"type":"string","minLength":1,"maxLength":64}},"required":["title","fields"],"additionalProperties":false}`, true, c.CreateDataForm)
	consoleTool(s, "smart_form_create", "Create a Smart Form (CDU v2). It starts with a develop tree holding a pages/index page (config with an empty Stack), locale, viewModel and styles/index. It creates no data template: add one (smart_form_data_form_create) only when the user asked to save submitted data or agreed when asked. Returns appId and env ids; edit develop with smart_form_files_get/update/create, then smart_form_release.",
		`{"type":"object","properties":{"ref":{"type":"string","pattern":"^[0-9a-zA-Z_-]{5,255}$","description":"Short name unique among the workspace's Smart Forms"},"title":{"type":"string","minLength":1,"maxLength":255},"description":{"type":"string","maxLength":10000},"sharedWith":{"type":"string","enum":["userList","allWorkspaceUsers","allRegisteredUsers"],"description":"Who may open it; default userList (its access list). Widen only when the user asks"}},"required":["ref","title"],"additionalProperties":false}`, true, c.CreateSmartForm)
	consoleTool(s, "smart_form_files_get", "List the develop folders and files of a Smart Form by path (pages/index/config, locale, styles/index, ...) with id, type and sourceHash; withSource adds file sources. Use folder ids for new folders and files and file ids for updates. developEnvId and productionEnvId name its environments.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"withSource":{"type":"boolean"}},"required":["appId"],"additionalProperties":false}`, false, c.GetSmartFormFiles)
	consoleTool(s, "smart_form_files_update", "Replace develop files of a Smart Form in one transaction. title and type are always written: send the current ones. A pages/<page>/config source must be a valid CDU page config ({schemaVersion:\"2.0\", id, tree}). A form the user fills in must save somewhere: take its page from smart_form_data_form_create, which also creates the data template it submits into; a page with inputs and an action-less button but no Form, a Form neither a form.submit names nor any simapi.call, widget.call, bus.publish, navigate.local/route or copy sends (vm.<valueKey>), or a form/submit without simapiExtraParams.formId is refused (accId and appId are set for you). Take component types and props ONLY from smart_form_components (guessed types such as DateInput or TextField are refused); the error carries the server's message naming what is wrong. ifMatchHash (the sourceHash you read) refuses a stale overwrite. Returns ids and new sourceHash. One call must stay under 960 KiB of arguments: split larger batches.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"items":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{"id":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":255},"type":{"type":"string","minLength":1,"maxLength":255},"source":{"type":"string","maxLength":65536},"ifMatchHash":{"type":"string","pattern":"^[0-9a-f]{64}$"}},"required":["id","title","type"],"additionalProperties":false}}},"required":["appId","items"],"additionalProperties":false}`, true, c.UpdateSmartFormFiles)
	consoleTool(s, "smart_form_files_create", "Create develop files of a Smart Form in one transaction, e.g. a pages/<page>/locale next to the page config. folderId comes from smart_form_files_get. Titles are not unique: update an existing file instead of creating a second one. Returns ids and sourceHash. One call must stay under 960 KiB of arguments: split larger batches.",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"items":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{"folderId":{"type":"integer","minimum":1},"title":{"type":"string","minLength":1,"maxLength":255},"type":{"type":"string","minLength":1,"maxLength":255},"source":{"type":"string","maxLength":65536}},"required":["folderId","title","type","source"],"additionalProperties":false}}},"required":["appId","items"],"additionalProperties":false}`, true, c.CreateSmartFormFiles)
	consoleTool(s, "smart_form_release", "Release the Smart Form: deploy develop into production, which then serves it. Returns {appId, releaseId, releaseNumber, status, ready, marker}; ready true means the form can be opened by appId. When ready, end your final answer with the marker line exactly as returned (appId: <appId>) so the reply embeds the form. A refused page config or route is reported and nothing is deployed. After the first release of a form you built, offer the user Corezoid processes for its business logic (loading data, handling submits): with a yes, corezoid_project_create, then corezoid_project_get until ready (skill simulator-ai-smart-forms-logic).",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255}},"required":["appId"],"additionalProperties":false}`, true, c.ReleaseSmartForm)
	consoleTool(s, "smart_form_release_get", "Read the current state of a Smart Form release: {appId, releaseId, releaseNumber, status, ready, marker}. ready is true while it is the active production release. Without releaseId it reads the release production serves now: use that to embed a form released before this run (end the answer with its marker).",
		`{"type":"object","properties":{"appId":{"type":"string","minLength":1,"maxLength":255},"releaseId":{"type":"integer","minimum":1}},"required":["appId"],"additionalProperties":false}`, false, c.GetSmartFormRelease)
}

func consoleTool[P, R any](s *server.MCPServer, name, description, schema string, write bool, run func(context.Context, P) (R, error)) {
	addConsoleTool(s, mcp.WithOutputSchema[R](), name, description, schema, write, run)
}

func addConsoleTool[P, R any](s *server.MCPServer, output mcp.ToolOption, name, description, schema string, write bool, run func(context.Context, P) (R, error)) {
	t := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(json.RawMessage(schema)), output,
		mcp.WithReadOnlyHintAnnotation(!write), mcp.WithDestructiveHintAnnotation(destructiveTools[name]), mcp.WithIdempotentHintAnnotation(!write || idempotentTools[name]))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decodeLimit[P](r, maxConsoleInput)
		if e != nil {
			return failure(e), nil
		}
		v, e := run(ctx, a)
		if raw, ok := any(v).(json.RawMessage); ok {
			return passResult(raw, e), nil
		}
		return result(v, e), nil
	})
}
