package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestPinnedContractAndGeneration(t *testing.T) {
	raw, e := os.ReadFile("../../contracts/public-openrpc.json")
	if e != nil {
		t.Fatal(e)
	}
	out, e := generate(raw)
	if e != nil {
		t.Fatal(e)
	}
	committed, e := os.ReadFile("../../internal/publicapi/types.gen.go")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(out, committed) {
		t.Fatal("generated types drift")
	}
	var d Document
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	d.Methods[0].Audience = "internal"
	b, _ := json.Marshal(d)
	if _, e = generate(b); e == nil {
		t.Fatal("internal API admitted")
	}
	d.Methods[0].Audience = "public"
	delete(d.Methods[0].Result.Schema.Properties, "id")
	b, _ = json.Marshal(d)
	if _, e = generate(b); e == nil {
		t.Fatal("required projection removal not detected")
	}
}
