package mcpserver

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	accessObject = `"objType":{"type":"string","enum":["actor","formTemplate","templateActors","account"]},"objId":{"type":"string","minLength":1,"maxLength":255},"currencyId":{"type":"integer","minimum":1}`
	privs        = `{"type":"object","properties":{"view":{"type":"boolean"},"modify":{"type":"boolean"},"remove":{"type":"boolean"},"sign":{"type":"boolean"},"ds":{"type":"boolean"},"execute":{"type":"boolean"}},"additionalProperties":false}`
	stageOrders  = `{"type":"object","properties":{"sign":{"type":"integer","minimum":1},"ds":{"type":"integer","minimum":1},"execute":{"type":"integer","minimum":1}},"additionalProperties":false}`
)

// addConsolePeople registers the access and file tools a single public method does not cover: one
// access tool per action across every object kind, and files over sim-api's upload/download routes.
func addConsolePeople(s *server.MCPServer, c *publicapi.Client) {
	rawTool(s, "access_rules_get", "List who has access to an object and with which privileges (view, modify, remove, sign, ds, execute) and stage orders. objType actor (objId the actor id), formTemplate (the template itself, objId its id), templateActors (the default access to the template's actors) or account (objId the account name id, with currencyId).",
		`{"type":"object","properties":{`+accessObject+`},"required":["objType","objId"],"additionalProperties":false}`, false,
		func(ctx context.Context, in publicapi.AccessObject) (json.RawMessage, error) {
			return c.AccessRules(ctx, in)
		})
	rawTool(s, "access_rules_save", "Share or unshare an object in one call. objType/objId as in access_rules_get. rules [{userId, privs, reactionOrders?}] grant or replace each subject's privileges (userId of a user, group or API user from users_search; view is added to any other privilege; all false removes the rule); sign asks to approve, execute to carry out, ds to sign legally, and reactionOrders {sign, ds, execute} orders those stages (same number = in parallel). remove [userId] revokes. mode merge (default) leaves everyone not named as is; replace also revokes everyone not named who only views, signs or carries out the object (whoever may modify it, the system owner and public-link guests stay unless remove names them). isNotify tells the subjects; isRecursive false keeps an actor's change off its layers (sim-api cascades by default). Needs modify on the object.",
		`{"type":"object","properties":{`+accessObject+`,"rules":{"type":"array","maxItems":200,"items":{"type":"object","properties":{"userId":{"type":"integer","minimum":1},"privs":`+privs+`,"reactionOrders":`+stageOrders+`},"required":["userId","privs"],"additionalProperties":false}},"remove":{"type":"array","maxItems":200,"items":{"type":"integer","minimum":1}},"mode":{"type":"string","enum":["merge","replace"]},"isNotify":{"type":"boolean"},"isRecursive":{"type":"boolean"},"filterIncorrect":{"type":"boolean"}},"required":["objType","objId"],"additionalProperties":false}`, true,
		func(ctx context.Context, in publicapi.AccessRulesSaveInput) (json.RawMessage, error) {
			return c.SaveAccessRules(ctx, in)
		})
	consoleTool(s, "attachment_upload", "Upload one file (up to "+strconv.Itoa(publicapi.MaxUploadBytes>>10)+" KiB) into the workspace: content is its bytes in standard base64, fileName becomes its title, contentType defaults from the extension. actorId then attaches it to an actor, or to a reaction with rootActorId its actor (needs modify; a refused attach is an error naming the uploaded id); ttl (seconds) makes it temporary. Returns {id, title, type, size, fileName, attachedTo}: id for reaction_create attachmentIds and attachment_attach, fileName for attachment_read. The antivirus scans it before it can be read.",
		`{"type":"object","properties":{"fileName":{"type":"string","minLength":1,"maxLength":255},"content":{"type":"string","minLength":4,"maxLength":`+strconv.Itoa((publicapi.MaxUploadBytes+2)/3*4)+`},"contentType":{"type":"string","maxLength":255},"actorId":{"type":"string","minLength":1,"maxLength":255},"rootActorId":{"type":"string","minLength":1,"maxLength":255},"ttl":{"type":"integer","minimum":1}},"required":["fileName","content"],"additionalProperties":false}`, true, c.UploadAttachment)
	consoleTool(s, "attachment_read", "Read an uploaded file by its fileName (the storage key from attachment_upload, attachments_list or attachments_by_actor_list, not its title): contentType and size, and for a text file (text/*, JSON, XML, YAML, CSV) its text, the first 100 KiB (truncated when longer). A binary file returns only binary true, its type and size (sizeAtLeast when larger). A file still being scanned is retried for a few seconds; a quarantined one is never served.",
		`{"type":"object","properties":{"fileName":{"type":"string","minLength":1,"maxLength":255}},"required":["fileName"],"additionalProperties":false}`, false, c.ReadAttachment)
}

// rawTool is a console tool whose result is the backend's own JSON, passed through like a parity
// tool's.
func rawTool[P any](s *server.MCPServer, name, description, schema string, write bool, run func(context.Context, P) (json.RawMessage, error)) {
	t := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(json.RawMessage(schema)),
		mcp.WithReadOnlyHintAnnotation(!write), mcp.WithDestructiveHintAnnotation(write), mcp.WithIdempotentHintAnnotation(!write))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, e := decodeLimit[P](r, maxConsoleInput)
		if e != nil {
			return failure(e), nil
		}
		v, e := run(ctx, a)
		return passResult(v, e), nil
	})
}
