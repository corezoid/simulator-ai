package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestGraphSourcePin(t *testing.T) {
	raw, err := os.ReadFile("graph-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Revision string `json:"revision"`
		Digest   string `json:"projectionSha256"`
		Methods  int    `json:"methods"`
	}
	if err = json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Revision != "d2d24e4eeec0037665de64d8ec2a4c179cd16731" || source.Digest != fmt.Sprintf("%x", sha256.Sum256(GraphReads)) || source.Methods != 3 {
		t.Fatal("graph contract source pin drift")
	}
	var document struct {
		Methods []Method `json:"methods"`
	}
	if err = json.Unmarshal(GraphReads, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Methods) != source.Methods {
		t.Fatal("graph catalog widened")
	}
	wanted := map[string]string{"edge_type/list": "actors.readonly", "form/system/get": "forms.readonly", "form/flowchart-block/search": "forms.readonly"}
	for _, method := range document.Methods {
		if wanted[method.Name] == "" || method.Scope != wanted[method.Name] || method.Audience != "public" {
			t.Fatalf("unexpected descriptor %s", method.Name)
		}
		delete(wanted, method.Name)
	}
	if len(wanted) != 0 {
		t.Fatal("missing graph metadata method")
	}
}

func TestGraphOutputSchemasValidateProjectedResults(t *testing.T) {
	for _, tc := range []struct{ method, output string }{
		{"form/system/get", `{"id":20,"title":"Layers","type":"system","form":{}}`},
		{"edge_type/list", `{"list":[{"id":3,"name":"interaction","isTree":false,"isSystem":true}]}`},
		{"form/flowchart-block/search", `{"list":[{"id":21,"parentId":20,"title":"Process","blockId":"process","updatedAt":1,"form":{}}],"hasMore":false}`},
	} {
		raw, err := GraphOutputSchema(tc.method)
		if err != nil {
			t.Fatal(err)
		}
		var schema, output any
		if err = json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal([]byte(tc.output), &output); err != nil {
			t.Fatal(err)
		}
		compiler := jsonschema.NewCompiler()
		if err = compiler.AddResource("https://test.example/graph", schema); err != nil {
			t.Fatal(err)
		}
		validator, err := compiler.Compile("https://test.example/graph")
		if err != nil {
			t.Fatal(err)
		}
		if err = validator.Validate(output); err != nil {
			t.Fatalf("%s: %v", tc.method, err)
		}
		output.(map[string]any)["editToken"] = "canary"
		if validator.Validate(output) == nil {
			t.Fatal("output schema admits capabilities")
		}
	}
}
