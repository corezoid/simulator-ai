package main

import (
	"encoding/json"
	"testing"
)

func TestProjectionRefusesMissingPrivateAndDuplicateMethods(t *testing.T) {
	d := document{}
	for _, name := range names {
		d.Methods = append(d.Methods, descriptor{Name: name, Audience: "public", Scope: "forms.readonly", Params: json.RawMessage(`[]`), Result: json.RawMessage(`{"schema":{"type":"object"}}`)})
	}
	marshal := func() []byte {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if _, err := project(marshal()); err != nil {
		t.Fatal(err)
	}
	d.Methods[0].Audience = "internal"
	if _, err := project(marshal()); err == nil {
		t.Fatal("internal method accepted")
	}
	d.Methods[0].Audience = "public"
	d.Methods = append(d.Methods, d.Methods[0])
	if _, err := project(marshal()); err == nil {
		t.Fatal("duplicate accepted")
	}
	d.Methods = d.Methods[:2]
	if _, err := project(marshal()); err == nil {
		t.Fatal("missing method accepted")
	}
}
