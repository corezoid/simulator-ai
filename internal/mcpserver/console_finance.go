package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleFinance registers the finance tool a single public method does not cover.
func addConsoleFinance(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "account_pair_create", "Find or register a workspace account pair by NAME, before opening it on actors (account_create) or recording values: the account name and the currency are looked up and only the missing ones created (symbol, precision, type only for a new currency; creating needs the accounts:management permission). A new pair is registered with you as its owner (registered true). An existing pair that is shared stays as it is: your access is what its rules give (to post you need modify), and one you may not see is refused. An existing pair nobody was given access to is refused unless takeOwnership is true, which makes you its only subject and hides its balances from everyone else: ask the user first. Returns {nameId, currencyId, registered}.",
		`{"type":"object","properties":{"accountName":{"type":"string","minLength":1,"maxLength":255},"currencyName":{"type":"string","minLength":1,"maxLength":255},"symbol":{"type":"string","maxLength":32},"precision":{"type":"integer","minimum":0,"maximum":18},"type":{"type":"string","enum":["number","percent","currency","dateTime","seconds"]},"takeOwnership":{"type":"boolean"}},"required":["accountName","currencyName"],"additionalProperties":false}`, true, c.CreateAccountPair)
}
