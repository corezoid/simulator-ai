package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"testing"
)

func TestReactionStatsSchemas(t *testing.T) {
	input, err := ReactionStatsInputSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		schema         []byte
		valid, invalid []string
	}{
		{input, []string{`{"actorId":"a"}`}, []string{`{}`, `{"actorId":null}`, `{"actorId":"a","accId":"foreign"}`, `{"actorId":12}`}},
		{[]byte(ReactionStatsOutputSchema), []string{`{}`, `{"comment":0}`, `{"comment":2,"view":1}`}, []string{`null`, `[]`, `{"comment":null}`, `{"comment":-1}`, `{"comment":1.5}`, `{"":2}`}},
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

func TestReactionStatsSourcePin(t *testing.T) {
	raw, err := os.ReadFile("reactions-source.json")
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
	if pin.Revision != "f3cd018a3d291ad59546ad77ee68b910c1cab94f" || pin.Methods != 1 || pin.Digest != fmt.Sprintf("%x", sha256.Sum256(ReactionStatsSource)) {
		t.Fatal("reaction source pin drift")
	}
}
