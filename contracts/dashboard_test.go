package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDashboardSourcePin(t *testing.T) {
	raw, err := os.ReadFile("dashboard-source.json")
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
	if source.Revision != "d13fc772bf38a51e345f39f51f789b227189d547" || source.Digest != fmt.Sprintf("%x", sha256.Sum256(DashboardSource)) || source.Methods != 1 {
		t.Fatal("dashboard contract source pin drift")
	}
}

func compileSchema(t *testing.T, raw []byte) *jsonschema.Schema {
	t.Helper()
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://test.example/dashboard", schema); err != nil {
		t.Fatal(err)
	}
	validator, err := compiler.Compile("https://test.example/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

func TestDashboardSchemas(t *testing.T) {
	input, output, err := DashboardSchemas()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(input)+string(output), "x-validation-rules") || strings.Contains(string(input), `"source"`) || strings.Contains(string(input), `"accId"`) {
		t.Fatal("schemas carry backend tags, the preview override or the host workspace")
	}
	ids := `"a1","a2"`
	for range 98 {
		ids += `,"a"`
	}
	for _, tc := range []struct {
		schema         []byte
		valid, invalid []string
	}{
		{input, []string{
			`{"chartId":"c"}`,
			`{"chartId":"c","from":1788210000000,"to":1788382799000,"interval":"day","timezoneOffset":-180,"with":["total","series"],"balanceOnDate":true,"counterType":"count","actorIds":["a1"]}`,
			`{"chartId":"c","actorIds":[` + ids + `]}`,
			// JS Date#getTimezoneOffset sign: Kiribati (UTC+14) is -840, Baker Island (UTC-12) 720.
			`{"chartId":"c","timezoneOffset":-840}`, `{"chartId":"c","timezoneOffset":720}`,
		}, []string{
			`{}`, `{"chartId":""}`, `{"chartId":12}`, `{"actorId":"c"}`, `{"chartId":"c","accId":"foreign"}`,
			`{"chartId":"c","source":{"accounts":[]}}`, `{"chartId":"c","with":["groups"]}`, `{"chartId":"c","with":["total","total"]}`,
			`{"chartId":"c","interval":"week"}`, `{"chartId":"c","counterType":"turnover"}`, `{"chartId":"c","timezoneOffset":721}`, `{"chartId":"c","timezoneOffset":-841}`,
			`{"chartId":"c","from":1.5}`, `{"chartId":"c","actorIds":[]}`, `{"chartId":"c","actorIds":[""]}`, `{"chartId":"c","actorIds":[` + ids + `,"x"]}`,
		}},
		{output, []string{
			`{"data":[]}`,
			`{"data":[{"actorId":"a","nameId":"n","currencyId":1,"incomeType":"total","accountType":"fact","value":0,"data":[{"bucketStart":1,"value":519}]}],"total":{"value":0,"data":[{"bucketStart":1,"value":519}]}}`,
		}, []string{`{}`, `{"data":null}`, `{"data":[{"actorId":"a"}]}`, `{"data":[{"actorId":"a","nameId":"n","currencyId":0,"incomeType":"total","accountType":"fact"}]}`}},
	} {
		validator := compileSchema(t, tc.schema)
		for _, raw := range tc.valid {
			var value any
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				t.Fatal(err)
			}
			if err = validator.Validate(value); err != nil {
				t.Fatalf("%s: %v", raw, err)
			}
		}
		for _, raw := range tc.invalid {
			var value any
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				t.Fatal(err)
			}
			if validator.Validate(value) == nil {
				t.Fatalf("invalid value accepted: %s", raw)
			}
		}
	}
}

func TestDashboardSchemasRefuseDrift(t *testing.T) {
	saved := DashboardSource
	t.Cleanup(func() { DashboardSource = saved })
	for _, edit := range []struct{ from, to string }{
		{`"name": "dashboard/get"`, `"name": "dashboard/group/get"`},
		{`"x-scope": "actors.readonly"`, `"x-scope": "actors.readwrite"`},
		{`"name": "balanceOnDate"`, `"name": "balanceAt"`},
		{`"name": "actorId",
          "required": true`, `"name": "actorId",
          "required": false`},
	} {
		if !strings.Contains(string(saved), edit.from) {
			t.Fatalf("projection lacks %s", edit.from)
		}
		DashboardSource = []byte(strings.Replace(string(saved), edit.from, edit.to, 1))
		if _, _, err := DashboardSchemas(); err == nil {
			t.Errorf("drift %s -> %s accepted", edit.from, edit.to)
		}
	}
}
