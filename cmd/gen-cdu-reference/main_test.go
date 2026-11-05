package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testPageSchema = `{"description":"Stored page configs.","required":["schemaVersion","id","tree"],"properties":{
"schemaVersion":{"type":"string","description":"Author '2.0'."},"id":{"$ref":"#/$defs/Identifier","description":"Page id."},"tree":{"$ref":"#/$defs/TreeNode"},
"meta":{"type":"object","properties":{"tags":{"type":"array","items":{"type":"string"},"description":"Labels."}},"description":"Settings."}},
"$defs":{"Identifier":{"type":"string","pattern":"^[A-Za-z_]+$","description":"An identifier."},"Expression":{"type":"string","description":"A {{ }} string."},
"TreeNode":{"type":"object","description":"One UI node.","allOf":[{"comment":"$ref excludes props.","if":{"required":["$ref"]}}],"properties":{"$ref":{"type":"string","pattern":"^#/definitions/[a-z]+$","description":"A definition."},"id":{"$ref":"#/$defs/Identifier"},"actions":{"type":"object","additionalProperties":{"type":"array","items":{"$ref":"#/$defs/ClientAction"}}}}},
"PageBinding":{"type":"object","required":["on"],"properties":{"on":{"type":"string"}}},
"ClientAction":{"description":"One step.","oneOf":[{"$ref":"#/$defs/Action_FormSubmit"}]},
"Action_FormSubmit":{"type":"object","required":["type","formId"],"title":"Submit a form","description":"Sends a form.\n\nIt does not validate.","properties":{"type":{"const":"form.submit"},"formId":{"$ref":"#/$defs/Identifier","description":"The Form node's id."},"onSuccess":{"type":"array","items":{"$ref":"#/$defs/ClientAction"}}}},
"ResponseAction":{"oneOf":[{"$ref":"#/$defs/RAction_Notify"}]},"RAction_Notify":{"$ref":"#/$defs/Action_Notify","title":"Show a toast","description":"As sent."},
"Action_Notify":{"type":"object","properties":{"type":{"const":"notify"},"title":{"type":"string"}}},
"JsonPatchOp":{"type":"object","properties":{"op":{"enum":["add","remove"]}}},"RouteDefinition":{"type":"object","properties":{"name":{"type":"string"}}},
"RouteParam":{"type":"object","properties":{"type":{"enum":["slug"]}}},"RouteActorLookup":{"oneOf":[{"type":"object","properties":{"formId":{"type":"integer"}}}]},
"Definition":{"description":"Reusable subtree."},"ViewModel":{"description":"Data."},"Locale":{"description":"Texts."}}}`

const testHelp = `{"entries":{"ctx:vm":{"summary":"The page's data."},"ctx:$locale":{"summary":"The texts."},"concept:page":{"summary":"A page.","body":"` + "```text\\nSmart Form\\n└─ pages\\n```" + `"},
"concept:form":{"title":"Form","summary":"A Form keeps values."},"type:Stack":{"summary":"Lays its children out."}}}`

const testManifest = `{"types":{"Stack":{"kind":"primitive","editor":{"slots":[{"name":"default"}]}},"Row":{"kind":"primitive","editor":{"slots":[{"name":"default","allow":["Col"]}]}},
"ActorList":{"kind":"block","editor":{"examples":[{"name":"Graphs","description":"List graphs.","props":{"formTitle": "Graphs"}}]}}}}`

func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, body := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "x"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	return dir
}

func testSources(t *testing.T) (source, source) {
	t.Helper()
	kitFiles := map[string]string{"packages/cdu-catalog/cdu-help.json": testHelp}
	for _, id := range exampleIDs {
		page := `{"schemaVersion":"2.0","pageId":"` + id + `","title":"T","viewModel":{"rows":[]},"locale":{"en":{"a":"A"}},"tree":{"id":"root","type":"Stack","children":[]},"routeState":{}}`
		path := "docs/cdu-examples/pages/" + id + ".page.json"
		if strings.HasPrefix(id, "R") {
			page, path = `{"schemaVersion":"2.0","id":"orders","tree":{"id":"root","type":"Stack"},"routes":[{"name":"orders","path":"/orders"}]}`, "docs/cdu-examples/configs/"+id+".config.json"
		}
		kitFiles[path] = page
		kitFiles["docs/cdu-examples/"+id+".md"] = "# " + id + " — Example\n\n**Goal.** Show it\nwell.\n\n## Mechanisms\n\n- **One** — first\n  line two.\n- **Two**\n\n## Page\n\n- not a mechanism\n"
	}
	sim := source{"sim-api", gitRepo(t, map[string]string{
		"internal/cdu/catalog/supported.json":                        `{"supportedVersions":["2.0"]}`,
		"internal/cdu/catalog/contracts/2.0/page-config.schema.json": testPageSchema,
		"internal/cdu/catalog/contracts/2.0/catalog.manifest.json":   testManifest,
	}), "HEAD"}
	kit := source{"simulator-ui-kit", gitRepo(t, kitFiles), "HEAD"}
	var err error
	if sim.rev, err = resolve(sim); err != nil {
		t.Fatal(err)
	}
	if kit.rev, err = resolve(kit); err != nil {
		t.Fatal(err)
	}
	return sim, kit
}

func TestGenerate(t *testing.T) {
	sim, kit := testSources(t)
	files, err := generate(sim, kit)
	if err != nil {
		t.Fatal(err)
	}
	protocol, examples, types := string(files["protocol.md"]), string(files["examples.md"]), string(files["types.json"])
	for _, want := range []string{
		"# CDU v2 page protocol (schemaVersion 2.0)\nGenerated by cmd/gen-cdu-reference from sim-api " + sim.rev + " and simulator-ui-kit " + kit.rev,
		"Smart Form\n└─ pages\n", // a fenced drawing keeps its lines
		"id: Identifier — Page id.\nschemaVersion: string — Author '2.0'.\ntree: TreeNode\nmeta?: object — Settings.\nmeta.tags?: string[] — Labels.",
		"actions?: {[key: string]: ClientAction[]}",
		"### form.submit — Submit a form\nSends a form.\nIt does not validate.\nformId: Identifier — The Form node's id.\nonSuccess?: ClientAction[]\n",
		"### notify — Show a toast\nAs sent.\ntitle?: string",
		"$ref: string ^#/definitions/[a-z]+$ — A definition.\nContract: $ref excludes props.\nRules (checked on a stand", `{"id":"field-rows","$ref":"#/definitions/field-row"`, "$locale — The texts.\nvm — The page's data.", "### form — Form",
	} {
		if !strings.Contains(protocol, want) {
			t.Errorf("protocol lacks %q:\n%s", want, protocol)
		}
	}
	for _, want := range []string{
		"## E12-conditional-validation — Example\nGoal: Show it well.\nMechanisms:\n- **One** — first line two.\n- **Two**\npages/E12-conditional-validation/config:\n{\n  \"schemaVersion\": \"2.0\",\n  \"id\": \"E12-conditional-validation\",\n  \"title\": \"T\",\n  \"tree\": {",
		"pages/E12-conditional-validation/viewModel:\n{\n  \"rows\": []\n}\npages/E12-conditional-validation/locale:",
		"pages/<page>/config:\n{\n  \"schemaVersion\": \"2.0\",\n  \"id\": \"orders\",",
	} {
		if !strings.Contains(examples, want) {
			t.Errorf("examples lack %q:\n%s", want, examples)
		}
	}
	if strings.Contains(examples, "routeState") || strings.Contains(examples, "not a mechanism") {
		t.Error("render output and other sections stay out of the example")
	}
	for _, want := range []string{`"purpose": "Lays its children out."`, `"children": "default — allows Col"`, `"children": "none (a leaf: children are ignored)"`, `"formTitle": "Graphs"`} {
		if !strings.Contains(types, want) {
			t.Errorf("types.json lacks %s:\n%s", want, types)
		}
	}
}

// -check regenerates at the recorded commits: equal passes, an edit is drift, a missing checkout skips.
func TestCheckDrift(t *testing.T) {
	sim, kit := testSources(t)
	files, err := generate(sim, kit)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	for name, body := range files {
		if err = os.WriteFile(filepath.Join(out, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moved := source{sim.name, sim.dir, "no-such-rev"} // the commit comes from the header, not the flag
	if err = checkDrift(moved, kit, out); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "examples.md"), append(files["examples.md"], 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = checkDrift(sim, kit, out); err == nil || !strings.Contains(err.Error(), "drift in examples.md") {
		t.Fatal("drift not detected:", err)
	}
	if err = checkDrift(source{"sim-api", t.TempDir(), ""}, kit, out); err != nil {
		t.Fatal("a missing checkout skips the check:", err)
	}
}

// The committed documents are generated ones (their regeneration is make gen-check).
func TestCommittedHeader(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("../..", outDir, "protocol.md"))
	if err != nil {
		t.Fatal(err)
	}
	if headerRevs.FindSubmatch(b) == nil {
		t.Fatal("protocol.md has no source header")
	}
}
