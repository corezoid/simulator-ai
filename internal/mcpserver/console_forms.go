package mcpserver

import (
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// formFieldOp is one op of form_fields_patch; which keys an op takes is checked by the handler, so
// the schema stays one flat object.
const formFieldOp = `{"type":"object","properties":{"op":{"type":"string","enum":["add","update","remove","move","section_add","section_update","section_remove"]},"id":{"type":"string","minLength":1,"maxLength":255},"field":{"type":"object"},"set":{"type":"object","minProperties":1},"unset":{"type":"array","items":{"type":"string"},"maxItems":20},"section":{"type":["integer","string"]},"before":{"type":"string","minLength":1,"maxLength":255},"after":{"type":"string","minLength":1,"maxLength":255},"title":{"type":"string","maxLength":1000},"at":{"type":"integer","minimum":0}},"required":["op"],"additionalProperties":false}`

// addConsoleForms registers the account template tools a single public method does not cover.
func addConsoleForms(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "form_fields_patch", "Change an account template field by field instead of resending sections to form_update: each op names a field by its id (its key in actor data). ops in order, all or none: add {field: {class, title, id?, type?, options?, required?, ...}, section? (index or title), before?|after? (a field id)} (no id: one is generated; default: end of the last section); update {id, set: {property: value}, unset?: [property]}; remove {id}; move {id, section?|before?|after?}; section_add {title, at?}; section_update {section, title}; section_remove {section} (only empty). form patches the template's own title, description, color, ref, settings, tags, parentId (null clears). expectedEditToken (editToken from form_get with editor, or from the last patch) refuses a template someone changed since; without it the ops apply to the current template and change only the fields they name. A refused value names the property and what it takes. Returns {formId, editToken, changed [{op, id, section, index}], sections [{title, fields}]}.",
		`{"type":"object","properties":{"formId":{"type":"integer","minimum":1},"expectedEditToken":{"type":"string","minLength":1,"maxLength":200},"ops":{"type":"array","maxItems":100,"items":`+formFieldOp+`},"form":{"type":"object","properties":{"title":{"type":"string","minLength":1},"description":{"type":["string","null"]},"color":{"type":["string","null"]},"ref":{"type":["string","null"]},"settings":{"type":"object"},"tags":{"type":"array"},"parentId":{"type":["integer","null"]}},"additionalProperties":false}},"required":["formId"],"additionalProperties":false}`, true, c.PatchFormFields)
}
