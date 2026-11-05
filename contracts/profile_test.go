package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSmartProfileSchemaAndCompatibility(t *testing.T) {
	if err := CheckCompatibility(SmartForms); err != nil {
		t.Fatal(err)
	}
	var d Document
	if err := json.Unmarshal(SmartForms, &d); err != nil {
		t.Fatal(err)
	}
	for _, m := range d.Methods {
		if _, err := InputSchema(m.Name); err != nil {
			t.Fatalf("%s: %v", m.Name, err)
		}
	}
	valid := `{"operationId":"generation-1","ref":"test-app","title":"Test"}`
	if err := ValidateInput("application/create", []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"ref":"test-app","title":"Test"}`,
		`{"operationId":null,"ref":"test-app","title":"Test"}`,
		`{"operationId":"bad key","ref":"test-app","title":"Test"}`,
		`{"operationId":"generation-1","ref":"test-app","title":"Test","corezoidCredentials":{}}`,
		`{"operationId":"generation-1","ref":"test-app","title":"Test","cduVersion":1}`,
		`{"operationId":"generation-1","ref":"test-app","title":"Test","accId":"other"}`,
	} {
		if ValidateInput("application/create", []byte(bad)) == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	for _, name := range []string{"operationId", "environmentPreconditions", "expectedSourceEnvironmentHash"} {
		mutated := strings.ReplaceAll(string(SmartForms), `"name": "`+name+`"`, `"name": "unsupported"`)
		if CheckCompatibility([]byte(mutated)) == nil {
			t.Fatalf("missing %s accepted", name)
		}
	}
	if CheckCompatibility([]byte(strings.Replace(string(SmartForms), `"x-audience": "public"`, `"x-audience": "private"`, 1))) == nil {
		t.Fatal("private method accepted")
	}
}
func TestUpdateSchemaRequiresCASAndPreservesExplicitEmptySource(t *testing.T) {
	hash := strings.Repeat("a", 64)
	input := `{"operationId":"update-1","actorId":"actor","environmentPreconditions":[{"envId":1,"environmentHash":"` + hash + `"}],"items":[{"id":3,"title":"config","type":"application/json","ifMatchHash":"` + hash + `","source":""}]}`
	if err := ValidateInput("app_file/update", []byte(input)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(input, `"environmentPreconditions"`, `"unknown"`, 1),
		strings.Replace(input, `"ifMatchHash"`, `"unknown"`, 1),
		strings.Replace(input, hash, "bad", 1),
		strings.Replace(input, `"source":""`, `"source":null`, 1),
		strings.Replace(input, `"title":"config",`, "", 1),
	} {
		if ValidateInput("app_file/update", []byte(bad)) == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
