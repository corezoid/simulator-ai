package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGraphInputBoundsAndHostBinding(t *testing.T) {
	for _, tc := range []struct {
		method, raw string
		valid       bool
	}{
		{"form/flowchart-block/search", `{}`, true},
		{"form/flowchart-block/search", `{"blockIds":["native-kind"]}`, true},
		{"form/flowchart-block/search", `{"blockIds":[]}`, false},
		{"form/flowchart-block/search", `{"blockIds":null}`, false},
		{"form/flowchart-block/search", `{"limit":101}`, false},
		{"form/flowchart-block/search", `{"limit":0}`, false},
		{"form/flowchart-block/search", `{"accId":"other"}`, false},
		{"form/flowchart-block/search", `{"token":"secret"}`, false},
		{"form/system/get", `{"title":"Layers"}`, true},
		{"form/system/get", `{"title":"Secrets"}`, false},
		{"edge_type/list", `{"limit":100,"offset":0}`, true},
		{"edge_type/list", `{"offset":-1}`, false},
		{"form/flowchart-block/list", `{}`, false},
		{"form/system/get", `{"title":"Layers"} {}`, false},
	} {
		if err := ValidateGraphInput(tc.method, []byte(tc.raw)); (err == nil) != tc.valid {
			t.Errorf("%s %s: %v", tc.method, tc.raw, err)
		}
	}
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "native-kind"
	}
	raw, _ := json.Marshal(map[string]any{"blockIds": ids})
	if ValidateGraphInput("form/flowchart-block/search", raw) == nil {
		t.Fatal("unbounded exact lookup")
	}
}

func TestGraphOutputSchemaDoesNotPromiseUnprojectedSecrets(t *testing.T) {
	for _, method := range []string{"form/system/get", "form/flowchart-block/search", "edge_type/list"} {
		raw, err := GraphOutputSchema(method)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"editToken", "ownerId", "userId", "accId"} {
			if strings.Contains(string(raw), `"`+field+`":`) {
				t.Fatalf("%s promises %s", method, field)
			}
		}
	}
}
