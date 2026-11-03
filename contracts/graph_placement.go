package contracts

// The backend's ActorLayerSettings has open Extra fields. blockId/shape are authored by
// UI graph-layer flowchartCommands and decoded by geometry/flowchartShape.ts (2e73d8c1).
// Keep the MCP subset explicit; no arbitrary settings proxy or delete action is exposed.
func narrowPlacementSchema(array map[string]any) error {
	array["minItems"], array["maxItems"] = 1, 100
	item := array["items"].(map[string]any)
	if err := retainGraphProperties(item, []string{"action", "data"}); err != nil {
		return err
	}
	props := item["properties"].(map[string]any)
	props["action"].(map[string]any)["enum"] = []string{"create"}
	item["required"] = []string{"action", "data"}
	data := props["data"].(map[string]any)
	if err := retainGraphProperties(data, []string{"id", "type", "laIdSource", "laIdTarget", "position", "areaPicture", "layerSettings"}); err != nil {
		return err
	}
	p := data["properties"].(map[string]any)
	for _, name := range []string{"laIdSource", "laIdTarget"} {
		p[name].(map[string]any)["minimum"] = 1
	}
	p["id"].(map[string]any)["maxLength"] = 255
	pos := p["position"].(map[string]any)
	pos["required"] = []string{"x", "y"}
	pos["additionalProperties"] = false
	picture := p["areaPicture"].(map[string]any)
	picture["additionalProperties"] = false
	for _, name := range []string{"width", "height"} {
		picture["properties"].(map[string]any)[name].(map[string]any)["exclusiveMinimum"] = 0
	}
	settings := p["layerSettings"].(map[string]any)
	if err := retainGraphProperties(settings, []string{"width", "height", "textFrame"}); err != nil {
		return err
	}
	settingsProps := settings["properties"].(map[string]any)
	for _, name := range []string{"width", "height"} {
		settingsProps[name].(map[string]any)["minimum"] = 1
	}
	settingsProps["blockId"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
	// shape is the frontend-managed native definition; the backend persists it unchanged.
	settingsProps["shape"] = map[string]any{"type": "object"}
	settings["required"] = []string{"width", "height", "blockId", "shape", "textFrame"}
	frame := settingsProps["textFrame"].(map[string]any)
	frame["properties"] = map[string]any{"x": map[string]any{"type": "number"}, "y": map[string]any{"type": "number"}, "w": map[string]any{"type": "number", "minimum": 0}, "h": map[string]any{"type": "number", "minimum": 0}}
	frame["required"] = []string{"x", "y", "w", "h"}
	frame["additionalProperties"] = false
	// Edge placements always need both exact anchors; node placements always need a position.
	data["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"type": map[string]any{"const": "node"}}, "required": []string{"id", "type", "position"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"laIdSource"}}, map[string]any{"required": []string{"laIdTarget"}}}}},
		map[string]any{"properties": map[string]any{"type": map[string]any{"const": "edge"}}, "required": []string{"id", "type", "laIdSource", "laIdTarget"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"position"}}, map[string]any{"required": []string{"areaPicture"}}, map[string]any{"required": []string{"layerSettings"}}}}},
	}
	return nil
}
