package mcpserver

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const readHash = "1111111111111111111111111111111111111111111111111111111111111111"

// pageFile is app_file/get of pages/index/config (file 103 of consoleReplies' tree).
var pageFile = `"result":{"id":103,"folderId":102,"objType":"file","title":"config","type":"application/json","sourceHash":"` + readHash + `","source":` +
	jsonString("{\n  \"schemaVersion\": \"2.0\",\n  \"id\": \"index\",\n  \"tree\": {\n    \"id\": \"root\",\n    \"type\": \"Stack\",\n    \"props\": {},\n    \"children\": [\n      {\"id\": \"name\", \"type\": \"TextInput\", \"props\": {\"label\": \"Name\"}}\n    ]\n  }\n}") + `}`

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestSmartFormPagePatch(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{"app_file/get": pageFile})
	reply := callTool(send, "smart_form_page_patch", `{"appId":"app1","nodeId":"root","ops":[{"op":"add","path":"/props/size","value":"xl"}]}`)
	if strings.Contains(reply, `"isError":true`) || methodsOf(*calls) != "application/env/list,application/struct,app_file/get,app_file/update" {
		t.Fatal(methodsOf(*calls), reply)
	}
	update := (*calls)[3].Params
	items, _ := update["items"].([]any)
	item, _ := items[0].(map[string]any)
	source, _ := item["source"].(string)
	if item["id"] != float64(103) || item["title"] != "config" || item["type"] != "application/json" || item["ifMatchHash"] != readHash ||
		!strings.Contains(source, "\"type\": \"Stack\",\n    \"props\": {\n      \"size\": \"xl\"\n    },") {
		t.Fatalf("app_file/update %v", item)
	}
	// The answer names the file and its new hash, never the source.
	if !strings.Contains(reply, `"fileId":103,"path":"pages/index/config","sourceHash":"h2","applied":["add /tree/props/size"]`) || strings.Contains(reply, "schemaVersion") {
		t.Fatal(reply)
	}
}

func TestSmartFormPagePatchRefusals(t *testing.T) {
	refused := `"error":{"code":-32602,"message":"invalid params: invalid props for component \"Stack\" at /tree/props: additional properties 'gap' not allowed; did you mean wrap?","data":{"reason":"invalidParams"}}`
	cases := []struct {
		name, args string
		overrides  map[string]string
		want       string
		writes     bool
	}{
		{"sim-api's refusal is passed through", `{"appId":"app1","nodeId":"root","ops":[{"op":"add","path":"/props/gap","value":"xl"}]}`,
			map[string]string{"app_file/update": refused}, `additional properties 'gap' not allowed; did you mean wrap?`, true},
		{"a failed test writes nothing", `{"appId":"app1","ops":[{"op":"replace","path":"/tree/props","value":{"size":"xl"}},{"op":"test","path":"/id","value":"other"}]}`,
			nil, "op 1 (test /id): test failed", false},
		{"a bad pointer names the op", `{"appId":"app1","nodeId":"name","ops":[{"op":"replace","path":"/props/caption","value":"x"}]}`,
			nil, `op 0 (replace /tree/children/0/props/caption): /tree/children/0/props has no key "caption"`, false},
		{"an unknown node lists the ids", `{"appId":"app1","nodeId":"email","ops":[{"op":"remove","path":"/props"}]}`, nil, "node ids: root, name", false},
		{"a stale ifMatchHash is refused before writing", `{"appId":"app1","ifMatchHash":"` + strings.Repeat("2", 64) + `","ops":[{"op":"remove","path":"/tree/props"}]}`,
			nil, "the file changed since you read it: its sourceHash is now " + readHash, false},
		{"an unknown path lists the files", `{"appId":"app1","path":"pages/thanks/config","ops":[{"op":"remove","path":"/tree"}]}`, nil, "no develop file pages/thanks/config; files: pages/index/config", false},
		{"path and fileId exclude each other", `{"appId":"app1","path":"pages/index/config","fileId":103,"ops":[{"op":"remove","path":"/tree"}]}`, nil, "by path or by fileId", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overrides := map[string]string{"app_file/get": pageFile}
			for k, v := range tc.overrides {
				overrides[k] = v
			}
			send, calls := consoleServer(t, overrides)
			reply := callTool(send, "smart_form_page_patch", tc.args)
			if !strings.Contains(reply, `"isError":true`) || !strings.Contains(reply, tc.want) {
				t.Fatal(reply)
			}
			if wrote := strings.Contains(methodsOf(*calls), "app_file/update"); wrote != tc.writes {
				t.Fatalf("app_file/update sent: %v (%s)", wrote, methodsOf(*calls))
			}
		})
	}
}

// By fileId the file is read directly; a patch that changes nothing (a test alone) writes nothing.
func TestSmartFormPagePatchByFileID(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{"app_file/get": pageFile})
	reply := callTool(send, "smart_form_page_patch", `{"appId":"app1","fileId":103,"ops":[{"op":"test","path":"/tree/type","value":"Stack"}]}`)
	if methodsOf(*calls) != "app_file/get" || !strings.Contains(reply, `"sourceHash":"`+readHash+`","applied":["test /tree/type"],"unchanged":true`) {
		t.Fatal(methodsOf(*calls), reply)
	}
}

func TestSmartFormFilesGetPaths(t *testing.T) {
	send, calls := consoleServer(t, map[string]string{"app_file/get": pageFile})
	// pages/index/config is named twice (by itself and under pages/index): one entry, not missing.
	reply := callTool(send, "smart_form_files_get", `{"appId":"app1","paths":["pages/index","pages/index/config","pages/thanks"]}`)
	if methodsOf(*calls) != "application/env/list,application/struct,app_file/get" {
		t.Fatal(methodsOf(*calls))
	}
	if fields, _ := (*calls)[1].Params["fields"].([]any); len(fields) != 6 {
		t.Fatal("paths must not read every source through application/struct:", fields)
	}
	if strings.Contains(reply, `"path":"pages",`) || !strings.Contains(reply, `{"id":102,"path":"pages/index","objType":"folder"}`) ||
		!strings.Contains(reply, `"path":"pages/index/config","objType":"file","type":"application/json","sourceHash":"`+readHash+`","source":"{`) ||
		!strings.Contains(reply, `"missing":["pages/thanks"]`) || strings.Count(reply, `"path":"pages/index/config"`) != 2 /* text + structuredContent */ {
		t.Fatal(reply)
	}
}

// smart_form_components with types answers compact lines: the recorded Stack+Form answer of
// sim-api's 2.2 catalog (60 KB of JSON Schema) fits a few KB and still names Stack's gap prop.
func TestSmartFormComponentsCompact(t *testing.T) {
	fixture, err := os.ReadFile("testdata/cdu-catalog-stack-form.json")
	if err != nil {
		t.Fatal(err)
	}
	send, calls := consoleServer(t, map[string]string{"cdu/catalog/get": `"result":` + string(fixture)})
	reply := callTool(send, "smart_form_components", `{"types":["Stack","Form"]}`)
	var r struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err = json.Unmarshal([]byte(send(`"method":"tools/call","params":{"name":"smart_form_components","arguments":{"types":["Stack","Form"]}}`)), &r); err != nil || len(r.Result.Content) != 1 {
		t.Fatal(err)
	}
	text := r.Result.Content[0].Text
	t.Logf("cdu/catalog/get %d chars, compact %d", len(fixture), len([]rune(text)))
	for _, want := range []string{
		`size?: 'none'|'min'|'3xs'|'2xs'|'xs'|'sm'|'md'|'lg'|'xl'|'2xl'|'3xl'|'4xl'|'5xl'|'6xl'|'7xl'|'8xl'|'9xl'|'10xl'|'11xl' | {{expr}} — Gap size between items`,
		`space?: {as?: LayoutElement | {{expr}}`, `sides?: ('top'|'right'|'bottom'|'left'|'x'|'y' | {{expr}})[] | {{expr}}`, `size?: SpacingSize | {{expr}}`,
		`"SpacingSize":"'none'|'min'|`, `name: string`, `valueKey?: string — The vm key holding this form's values (default: the form id).`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("compact catalog lacks %s", want)
		}
	}
	if n := len([]rune(text)); n > 8000 || strings.Contains(text, "$defs") || strings.Contains(text, "aria-label") {
		t.Fatalf("compact catalog is %d chars:\n%s", n, text)
	}
	raw := callTool(send, "smart_form_components", `{"types":["Stack"],"schema":true}`)
	if last := (*calls)[len(*calls)-1].Params; last["schema"] != nil || !strings.Contains(raw, `"$defs"`) {
		t.Fatal("schema true must return the backend's schemas and stay local:", last)
	}
	if !strings.Contains(reply, `"notation"`) {
		t.Fatal(reply)
	}
}

func TestCompactCatalogEventsStateAPI(t *testing.T) {
	out, err := compactCatalog(json.RawMessage(`{"schemaVersion":"2.2","list":[{"type":"Panel","kind":"block","label":"Panel",
		"props":{"type":"object","required":["mode"],"properties":{"mode":{"enum":["a","b"],"description":"Mode. More text."},"onClick":{"type":"string"},"aria-label":{"type":"string"}}},
		"events":{"changed":{"type":"object","properties":{"value":{"type":"string"}},"description":"Fires on a change"},"opened":{"type":"object","properties":{}}},
		"state":{"type":"object","properties":{"isOpen":{"type":"boolean","description":"Whether it is open"}}},
		"api":{"open":{"args":{"type":"object","properties":{}},"description":"Open it"},"select":{"args":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}}}],"unknown":["Nope"]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `"list":[{"type":"Panel","kind":"block","label":"Panel","props":["mode: 'a'|'b' — Mode.","(+2 HTML/aria-*/on* attributes)"],` +
		`"events":["changed $event {value?: string} — Fires on a change","opened"],"state":["isOpen: boolean — Whether it is open"],"api":["open() — Open it","select({id: string})"]}],"unknown":["Nope"]}`
	if !strings.HasSuffix(string(out), want) {
		t.Fatalf("got %s", out)
	}
	brief := json.RawMessage(`{"schemaVersion":"2.2","list":[{"type":"Stack","kind":"primitive","label":"Stack"}]}`)
	if out, _ = compactCatalog(brief); string(out) != string(brief) {
		t.Fatal("a brief answer is returned as it came:", string(out))
	}
}

// paths ["definitions"] answers the folder, its subfolders (definitions/cards) and every
// definition file with its source; a definition is patched by its path like a page.
func TestSmartFormDefinitions(t *testing.T) {
	tree := `"result":{"tree":{"id":100,"objType":"folder","title":"root","children":[{"id":101,"objType":"folder","title":"pages","children":[]},{"id":110,"objType":"folder","title":"definitions","children":[
{"id":111,"objType":"file","title":"field-row","type":"application/json","sourceHash":"h"},{"id":112,"objType":"folder","title":"cards","children":[]}]}]}}`
	def := `"result":{"id":111,"title":"field-row","type":"application/json","sourceHash":"` + readHash + `","source":"{\"id\":\"row\",\"type\":\"Stack\",\"children\":[{\"id\":\"row-label\",\"type\":\"Label\",\"props\":{\"text\":\"Label\"}}]}"}`
	send, calls := consoleServer(t, map[string]string{"application/struct": tree, "app_file/get": def})
	reply := callTool(send, "smart_form_files_get", `{"appId":"app1","paths":["definitions"]}`)
	for _, want := range []string{`"path":"definitions","objType":"folder"`, `"path":"definitions/cards","objType":"folder"`, `"path":"definitions/field-row","objType":"file"`, `"source":"{`} {
		if !strings.Contains(reply, want) {
			t.Errorf("files_get definitions lacks %s:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, `"path":"pages"`) || strings.Count(methodsOf(*calls), "app_file/get") != 1 {
		t.Fatal(methodsOf(*calls), reply)
	}
	*calls = nil
	reply = callTool(send, "smart_form_page_patch", `{"appId":"app1","path":"definitions/field-row","nodeId":"row-label","ops":[{"op":"replace","path":"/props/text","value":"Name"}]}`)
	if !strings.Contains(reply, `"path":"definitions/field-row"`) || !strings.Contains(reply, `"applied":["replace /children/0/props/text"]`) || !strings.HasSuffix(methodsOf(*calls), "app_file/get,app_file/update") {
		t.Fatal(methodsOf(*calls), reply)
	}
}

// A Form without its submit yet saves (a page built in parts) with pending saying so; the release
// refuses the page until it sends what is typed.
func TestSmartFormPendingSubmit(t *testing.T) {
	formOnly := `{"schemaVersion":"2.0","id":"index","tree":{"id":"request","type":"Form","children":[{"id":"c","type":"TextInput"}]}}`
	send, calls := consoleServer(t, nil)
	src, _ := json.Marshal(formOnly)
	reply := callTool(send, "smart_form_files_update", `{"appId":"app1","items":[{"id":103,"title":"config","type":"application/json","ifMatchHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source":`+string(src)+`}]}`)
	if !strings.Contains(reply, `"pending":["file 0: Form "request" has no button with form.submit`) || !strings.Contains(methodsOf(*calls), "app_file/update") {
		t.Fatal(methodsOf(*calls), reply)
	}
	send, calls = consoleServer(t, map[string]string{"app_file/get": `"result":{"id":103,"folderId":102,"objType":"file","title":"config","type":"application/json","sourceHash":"h","source":` + string(src) + `}`})
	reply = callTool(send, "smart_form_release", `{"appId":"app1"}`)
	if !strings.Contains(reply, `not released, a page would send nothing: pages/index/config: Form "request" has no button with form.submit`) || strings.Contains(methodsOf(*calls), "release/create") {
		t.Fatal(methodsOf(*calls), reply)
	}
}
