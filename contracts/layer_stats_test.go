package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"testing"
)

func TestLayerStatsSchemas(t *testing.T) {
	input, output, err := LayerStatsSchemas()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		schema         []byte
		valid, invalid []string
	}{
		{input, []string{`{"actorId":"a"}`}, []string{`{}`, `{"actorId":null}`, `{"actorId":"a","accId":"foreign"}`, `{"actorId":12}`}},
		{output, []string{`{"nodes":0,"edges":0,"holes":0,"holesClosed":0,"edgeHoles":0,"edgeHolesClosed":0}`, `{"nodes":7,"edges":8,"holes":1,"holesClosed":2,"edgeHoles":3,"edgeHolesClosed":4}`}, []string{`null`, `[]`, `{}`, `{"nodes":null}`, `{"nodes":-1,"edges":0,"holes":0,"holesClosed":0,"edgeHoles":0,"edgeHolesClosed":0}`, `{"nodes":1.5,"edges":0,"holes":0,"holesClosed":0,"edgeHoles":0,"edgeHolesClosed":0}`}},
	} {
		var schema any
		if err = json.Unmarshal(tc.schema, &schema); err != nil {
			t.Fatal(err)
		}
		compiler := jsonschema.NewCompiler()
		if err = compiler.AddResource("https://test.example/stats", schema); err != nil {
			t.Fatal(err)
		}
		validator, err := compiler.Compile("https://test.example/stats")
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range tc.valid {
			var value any
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				t.Fatal(err)
			}
			if err = validator.Validate(value); err != nil {
				t.Fatal(err)
			}
		}
		for _, raw := range tc.invalid {
			var value any
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				t.Fatal(err)
			}
			if validator.Validate(value) == nil {
				t.Fatalf("invalid schema value accepted: %s", raw)
			}
		}
	}
}

func TestLayerStatsSourcePin(t *testing.T) {
	raw, err := os.ReadFile("layer-stats-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var pin struct {
		Revision string `json:"revision"`
		Digest   string `json:"projectionSha256"`
		Methods  int    `json:"methods"`
	}
	if err = json.Unmarshal(raw, &pin); err != nil {
		t.Fatal(err)
	}
	if pin.Revision != "26ed2cc681bfd4187b512061e164d5ef57814936" || pin.Methods != 1 || pin.Digest != fmt.Sprintf("%x", sha256.Sum256(LayerStatsSource)) {
		t.Fatal("layer source pin drift")
	}
}
