package contracts

import (
	"encoding/json"
	"errors"
)

// Project output schemas with the same allow-lists as publicapi's named result DTOs.
func graphOutput(method string, raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if method == "form/system/get" {
		if err := retainGraphProperties(schema, []string{"id", "title", "type", "form"}); err != nil {
			return nil, err
		}
		schema["required"] = []string{"id", "title", "type", "form"}
	} else {
		fields := []string{"list"}
		if method == "form/flowchart-block/search" {
			fields = append(fields, "hasMore")
		}
		if err := retainGraphProperties(schema, fields); err != nil {
			return nil, err
		}
		schema["required"] = fields
		props := schema["properties"].(map[string]any)
		list, ok := props["list"].(map[string]any)
		if !ok {
			return nil, errors.New("invalid graph list schema")
		}
		item, ok := list["items"].(map[string]any)
		if !ok {
			return nil, errors.New("invalid graph item schema")
		}
		if method == "edge_type/list" {
			fields = []string{"id", "name", "accId", "userId", "isSystem", "createdAt", "isTree"}
			if err := retainGraphProperties(item, fields); err != nil {
				return nil, err
			}
			// CSV selection can omit every property. The typed client validates full
			// identity before projection and normalizes the legacy nullable creator.
			item["required"] = []string{}
			creator := item["properties"].(map[string]any)["userId"].(map[string]any)
			creator["type"] = []string{"integer", "null"}
		} else {
			if err := retainGraphProperties(item, []string{"id", "parentId", "title", "blockId", "collection", "quickBlock", "updatedAt", "form"}); err != nil {
				return nil, err
			}
			item["required"] = []string{"id", "parentId", "title", "blockId", "updatedAt", "form"}
		}
	}
	return json.Marshal(schema)
}

func retainGraphProperties(schema map[string]any, names []string) error {
	properties, ok := schema["properties"].(map[string]any)
	if !ok || schema["type"] != "object" {
		return errors.New("invalid graph object schema")
	}
	selected := map[string]any{}
	for _, name := range names {
		value, ok := properties[name]
		if !ok {
			return errors.New("missing graph projection property")
		}
		selected[name] = value
	}
	schema["properties"] = selected
	schema["additionalProperties"] = false
	return nil
}
