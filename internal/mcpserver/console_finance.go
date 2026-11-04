package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleFinance registers the finance tool a single public method does not cover.
func addConsoleFinance(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "account_pair_create", "Register a workspace account pair by NAME, before opening it on actors (account_create) or recording values: creates the account name and the currency when missing (symbol, precision, type only for a new currency) and registers the pair, which makes you its owner. Safe to repeat. Returns {nameId, currencyId}. If the pair is already shared and you are not among its subjects it is refused: an owner must share it (access_rules_save objType account).",
		`{"type":"object","properties":{"accountName":{"type":"string","minLength":1,"maxLength":255},"currencyName":{"type":"string","minLength":1,"maxLength":255},"symbol":{"type":"string","maxLength":32},"precision":{"type":"integer","minimum":0,"maximum":18},"type":{"type":"string","enum":["number","percent","currency","dateTime","seconds"]}},"required":["accountName","currencyName"],"additionalProperties":false}`, true, c.CreateAccountPair)
}
