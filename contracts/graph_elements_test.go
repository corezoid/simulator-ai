package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestGraphElementsPinAndSchema(t *testing.T) {
	raw, err := os.ReadFile("graph-elements-source.json")
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
	if pin.Revision != "5e9f16c699dea8d831e353e32bb69accde7d04bb" || pin.Digest != fmt.Sprintf("%x", sha256.Sum256(GraphElementsSource)) || pin.Methods != 1 {
		t.Fatal("exact graph source pin drift")
	}
	good := `{"actorId":"layer","nodes":[11],"edges":[11]}`
	if err = ValidateGraphElementsInput([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"actorId":"layer","nodes":[11,11],"edges":[]}`, `{"actorId":"layer","nodes":[],"edges":null}`, `{"actorId":"layer","nodes":[0],"edges":[]}`, `{"actorId":"layer","nodes":[],"edges":[],"fields":["data"]}`, `{"actorId":"layer","nodes":[],"edges":[],"accId":"foreign"}`} {
		if ValidateGraphElementsInput([]byte(bad)) == nil {
			t.Fatal("unsafe exact graph arguments accepted", bad)
		}
	}
}
