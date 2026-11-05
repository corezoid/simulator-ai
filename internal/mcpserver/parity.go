package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// BriefUnlessNarrowed sends brief true when neither types nor kind narrows the answer.
	BriefUnlessNarrowed bool
	// Params, when set, keeps only these params of the method (the legacy tool's own set).
	Params []string
	// RandomColor sends a random editor palette colour when no color is given.
	RandomColor bool
	// Destructive hints that the call cannot be undone or replaces what it touches.
	Destructive bool
	// Idempotent hints that a repeat of a write returns the same result and changes nothing more.
	Idempotent bool
	// Strip drops these keys (a dot path into the result object) from the answer the model sees.
	Strip []string
	// Local are plugin-side params (name: JSON Schema) added to the method's own; the handler hands
	// them to Shape and never sends them.
	Local map[string]json.RawMessage
	// Shape, when set, rewrites the backend's answer with the call's Local arguments.
	Shape func(local map[string]json.RawMessage, v json.RawMessage) (json.RawMessage, error)
}

// rowFields is the compact projection of list rows and graph nodes unless the caller sends fields.
var rowFields = map[string]any{"fields": []string{"id", "title", "formId"}}

// metaFields is the actor projection a write returns unless the caller sends its own fields.
var metaFields = map[string]any{"fields": []string{"id", "title", "description", "formId", "status", "ref", "color"}}

// addParity registers every tool whose method this profile allows, skipping names already taken.
func addParity(s *server.MCPServer, c *publicapi.Client, tools []parityTool) {
	for _, t := range tools {
		if !c.ParityAllowed(t.Method) || s.GetTool(t.Name) != nil {
			continue
		}
		schema, err := contracts.ParityInputSchema(t.Method)
		if err == nil && (len(t.Defaults) > 0 || len(t.Params) > 0) {
			schema, err = narrow(schema, t.Defaults, t.Params)
		}
		if err == nil && len(t.Local) > 0 {
			schema, err = withLocal(schema, t.Local)
		}
		if err != nil {
			panic(err) // pinned build artifact, never remote input
		}
		m := publicapi.ParityMethods[t.Method]
		// manage takes delete items and move takes placements off the source layer.
		destructive := t.Destructive || strings.HasSuffix(t.Method, "/delete") || t.Method == "graph/layer/clean" || t.Method == "graph/layer/actors/manage" || t.Method == "graph/layer/actors/move" || t.Method == "attachment/detach" ||
			t.Method == "access_rule/actor/request/confirm" || t.Method == "access_rule/actor/request/cancel" // grant or turn down another user's access
		tool := mcp.NewTool(t.Name, mcp.WithDescription(t.Description+" Public "+t.Method+"."), mcp.WithRawInputSchema(schema),
			mcp.WithReadOnlyHintAnnotation(!m.Write), mcp.WithDestructiveHintAnnotation(destructive), mcp.WithIdempotentHintAnnotation(!m.Write || t.Idempotent))
		s.AddTool(tool, parityHandler(c, t))
	}
}

func parityHandler(c *publicapi.Client, t parityTool) server.ToolHandlerFunc {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := rawArguments(r)
		if err != nil {
			return failure(err), nil
		}
		local := map[string]json.RawMessage{}
		for k := range t.Local {
			if v, ok := args[k]; ok {
				local[k] = v
				delete(args, k)
			}
		}
		for k, v := range t.Defaults {
			if _, ok := args[k]; !ok {
				args[k], _ = json.Marshal(v)
			}
		}
		if _, set := args["color"]; t.RandomColor && !set {
			args["color"], _ = json.Marshal(publicapi.RandomActorColor())
		}
		if _, set := args["brief"]; t.BriefUnlessNarrowed && !set && args["types"] == nil && args["kind"] == nil {
			args["brief"] = json.RawMessage("true")
		}
		if _, typed := args["edgeTypeId"]; t.Hierarchy && !typed && args["sysLinkTypes"] == nil {
			id, err := c.HierarchyEdgeType(ctx)
			if err != nil {
				return failure(err), nil
			}
			args["edgeTypeId"], _ = json.Marshal(id)
		}
		v, err := c.Pass(ctx, t.Method, args)
		if err == nil && len(t.Strip) > 0 {
			v, err = strip(v, t.Strip)
		}
		if err == nil && t.Shape != nil {
			v, err = t.Shape(local, v)
		}
		return passResult(v, err), nil
	}
}

// strip removes dot-path keys from a result object, keeping every other value as the backend wrote
// it; a path that is not there changes nothing.
func strip(v json.RawMessage, paths []string) (json.RawMessage, error) {
	for _, p := range paths {
		v = stripPath(v, strings.Split(p, "."))
	}
	return v, nil
}

func stripPath(v json.RawMessage, keys []string) json.RawMessage {
	var doc map[string]json.RawMessage
	if json.Unmarshal(v, &doc) != nil {
		return v
	}
	inner, ok := doc[keys[0]]
	switch {
	case !ok:
		return v
	case len(keys) == 1:
		delete(doc, keys[0])
	default:
		doc[keys[0]] = stripPath(inner, keys[1:])
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return v
	}
	return out
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

// withLocal adds the plugin-side params to the method's input schema.
func withLocal(schema json.RawMessage, local map[string]json.RawMessage) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(doc["properties"], &properties); err != nil {
		return nil, err
	}
	for k, v := range local {
		if _, taken := properties[k]; taken {
			return nil, fmt.Errorf("local param %s shadows a method param", k)
		}
		properties[k] = v
	}
	doc["properties"], _ = json.Marshal(properties)
	return json.Marshal(doc)
}

// narrow drops defaulted keys from the schema's required list (the handler fills them) and, with
// params, every property outside that list.
func narrow(schema json.RawMessage, defaults map[string]any, params []string) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	var required []string
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(doc["required"], &required); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(doc["properties"], &properties); err != nil {
		return nil, err
	}
	if len(params) > 0 {
		keep := map[string]json.RawMessage{}
		for _, p := range params {
			if v, ok := properties[p]; ok {
				keep[p] = v
			}
		}
		properties = keep
	}
	kept := []string{}
	for _, r := range required {
		if _, defaulted := defaults[r]; !defaulted {
			if _, ok := properties[r]; !ok {
				return nil, fmt.Errorf("required param %s is not kept", r)
			}
			kept = append(kept, r)
		}
	}
	doc["required"], _ = json.Marshal(kept)
	doc["properties"], _ = json.Marshal(properties)
	return json.Marshal(doc)
}
