package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type referenceGrepArgs struct {
	Pattern string `json:"pattern"`
	Context *int   `json:"context"`
	Limit   int    `json:"limit"`
	Doc     string `json:"doc"`
}

type referenceReadArgs struct {
	Doc    string `json:"doc"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// cduReference serves the reference documents of one server; components is fetched on first use.
type cduReference struct{ components *componentsCache }

// docs returns the documents a call needs (all for "", else the one name starts with). Without the
// catalog the embedded ones still answer an all-documents call, with a note saying so.
func (ref *cduReference) docs(ctx context.Context, name string) ([]referenceDoc, []string, error) {
	want := func(n string) bool { return name == "" || name == n || strings.HasPrefix(name, n+"/") }
	var docs []referenceDoc
	var notes []string
	if want("components") {
		d, err := ref.components.get(ctx)
		switch {
		case err == nil:
			docs = append(docs, d)
		case name != "":
			return nil, nil, err
		default:
			notes = append(notes, "components: not available ("+err.Error()+")")
		}
	}
	for _, d := range []referenceDoc{protocolDoc, examplesDoc} {
		if want(d.Name) {
			docs = append(docs, d)
		}
	}
	return docs, notes, nil
}

// addSmartFormReference registers smart_form_reference_grep and smart_form_reference_read.
func addSmartFormReference(s *server.MCPServer, c *publicapi.Client) {
	ref := &cduReference{components: &componentsCache{now: time.Now, fetch: func(ctx context.Context) (json.RawMessage, error) {
		return c.Pass(ctx, "cdu/catalog/get", map[string]json.RawMessage{})
	}}}
	addTextTool(s, "smart_form_reference_grep", "Search the CDU v2 reference like grep: a case-insensitive RE2 pattern over the documents components (every catalog type: props, events, state, api; the validator's own catalog), protocol (page config, nodes, {{ }} expressions and their context, bindings, actions, routes) and examples (complete pages). Find a component or prop by idea (gap|spacing, upload, date), an action (form\\.submit) or a context ($locale); {pattern: \"^## \", doc: \"components\", context: 0, limit: 200} lists every type. A hit is doc:line: text, context lines doc-line- text, the section it sits in doc=line= ## Stack (…). doc narrows to one document or doc/<section>. The answer is capped: narrow the pattern when hits are cut; read around a hit with smart_form_reference_read.",
		`{"type":"object","properties":{"pattern":{"type":"string","minLength":1,"maxLength":500},"context":{"type":"integer","minimum":0,"maximum":10,"description":"Lines before and after each hit; default 2"},"limit":{"type":"integer","minimum":1,"maximum":200,"description":"Hits shown; default 40"},"doc":{"type":"string","minLength":1,"maxLength":300,"description":"components, protocol, examples, or doc/<section> (components/Stack)"}},"required":["pattern"],"additionalProperties":false}`,
		func(ctx context.Context, a referenceGrepArgs) (string, error) {
			docs, notes, err := ref.docs(ctx, a.Doc)
			if err != nil {
				return "", err
			}
			windows := make([]docWindow, 0, len(docs))
			for _, d := range docs {
				name := d.Name
				if a.Doc != "" {
					name = a.Doc
				}
				w, err := window(docs, name)
				if err != nil {
					return "", err
				}
				windows = append(windows, w)
			}
			context := 2
			if a.Context != nil {
				context = *a.Context
			}
			return grepReference(windows, notes, a.Pattern, context, a.Limit)
		})
	addTextTool(s, "smart_form_reference_read", "Read the CDU v2 reference by line: doc components, protocol or examples, or one section as doc/<section> (components/DatePicker, protocol/form.submit, examples/E12-conditional-validation). offset (a line number, as grep shows them) and limit (default 120) page through it; the answer has the line numbers, the document's total and the next offset. Without doc: the documents with their sizes and sections.",
		`{"type":"object","properties":{"doc":{"type":"string","minLength":1,"maxLength":300},"offset":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":400}},"additionalProperties":false}`,
		func(ctx context.Context, a referenceReadArgs) (string, error) {
			docs, notes, err := ref.docs(ctx, a.Doc)
			if err != nil {
				return "", err
			}
			if a.Doc == "" {
				return listReference(docs, notes), nil
			}
			w, err := window(docs, a.Doc)
			if err != nil {
				return "", err
			}
			return readReference(w, a.Offset, a.Limit)
		})
}

// addTextTool registers a read-only console tool whose answer is plain text: grep-like lines read
// better, and cost less, than the same text escaped in a JSON string.
func addTextTool[P any](s *server.MCPServer, name, description, schema string, run func(context.Context, P) (string, error)) {
	if s.GetTool(name) != nil {
		panic("console tool registered twice: " + name)
	}
	t := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithRawInputSchema(json.RawMessage(schema)),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true))
	s.AddTool(t, func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, err := decodeLimit[P](r, maxConsoleInput)
		if err != nil {
			return failure(err), nil
		}
		text, err := run(ctx, a)
		if err != nil {
			return failure(err), nil
		}
		return mcp.NewToolResultText(text), nil
	})
}
