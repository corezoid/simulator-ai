package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestGraphCreateContractPinAndPreflight(t *testing.T) {
	raw, err := os.ReadFile("graph-create-source.json")
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
	if pin.Revision != "0178ed493ecc5e24b389c57d596fd22b48e618c9" || pin.Methods != 6 || pin.Digest != fmt.Sprintf("%x", sha256.Sum256(GraphCreates)) {
		t.Fatal("graph create pin drift")
	}
	if err = CheckGraphCreateCompatibility(GraphCreates); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(string(GraphCreates), `"operationId"`, `"ignoredKey"`, 1),
		strings.Replace(string(GraphCreates), `"public"`, `"internal"`, 1),
		strings.Replace(string(GraphCreates), `"actors.management"`, `"actors.readonly"`, 1),
		`{"methods":[]}`,
	} {
		if CheckGraphCreateCompatibility([]byte(bad)) == nil {
			t.Fatal("incompatible graph write backend accepted")
		}
	}
}
func TestGraphCreationInputs(t *testing.T) {
	actor := `{"operationId":"actor-1","formId":20,"title":"Test","data":{"__form__21:view":{"shape":{}}}}`
	node := `{"operationId":"nodes-1","actorId":"layer","items":[{"action":"create","data":{"id":"actor","type":"node","position":{"x":0,"y":0}}}]}`
	edge := `{"operationId":"edges-1","actorId":"layer","items":[{"action":"create","data":{"id":"edge","type":"edge","laIdSource":1,"laIdTarget":2}}]}`
	for _, tc := range []struct {
		method, raw string
		valid       bool
	}{
		{"actor/create", actor, true},
		{"actor/create", strings.Replace(actor, `"operationId":"actor-1",`, "", 1), false},
		{"actor/create", strings.Replace(actor, `"formId":20`, `"formId":0`, 1), false},
		{"actor/create", strings.Replace(actor, `"title":"Test"`, `"title":""`, 1), false},
		{"actor/create", strings.Replace(actor, `"data":{"__form__21:view":{"shape":{}}}`, `"data":null`, 1), false},
		{"actor/create", strings.Replace(actor, `"title":"Test"`, `"title":"Test","isPublic":true`, 1), false},
		{"actor/create", strings.Replace(actor, `"title":"Test"`, `"title":"Test","accId":"other"`, 1), false},
		{"actor/operation/get", `{"operationId":"actor-1"}`, true},
		{"edge/create", `{"operationId":"edge-1","source":"a","target":"b","edgeTypeId":3}`, true},
		{"edge/operation/get", `{"operationId":""}`, false},
		{"graph/layer/actors/manage", node, true},
		{"graph/layer/actors/manage", edge, true},
		{"graph/layer/actors/manage", strings.Replace(node, `"action":"create"`, `"action":"delete"`, 1), false},
		{"graph/layer/actors/manage", strings.Replace(node, `"x":0,"y":0`, `"x":0`, 1), false},
		{"graph/layer/actors/manage", strings.Replace(edge, `,"laIdTarget":2`, "", 1), false},
		{"graph/layer/actors/manage", strings.Replace(edge, `"laIdSource":1`, `"laIdSource":0`, 1), false},
		{"graph/layer/actors/manage", strings.Replace(node, `"type":"node"`, `"type":"node","laId":3`, 1), false},
		{"graph/layer/actors/manage", strings.Replace(node, `"type":"node"`, `"type":"node","laIdSource":3`, 1), false},
		{"graph/layer/operation/get", `{"operationId":"nodes-1","actorId":"layer"}`, true},
		{"graph/layer/operation/get", `{"operationId":"nodes-1"}`, false},
		{"actor/update", `{}`, false},
	} {
		if err := ValidateGraphCreateInput(tc.method, []byte(tc.raw)); (err == nil) != tc.valid {
			t.Errorf("%s %s: %v", tc.method, tc.raw, err)
		}
	}
}

func TestNativePlacementSchemaBounds(t *testing.T) {
	native := `{"operationId":"native-1","actorId":"layer","items":[{"action":"create","data":{"id":"block","type":"node","position":{"x":100,"y":200},"areaPicture":{"width":200,"height":100,"img":"data:image/svg+xml;base64,fixture"},"layerSettings":{"width":200,"height":100,"blockId":"actual-kind","shape":{"type":"primitive","primitive":[{"kind":"rect","x":0,"y":0,"w":200,"h":100}]},"textFrame":{"x":10,"y":10,"w":180,"h":80}}}}]}`
	if err := ValidateGraphCreateInput("graph/layer/actors/manage", []byte(native)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(native, `"width":200`, `"width":0`, 1),
		strings.Replace(native, `"blockId":"actual-kind"`, `"blockId":""`, 1),
		strings.Replace(native, `"blockId":"actual-kind"`, `"blockId":"`+strings.Repeat("a", 129)+`"`, 1),
		strings.Replace(native, `"textFrame":{"x":10,`, `"textFrame":{`, 1),
		strings.Replace(native, `"blockId":"actual-kind"`, `"blockId":"actual-kind","offset":{"top":4}`, 1),
		strings.Replace(native, `"w":180`, `"w":-1`, 1),
		strings.Replace(native, `"shape":{"type":"primitive","primitive":[{"kind":"rect","x":0,"y":0,"w":200,"h":100}]}`, `"shape":"rectangle"`, 1),
	} {
		if ValidateGraphCreateInput("graph/layer/actors/manage", []byte(bad)) == nil {
			t.Fatal("invalid native placement accepted")
		}
	}
}
