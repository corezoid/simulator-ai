package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCollapseSourcePin(t *testing.T) {
	raw, err := os.ReadFile("collapse-source.json")
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
	if source.Revision != "2b64aab1134821a20bc4a453148d450650abc05a" || source.Digest != fmt.Sprintf("%x", sha256.Sum256(CollapseSource)) || source.Methods != len(collapseParams) {
		t.Fatal("collapse contract source pin drift")
	}
}

func TestCollapseSchemas(t *testing.T) {
	get, count, update, err := CollapseSchemas()
	if err != nil {
		t.Fatal(err)
	}
	all := string(get) + string(count) + string(update)
	if strings.Contains(all, "x-validation-rules") || strings.Contains(all, `"accId"`) || strings.Contains(all, `"actorId"`) {
		t.Fatal("schemas carry backend tags or backend names")
	}
	for _, tc := range []struct {
		schema []byte
		valid  []string
		bad    []string
	}{
		{get, []string{`{"layerId":"L"}`, `{"layerId":"L","laId":3,"q":"x","limit":50,"offset":10}`},
			[]string{`{}`, `{"layerId":"L","limit":51}`, `{"layerId":"L","laId":0}`, `{"layerId":"L","x":1}`}},
		{count, []string{`{"layerId":"L","laId":3,"condition":{"formId":5,"q":"a=1"}}`, `{"layerId":"L","laId":3,"condition":{"formId":5,"incomeType":"credit","amountFrom":1.5}}`},
			[]string{`{"layerId":"L","laId":3}`, `{"layerId":"L","laId":3,"condition":{}}`, `{"layerId":"L","laId":3,"condition":{"formId":5,"incomeType":"x"}}`}},
		{update, []string{`{"layerId":"L","collapse":[1]}`, `{"layerId":"L","expandAll":true,"fanOpen":[2]}`, `{"layerId":"L","conditional":[{"laId":1,"condition":{"formId":5}}]}`},
			[]string{`{"layerId":"L"}`, `{"layerId":"L","conditional":[{"laId":1}]}`, `{"layerId":"L","roots":[1]}`}},
	} {
		v := compileSchema(t, tc.schema)
		for _, doc := range tc.valid {
			var x any
			_ = json.Unmarshal([]byte(doc), &x)
			if err := v.Validate(x); err != nil {
				t.Errorf("%s refused: %v", doc, err)
			}
		}
		for _, doc := range tc.bad {
			var x any
			_ = json.Unmarshal([]byte(doc), &x)
			if v.Validate(x) == nil {
				t.Errorf("%s accepted", doc)
			}
		}
	}
	if !strings.Contains(string(count), "saved actor-filter grammar") || strings.Contains(string(update), "saved actor-filter grammar") {
		t.Fatal("the condition's words belong to count only")
	}
}

func TestCollapseSchemasRefuseDrift(t *testing.T) {
	saved := CollapseSource
	t.Cleanup(func() { CollapseSource = saved })
	for _, drift := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `"x-scope": "actors.readonly"`, `"x-scope": "actors.management"`, 1)
		},
		func(s string) string { return strings.Replace(s, `"name": "fanReset"`, `"name": "fanResetAll"`, 1) },
		func(s string) string {
			return strings.Replace(s, `"x-audience": "public"`, `"x-audience": "internal"`, 1)
		},
	} {
		CollapseSource = []byte(drift(string(saved)))
		if string(CollapseSource) == string(saved) {
			t.Fatal("drift did not apply")
		}
		if _, _, _, err := CollapseSchemas(); err == nil {
			t.Error("drift accepted")
		}
	}
}
