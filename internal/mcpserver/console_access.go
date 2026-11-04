package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	accessObject = `"objType":{"type":"string","enum":["actor","formTemplate","templateActors","account"]},"objId":{"type":"string","minLength":1,"maxLength":255},"currencyId":{"type":"integer","minimum":1}`
	privs        = `{"type":"object","properties":{"view":{"type":"boolean"},"modify":{"type":"boolean"},"remove":{"type":"boolean"},"sign":{"type":"boolean"},"ds":{"type":"boolean"},"execute":{"type":"boolean"}},"additionalProperties":false}`
	stageOrders  = `{"type":"object","properties":{"sign":{"type":"integer","minimum":1},"ds":{"type":"integer","minimum":1},"execute":{"type":"integer","minimum":1}},"additionalProperties":false}`
)

// addConsoleAccess registers one access tool per action across every object kind; their results
// are the backend's own JSON.
func addConsoleAccess(s *server.MCPServer, c *publicapi.Client) {
	addConsoleTool(s, rawOutput, "access_rules_get", "List who has access to an object and with which privileges (view, modify, remove, sign, ds, execute) and stage orders. objType actor (objId the actor id), formTemplate (the template itself, objId its id), templateActors (the default access to the template's actors) or account (objId the account name id, with currencyId).",
		`{"type":"object","properties":{`+accessObject+`},"required":["objType","objId"],"additionalProperties":false}`, false, c.AccessRules)
	addConsoleTool(s, rawOutput, "access_rules_save", "Share or unshare an object in one call. objType/objId as in access_rules_get. rules [{userId, privs, reactionOrders?}] grant or replace each subject's privileges (userId of a user, group or API user from users_search; view is added to any other privilege; all false removes the rule); sign asks to approve, execute to carry out, ds to sign legally, and reactionOrders {sign, ds, execute} orders those stages (same number = in parallel). remove [userId] revokes. mode merge (default) leaves everyone not named as is; replace also revokes everyone not named who views, signs, signs legally or carries out the object (whoever may modify or remove it, the system owner and public-link guests stay unless remove names them), and with nothing to change returns the rules as they are. A listed subject that no longer exists fails the call: repeat it with filterIncorrect true, which also skips unknown userIds of rules. isNotify tells the subjects; isRecursive false keeps an actor's change off its layers (sim-api cascades by default). Needs modify on the object.",
		`{"type":"object","properties":{`+accessObject+`,"rules":{"type":"array","maxItems":200,"items":{"type":"object","properties":{"userId":{"type":"integer","minimum":1},"privs":`+privs+`,"reactionOrders":`+stageOrders+`},"required":["userId","privs"],"additionalProperties":false}},"remove":{"type":"array","maxItems":200,"items":{"type":"integer","minimum":1}},"mode":{"type":"string","enum":["merge","replace"]},"isNotify":{"type":"boolean"},"isRecursive":{"type":"boolean"},"filterIncorrect":{"type":"boolean"}},"required":["objType","objId"],"additionalProperties":false}`, true, c.SaveAccessRules)
}

// rawOutput declares no output schema: the result is the backend's own JSON, passed through like a
// parity tool's.
var rawOutput mcp.ToolOption = func(*mcp.Tool) {}
