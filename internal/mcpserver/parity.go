package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/corezoid/simulator-ai/contracts"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// parityTool is one legacy-parity tool: a single public method, like a legacy Operation is a single
// /papi call. Its input schema is the method's pinned public params; Defaults fill absent arguments
// (a compact projection for writes, so answers stay small unless the caller asks for more).
type parityTool struct {
	Name, Method, Description string
	Defaults                  map[string]any
	// Hierarchy sets edgeTypeId to the hierarchy type when neither it nor sysLinkTypes is sent.
	Hierarchy bool
}

// metaFields is the actor projection a write returns unless the caller sends its own fields.
var metaFields = map[string]any{"fields": []string{"id", "title", "description", "formId", "status", "ref", "color"}}

// addParity registers every tool whose method this profile allows, skipping names already taken.
func addParity(s *server.MCPServer, c *publicapi.Client, tools []parityTool) {
	for _, t := range tools {
		if !c.ParityAllowed(t.Method) || s.GetTool(t.Name) != nil {
			continue
		}
		schema, err := contracts.ParityInputSchema(t.Method)
		if err == nil && len(t.Defaults) > 0 {
			schema, err = optional(schema, t.Defaults)
		}
		if err != nil {
			panic(err) // pinned build artifact, never remote input
		}
		m := publicapi.ParityMethods[t.Method]
		destructive := strings.HasSuffix(t.Method, "/delete") || t.Method == "graph/layer/clean"
		tool := mcp.NewTool(t.Name, mcp.WithDescription(t.Description+" Public "+t.Method+"."), mcp.WithRawInputSchema(schema),
			mcp.WithReadOnlyHintAnnotation(!m.Write), mcp.WithDestructiveHintAnnotation(destructive), mcp.WithIdempotentHintAnnotation(!m.Write))
		s.AddTool(tool, parityHandler(c, t))
	}
}

func parityHandler(c *publicapi.Client, t parityTool) server.ToolHandlerFunc {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := rawArguments(r)
		if err != nil {
			return failure(err), nil
		}
		for k, v := range t.Defaults {
			if _, ok := args[k]; !ok {
				args[k], _ = json.Marshal(v)
			}
		}
		if _, typed := args["edgeTypeId"]; t.Hierarchy && !typed && args["sysLinkTypes"] == nil {
			id, err := c.HierarchyEdgeType(ctx)
			if err != nil {
				return failure(err), nil
			}
			args["edgeTypeId"], _ = json.Marshal(id)
		}
		v, err := c.Pass(ctx, t.Method, args)
		return passResult(v, err), nil
	}
}

// rawArguments returns the tool arguments as raw values, bounded like every console call.
func rawArguments(r mcp.CallToolRequest) (map[string]json.RawMessage, error) {
	b, err := json.Marshal(r.GetRawArguments())
	if err != nil || len(b) > maxConsoleInput {
		return nil, errors.New("invalid or oversized tool arguments")
	}
	args := map[string]json.RawMessage{}
	if len(b) > 0 && string(b) != "null" {
		if err = json.Unmarshal(b, &args); err != nil {
			return nil, errors.New("tool arguments must be an object")
		}
	}
	return args, nil
}

// passResult returns the backend's result as structured content; a non-object result is wrapped.
func passResult(v json.RawMessage, err error) *mcp.CallToolResult {
	if err != nil {
		return failure(err)
	}
	if len(v) == 0 || v[0] != '{' {
		return mcp.NewToolResultStructuredOnly(map[string]json.RawMessage{"result": v})
	}
	return mcp.NewToolResultStructuredOnly(v)
}

// optional drops defaulted keys from the schema's required list: the handler fills them.
func optional(schema json.RawMessage, defaults map[string]any) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	var required []string
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(doc["required"], &required); err != nil {
		return nil, err
	}
	kept := []string{}
	for _, r := range required {
		if _, ok := defaults[r]; !ok {
			kept = append(kept, r)
		}
	}
	doc["required"], _ = json.Marshal(kept)
	return json.Marshal(doc)
}
