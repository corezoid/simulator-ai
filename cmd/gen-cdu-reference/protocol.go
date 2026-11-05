package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/corezoid/simulator-ai/internal/cduschema"
)

// namedDefs are written by name where they are used and described in a section of their own.
var namedDefs = []string{"ClientAction", "ResponseAction", "TreeNode", "PageBinding", "JsonPatchOp", "Identifier", "Expression", "RouteDefinition", "RouteParam", "RouteActorLookup"}

// doc collects a document's lines.
type doc struct{ lines []string }

func (d *doc) add(lines ...string) { d.lines = append(d.lines, lines...) }

func (d *doc) bytes() []byte { return []byte(strings.Join(d.lines, "\n") + "\n") }

// renderProtocol writes the page protocol: every key of the stored page config, a tree node and a
// binding, the expression context, each client and response action with its fields, routes and
// definitions. Fields, types and which are required come from page-config.schema.json (what sim-api
// enforces on save); the concept and context text from the help bundle.
func renderProtocol(head string, in *inputs) ([]byte, error) {
	defs, _ := in.pageSchema["$defs"].(map[string]any)
	if defs == nil {
		return nil, fmt.Errorf("page-config.schema.json has no $defs")
	}
	r := cduschema.New(defs, namedDefs...)
	r.Full = true
	def := func(name string) map[string]any { m, _ := defs[name].(map[string]any); return m }
	d := &doc{}
	d.add("# CDU v2 page protocol (schemaVersion "+in.protocol+")", head,
		"Sections: page, node, expressions, bindings, actions, responses, patch, routes, definitions, data, concepts. Read one with smart_form_reference_read {doc: \"protocol/<section>\"}, one action with {doc: \"protocol/form.submit\"}.",
		"Notation: key?: optional, 'a'|'b' enum, A[] array, {[key: string]: T} a map; ClientAction, TreeNode, Identifier, Expression, … are defined in their own section. A field marked Expression takes a literal or {{ JSONata }}.")

	d.add("", "## page — the page config: the file pages/<page>/config")
	d.add(paragraphs(cduschema.Description(in.pageSchema))...)
	d.add(in.helpLines("concept:page")...)
	d.add(fieldLines(r, in.pageSchema, "")...)

	d.add("", "## node — TreeNode, one node of the tree")
	d.add(paragraphs(cduschema.Description(def("TreeNode")))...)
	d.add(fieldLines(r, def("TreeNode"), "")...)
	d.add("Identifier: string " + fmt.Sprint(def("Identifier")["pattern"]) + cduschema.DescribeFull(def("Identifier")))
	d.add(in.helpLines("concept:loop")...)

	d.add("", "## expressions — {{ }} and what they can read")
	d.add(in.helpLines("concept:expression")...)
	d.add("Expression: string" + cduschema.DescribeFull(def("Expression")))
	for _, id := range idsWith(in.help, "ctx:") {
		d.add(strings.TrimPrefix(id, "ctx:") + " — " + strings.Join(in.helpLines(id), " "))
	}

	d.add("", "## bindings — PageBinding, the page's reactions to block events")
	d.add(in.helpLines("concept:binding")...)
	d.add(paragraphs(cduschema.Description(def("PageBinding")))...)
	d.add(fieldLines(r, def("PageBinding"), "")...)

	d.add("", "## actions — ClientAction, the steps of an action chain")
	d.add(in.helpLines("concept:action-chain")...)
	d.add(paragraphs(cduschema.Description(def("ClientAction")))...)
	if err := variantSections(d, r, defs, "ClientAction"); err != nil {
		return nil, err
	}

	d.add("", "## responses — ResponseAction, what a form.submit reply may ask the page to do")
	d.add(in.helpLines("concept:response-action")...)
	d.add(paragraphs(cduschema.Description(def("ResponseAction")))...)
	if err := variantSections(d, r, defs, "ResponseAction"); err != nil {
		return nil, err
	}

	d.add("", "## patch — JsonPatchOp, one op of a response patch")
	d.add(paragraphs(cduschema.Description(def("JsonPatchOp")))...)
	d.add(fieldLines(r, def("JsonPatchOp"), "")...)

	d.add("", "## routes — RouteDefinition, short URLs that open a page")
	d.add(fieldLines(r, def("RouteDefinition"), "")...)
	d.add("RouteParam:")
	d.add(fieldLines(r, def("RouteParam"), "")...)
	d.add("RouteActorLookup: " + unionText(r, def("RouteActorLookup")))
	d.add(in.helpLines("concept:route-state")...)

	d.add("", "## definitions — reusable subtrees, files definitions/<name>")
	d.add(in.helpLines("concept:definition")...)
	d.add(paragraphs(cduschema.Description(def("Definition")))...)

	d.add("", "## data — the viewModel and locale files")
	d.add("ViewModel" + cduschema.DescribeFull(def("ViewModel")))
	d.add("Locale" + cduschema.DescribeFull(def("Locale")))

	d.add("", "## concepts")
	for _, name := range []string{"form", "overlay"} {
		d.add("", strings.TrimSuffix("### "+name+" — "+in.help["concept:"+name].Title, " — "))
		d.add(in.helpLines("concept:" + name)...)
	}
	return d.bytes(), nil
}

// fieldLines renders an object's keys one per line, required first; a key holding an object of its
// own keys is followed by them as <key>.<sub>.
func fieldLines(r *cduschema.Text, schema map[string]any, prefix string) []string {
	props, _ := schema["properties"].(map[string]any)
	required := map[string]bool{}
	list, _ := schema["required"].([]any)
	for _, k := range list {
		if s, ok := k.(string); ok {
			required[s] = true
		}
	}
	keys := cduschema.SortedKeys(props)
	sort.SliceStable(keys, func(i, j int) bool { return required[keys[i]] && !required[keys[j]] })
	var lines []string
	for _, k := range keys {
		if prefix == "" && k == "type" && isConst(props[k]) {
			continue // a variant's discriminator heads its section
		}
		node, _ := props[k].(map[string]any)
		mark := "?"
		if required[k] {
			mark = ""
		}
		sub, _ := node["properties"].(map[string]any)
		if len(sub) > 0 && node["$ref"] == nil {
			lines = append(lines, prefix+k+mark+": object"+cduschema.DescribeFull(node))
			lines = append(lines, fieldLines(r, node, prefix+k+".")...)
			continue
		}
		lines = append(lines, prefix+k+mark+": "+r.Type(node)+cduschema.DescribeFull(node))
	}
	return lines
}

// variantSections writes one ### section per member of a union keyed on type: its title, its
// description and its other fields. A member may point to another (RAction_Notify to Action_Notify):
// its own title and description win, the fields are its target's.
func variantSections(d *doc, r *cduschema.Text, defs map[string]any, union string) error {
	u, _ := defs[union].(map[string]any)
	members, _ := u["oneOf"].([]any)
	if len(members) == 0 {
		return fmt.Errorf("%s has no members", union)
	}
	for _, m := range members {
		var title, description string
		node, _ := m.(map[string]any)
		for hops := 0; node != nil && hops < 4; hops++ {
			if title == "" {
				title, _ = node["title"].(string)
			}
			if description == "" {
				description, _ = node["description"].(string)
			}
			ref, ok := node["$ref"].(string)
			if !ok {
				break
			}
			node, _ = defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		}
		props, _ := node["properties"].(map[string]any)
		typ, _ := props["type"].(map[string]any)
		name, ok := typ["const"].(string)
		if !ok {
			return fmt.Errorf("%s member without a type const", union)
		}
		d.add("", strings.TrimSuffix("### "+name+" — "+title, " — "))
		d.add(paragraphs(description)...)
		d.add(fieldLines(r, node, "")...)
	}
	return nil
}

// unionText is a oneOf of objects as {…} | {…}.
func unionText(r *cduschema.Text, schema map[string]any) string {
	members, _ := schema["oneOf"].([]any)
	parts := make([]string, 0, len(members))
	for _, m := range members {
		mm, _ := m.(map[string]any)
		parts = append(parts, r.Object(mm))
	}
	return strings.Join(parts, " | ")
}

func isConst(node any) bool {
	m, _ := node.(map[string]any)
	_, ok := m["const"]
	return ok
}

func idsWith(entries map[string]helpEntry, prefix string) []string {
	var ids []string
	for id := range entries {
		if strings.HasPrefix(id, prefix) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
