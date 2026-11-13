package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const collapseGetDescription = "Read the user's own collapse of a layer — what their Graph Editor folds: folds (actors collapsed whole, " +
	"or by condition: only the direct children the condition matches fold) with title, hiddenCount (placements folded into it), " +
	"childCount/matchedChildCount and visible (false: another fold hides it); fans (actors with more direct children than the layer's " +
	"maxVisibleChildren, their hiddenCount, opened: the user opened it whole); hiddenTotal. A fold whose actor left the layer is gone. " +
	"laId (a fold or a fan's parent) adds under: one page of the placements hidden under it (limit/offset; q searches a fan's by title). " +
	"owner \"apiUser\": an API key reads the API user's own view, not a person's. Personal: nobody else's view. Skill simulator-ai-graph, Collapse."

const collapseCountDescription = "Preview a fold by condition: how many direct children of laId the condition matches (count) of all " +
	"of them (childCount), before layer_collapse_update folds them. condition: formId (the template of the children), q (field terms " +
	"key=value joined by & and, groups by || or; = != < <= > >=), optional balance bounds (accountNameId + currencyId, amountFrom/amountTo, " +
	"incomeType). A child the user cannot view never matches. capped: only the first part of the children was read."

const collapseUpdateDescription = "Change the user's own collapse of a layer (their Graph Editor view only: nothing moves, nobody else " +
	"sees it). collapse: fold actors (laId) whole — everything below them hides; conditional [{laId, condition, summary?}]: fold only the " +
	"direct children the condition matches (preview with layer_collapse_count); expand / expandAll: unfold; fanOpen / fanClose / fanReset: " +
	"open a wide fan whole or fold it again. Expansions apply first, so one call replaces a fold. Returns the whole new state; changed " +
	"false = it already was so; openTabs: the user's open views of the layer that updated at once (0: they see it when they open it). " +
	"Refused with an API key (it would fold the API user's view). The fan limit itself is layer_settings_update maxVisibleChildren."

// collapseOutput is written by hand: the reflected one carries every nested type in full.
const collapseOutput = `{"type":"object","properties":{"layerId":{"type":"string"},"owner":{"type":"string","enum":["you","apiUser"]},"maxVisibleChildren":{"type":"integer"},"folds":{"type":"array","items":{"type":"object"}},"fans":{"type":"array","items":{"type":"object"}},"fanShown":{"type":"array"},"hiddenTotal":{"type":"integer"},"truncated":{"type":"boolean"},"under":{"type":"object"}},"required":["layerId","owner","maxVisibleChildren","folds","fans","hiddenTotal"]}`

// addLayerCollapse registers the collapse tools: the two reads wherever graph reads are, the
// person's own write in the console.
func addLayerCollapse(s *server.MCPServer, c *publicapi.Client) {
	get, count, update, err := contracts.CollapseSchemas()
	if err != nil {
		panic(err)
	}
	readOnly := []mcp.ToolOption{mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true)}
	s.AddTool(mcp.NewTool("layer_collapse_get", append([]mcp.ToolOption{mcp.WithDescription(collapseGetDescription), mcp.WithRawInputSchema(get),
		mcp.WithRawOutputSchema(json.RawMessage(collapseOutput))}, readOnly...)...), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		in, err := decode[publicapi.CollapseGetInput](r)
		if err != nil {
			return failure(err), nil
		}
		v, err := c.GetLayerCollapse(ctx, in)
		return result(v, err), nil
	})
	s.AddTool(mcp.NewTool("layer_collapse_count", append([]mcp.ToolOption{mcp.WithDescription(collapseCountDescription), mcp.WithRawInputSchema(count),
		mcp.WithOutputSchema[publicapi.CollapseCount]()}, readOnly...)...), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		in, err := decodeLimit[publicapi.CollapseCountInput](r, 32<<10)
		if err != nil {
			return failure(err), nil
		}
		v, err := c.CountLayerCollapse(ctx, in)
		return result(v, err), nil
	})
	if !c.ConsoleEnabled() {
		return
	}
	// A personal write: idempotent (the same change twice is changed: false), not destructive.
	s.AddTool(mcp.NewTool("layer_collapse_update", mcp.WithDescription(collapseUpdateDescription), mcp.WithRawInputSchema(update),
		mcp.WithOutputSchema[publicapi.CollapseUpdated](), mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true)), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		in, err := decodeLimit[publicapi.CollapseUpdateInput](r, 256<<10)
		if err != nil {
			return failure(err), nil
		}
		v, err := c.UpdateLayerCollapse(ctx, in)
		return result(v, err), nil
	})
}
