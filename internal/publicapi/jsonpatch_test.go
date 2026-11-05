package publicapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// patchDoc applies ops (a JSON array) to src and returns the compact result or the error text.
func patchDoc(t *testing.T, src, ops string) (string, string) {
	t.Helper()
	var list []PatchOp
	if err := json.Unmarshal([]byte(ops), &list); err != nil {
		t.Fatal(err)
	}
	out, _, _, err := patchSource(src, "", list)
	if err != nil {
		return "", err.Error()
	}
	return out, ""
}

func TestApplyPatch(t *testing.T) {
	const src = `{"b":1,"a":{"x":[1,2,3],"s/l":"~"}}`
	cases := []struct{ name, ops, want, err string }{
		{"add a null value", `[{"op":"add","path":"/c","value":null}]`, `{"b":1,"a":{"x":[1,2,3],"s/l":"~"},"c":null}`, ""},
		{"add a new key keeps order", `[{"op":"add","path":"/c","value":true}]`, `{"b":1,"a":{"x":[1,2,3],"s/l":"~"},"c":true}`, ""},
		{"add over a key replaces in place", `[{"op":"add","path":"/b","value":{"k":null}}]`, `{"b":{"k":null},"a":{"x":[1,2,3],"s/l":"~"}}`, ""},
		{"add inserts at an index", `[{"op":"add","path":"/a/x/1","value":9}]`, `{"b":1,"a":{"x":[1,9,2,3],"s/l":"~"}}`, ""},
		{"add appends with -", `[{"op":"add","path":"/a/x/-","value":4}]`, `{"b":1,"a":{"x":[1,2,3,4],"s/l":"~"}}`, ""},
		{"add at the end index", `[{"op":"add","path":"/a/x/3","value":4}]`, `{"b":1,"a":{"x":[1,2,3,4],"s/l":"~"}}`, ""},
		{"replace an escaped key", `[{"op":"replace","path":"/a/s~1l","value":"~0"}]`, `{"b":1,"a":{"x":[1,2,3],"s/l":"~0"}}`, ""},
		{"remove an array item", `[{"op":"remove","path":"/a/x/0"}]`, `{"b":1,"a":{"x":[2,3],"s/l":"~"}}`, ""},
		{"remove a key", `[{"op":"remove","path":"/b"}]`, `{"a":{"x":[1,2,3],"s/l":"~"}}`, ""},
		{"move within an array", `[{"op":"move","from":"/a/x/0","path":"/a/x/-"}]`, `{"b":1,"a":{"x":[2,3,1],"s/l":"~"}}`, ""},
		{"move a key", `[{"op":"move","from":"/b","path":"/a/b"}]`, `{"a":{"x":[1,2,3],"s/l":"~","b":1}}`, ""},
		{"copy is deep", `[{"op":"copy","from":"/a/x","path":"/y"},{"op":"add","path":"/y/0","value":0}]`, `{"b":1,"a":{"x":[1,2,3],"s/l":"~"},"y":[0,1,2,3]}`, ""},
		{"test then replace", `[{"op":"test","path":"/a","value":{"s/l":"~","x":[1.0,2,3]}},{"op":"replace","path":"/b","value":2}]`, `{"b":2,"a":{"x":[1,2,3],"s/l":"~"}}`, ""},
		{"replace the whole document", `[{"op":"replace","path":"","value":[1]}]`, `[1]`, ""},
		{"failed test stops it", `[{"op":"replace","path":"/b","value":2},{"op":"test","path":"/b","value":1}]`, "", "op 1 (test /b): test failed"},
		{"replace of a missing key", `[{"op":"replace","path":"/a/gap","value":1}]`, "", `op 0 (replace /a/gap): /a has no key "gap"`},
		{"index out of range", `[{"op":"remove","path":"/a/x/3"}]`, "", "index 3 is out of range (the array has 3 items)"},
		{"leading zero is no index", `[{"op":"add","path":"/a/x/01","value":1}]`, "", `"01" is not an array index`},
		{"pointer without slash", `[{"op":"add","path":"a","value":1}]`, "", `pointer "a" must start with /`},
		{"bad escape", `[{"op":"add","path":"/a~2","value":1}]`, "", "~ must be ~0 or ~1"},
		{"through a scalar", `[{"op":"add","path":"/b/c","value":1}]`, "", "/b is not an object or array"},
		{"value required", `[{"op":"add","path":"/c"}]`, "", "value required"},
		{"move into itself", `[{"op":"move","from":"/a","path":"/a/x/0"}]`, "", "cannot move a value into itself"},
		{"unknown op", `[{"op":"merge","path":"/a"}]`, "", `unknown op "merge"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := patchDoc(t, src, tc.ops)
			if tc.err != "" {
				if !strings.Contains(err, tc.err) {
					t.Fatalf("error %q, want %q", err, tc.err)
				}
				return
			}
			if err != "" || got != tc.want {
				t.Fatalf("got %s (%s), want %s", got, err, tc.want)
			}
		})
	}
}

// A patched source keeps its key order, indentation, big numbers and unescaped expressions.
func TestPatchSourceKeepsFormat(t *testing.T) {
	src := "{\n    \"z\": 12345678901234567890,\n    \"tree\": {\n        \"id\": \"root\",\n        \"type\": \"Stack\",\n        \"props\": {\n            \"title\": \"{{ a < b && c }}\"\n        }\n    }\n}"
	out, applied, changed, err := patchSource(src, "", []PatchOp{{Op: "add", Path: "/tree/props/size", Value: json.RawMessage(`"xl"`)}})
	if err != nil || !changed {
		t.Fatal(err)
	}
	want := "{\n    \"z\": 12345678901234567890,\n    \"tree\": {\n        \"id\": \"root\",\n        \"type\": \"Stack\",\n        \"props\": {\n            \"title\": \"{{ a < b && c }}\",\n            \"size\": \"xl\"\n        }\n    }\n}"
	if out != want || strings.Join(applied, ";") != "add /tree/props/size" {
		t.Fatalf("got\n%s\n%v", out, applied)
	}
	if out, _, _, _ = patchSource(`{"a":1}`, "", []PatchOp{{Op: "replace", Path: "/a", Value: json.RawMessage(`2`)}}); out != `{"a":2}` {
		t.Fatal("a one-line source stays one line:", out)
	}
	if _, _, changed, _ = patchSource(`{"a":1}`, "", []PatchOp{{Op: "replace", Path: "/a", Value: json.RawMessage(`1.0`)}}); changed {
		t.Fatal("an equal value is no change")
	}
	if _, _, _, err = patchSource("@color: red;", "", []PatchOp{{Op: "remove", Path: "/a"}}); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatal("a LESS file must be refused:", err)
	}
}

const patchPage = `{"schemaVersion":"2.0","id":"index","tree":{"id":"root","type":"Stack","props":{},"children":[
{"id":"title","type":"Label","props":{"text":"Hi"}},
{"id":"tabs","type":"Tabs","children":{"first":[{"id":"field","type":"TextInput","props":{"label":"Name"}}]}}]}}`

func TestPatchSourceNodeID(t *testing.T) {
	out, applied, _, err := patchSource(patchPage, "field", []PatchOp{
		{Op: "replace", Path: "/props/label", Value: json.RawMessage(`"Full name"`)},
		{Op: "copy", From: "/props/label", Path: "/props/placeholder"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `{"id":"field","type":"TextInput","props":{"label":"Full name","placeholder":"Full name"}}`) ||
		applied[0] != "replace /tree/children/1/children/first/0/props/label" ||
		applied[1] != "copy /tree/children/1/children/first/0/props/label -> /tree/children/1/children/first/0/props/placeholder" {
		t.Fatalf("%s\n%v", out, applied)
	}
	if out, _, _, err = patchSource(patchPage, "root", []PatchOp{{Op: "add", Path: "/props/size", Value: json.RawMessage(`"xl"`)}}); err != nil || !strings.Contains(out, `"type":"Stack","props":{"size":"xl"}`) {
		t.Fatal(out, err)
	}
	if _, _, _, err = patchSource(patchPage, "nope", []PatchOp{{Op: "remove", Path: "/props"}}); err == nil || !strings.Contains(err.Error(), "node ids: root, title, tabs, field") {
		t.Fatal("an unknown nodeId must list the ids:", err)
	}
	dup := strings.Replace(patchPage, `"id":"title"`, `"id":"field"`, 1)
	if _, _, _, err = patchSource(dup, "field", []PatchOp{{Op: "remove", Path: "/props"}}); err == nil || !strings.Contains(err.Error(), "names 2 nodes") {
		t.Fatal("a duplicate id must be refused:", err)
	}
	if _, _, _, err = patchSource(`{"a":1}`, "root", []PatchOp{{Op: "remove", Path: "/a"}}); err == nil || !strings.Contains(err.Error(), "a file with a tree") {
		t.Fatal("nodeId needs a tree:", err)
	}
	if _, _, _, err = patchSource(patchPage, "root", []PatchOp{{Op: "remove", Path: "props"}}); err == nil || !strings.Contains(err.Error(), "must start with /") {
		t.Fatal("a relative path must still be a pointer:", err)
	}
}

// "" under nodeId is the node itself: add and replace swap it, remove deletes it, and an op after
// the node is gone is refused instead of reaching the sibling that took its index.
func TestPatchSourceNodeItself(t *testing.T) {
	label := json.RawMessage(`{"id":"title","type":"Label","props":{"text":"Hello"}}`)
	for _, op := range []string{"add", "replace"} {
		out, applied, _, err := patchSource(patchPage, "title", []PatchOp{{Op: op, Path: "", Value: label}})
		if err != nil || strings.Count(out, `"id":"title"`) != 1 || !strings.Contains(out, `"children":[{"id":"title","type":"Label","props":{"text":"Hello"}},{"id":"tabs"`) ||
			applied[0] != "replace /tree/children/0" {
			t.Fatalf("%s \"\": %s %v %v", op, out, applied, err)
		}
	}
	out, applied, _, err := patchSource(patchPage, "title", []PatchOp{{Op: "remove", Path: ""}})
	if err != nil || strings.Contains(out, `"id":"title"`) || applied[0] != "remove /tree/children/0" {
		t.Fatal(out, applied, err)
	}
	_, _, _, err = patchSource(patchPage, "title", []PatchOp{{Op: "remove", Path: ""}, {Op: "add", Path: "/props/size", Value: json.RawMessage(`"xl"`)}})
	if err == nil || !strings.Contains(err.Error(), `op 1: after the ops before it, no node with id "title"`) {
		t.Fatal("an op after the node was removed must be refused:", err)
	}
	// replaced by a node of another id, the node is gone for the ops after it too
	_, _, _, err = patchSource(patchPage, "title", []PatchOp{{Op: "replace", Path: "", Value: json.RawMessage(`{"id":"heading","type":"Label"}`)}, {Op: "remove", Path: "/props"}})
	if err == nil || !strings.Contains(err.Error(), `op 1: after the ops before it, no node with id "title"`) {
		t.Fatal(err)
	}
	if _, _, _, err = patchSource(patchPage, "title", []PatchOp{{Op: "copy", From: "/props", Path: ""}}); err == nil || !strings.Contains(err.Error(), "onto the node itself") {
		t.Fatal(err)
	}
}

// Nodes inside overlay.open content, in node actions and in bindings, are found like tree nodes.
func TestFindNodeInOverlays(t *testing.T) {
	page := `{"schemaVersion":"2.0","id":"index","tree":{"id":"root","type":"Stack","children":[
{"id":"open","type":"Button","actions":{"onClick":[{"type":"if","then":[{"type":"overlay.open","content":{"id":"dialog","type":"Stack","children":[{"id":"ok","type":"Button"}]}}]}]}}]},
"bindings":[{"on":"x","do":[{"type":"overlay.open","content":[{"id":"side","type":"Label"}]}]}]}`
	doc, err := decodeDoc([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"ok":   "/tree/children/0/actions/onClick/0/then/0/content/children/0",
		"side": "/bindings/0/do/0/content/0",
	} {
		if got, err := findNode(doc, id); err != nil || got != want {
			t.Errorf("%s: %s %v, want %s", id, got, err, want)
		}
	}
	out, _, _, err := patchSource(page, "ok", []PatchOp{{Op: "add", Path: "/props", Value: json.RawMessage(`{"title":"OK"}`)}})
	if err != nil || !strings.Contains(out, `{"id":"ok","type":"Button","props":{"title":"OK"}}`) {
		t.Fatal(out, err)
	}
}

// A definition file is one node: nodeId finds its nodes, its root by its id or "" paths. A $ref
// instance is found by its own id, its definition is not opened, and an op giving it a key only the
// definition holds is refused, naming the file to patch.
func TestPatchDefinitionsAndInstances(t *testing.T) {
	def := `{"id":"row","type":"Stack","props":{"size":"md"},"children":[{"id":"row-label","type":"Label","props":{"text":"Label"}}]}`
	out, applied, _, err := patchSource(def, "row-label", []PatchOp{{Op: "replace", Path: "/props/text", Value: json.RawMessage(`"Name"`)}})
	if err != nil || applied[0] != "replace /children/0/props/text" || !strings.Contains(out, `"text":"Name"`) {
		t.Fatal(out, applied, err)
	}
	if _, applied, _, err = patchSource(def, "row", []PatchOp{{Op: "replace", Path: "/props/size", Value: json.RawMessage(`"lg"`)}}); err != nil || applied[0] != "replace /props/size" {
		t.Fatal(applied, err)
	}
	page := `{"schemaVersion":"2.0","id":"index","tree":{"id":"root","type":"Stack","children":[{"id":"a","$ref":"#/definitions/field-row"},{"id":"b","$ref":"#/definitions/field-row","if":"vm.more"}]}}`
	if _, _, _, err = patchSource(page, "row-label", []PatchOp{{Op: "remove", Path: "/props"}}); err == nil || !strings.Contains(err.Error(), "patch path definitions/<name> (this file uses definitions/field-row)") {
		t.Fatal("a node inside a definition must point to its file:", err)
	}
	for _, path := range []string{"/props", "/props/size", "/children/-", "/type", "/actions"} {
		_, _, _, err = patchSource(page, "a", []PatchOp{{Op: "add", Path: path, Value: json.RawMessage(`{}`)}})
		if err == nil || !strings.Contains(err.Error(), "is an instance of #/definitions/field-row") || !strings.Contains(err.Error(), "patch path definitions/field-row") {
			t.Errorf("%s on an instance: %v", path, err)
		}
	}
	out, _, _, err = patchSource(page, "b", []PatchOp{{Op: "replace", Path: "/if", Value: json.RawMessage(`"vm.all"`)}, {Op: "add", Path: "/visibility", Value: json.RawMessage(`"hidden"`)}})
	if err != nil || !strings.Contains(out, `{"id":"b","$ref":"#/definitions/field-row","if":"vm.all","visibility":"hidden"}`) {
		t.Fatal("the instance's own keys are patched in place:", out, err)
	}
	out, _, _, err = patchSource(page, "a", []PatchOp{{Op: "replace", Path: "", Value: json.RawMessage(`{"id":"a","type":"Label","props":{"text":"inline"}}`)}})
	if err != nil || !strings.Contains(out, `"children":[{"id":"a","type":"Label","props":{"text":"inline"}},`) {
		t.Fatal("an instance may be replaced by an inline node:", out, err)
	}
}
