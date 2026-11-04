package contracts

import (
	"strings"
	"testing"
)

func TestCompactKeepsPropertiesNamedDescription(t *testing.T) {
	for _, method := range []string{"form/create", "form/update"} {
		schema, err := ParityInputSchema(method)
		if err != nil {
			t.Fatal(err)
		}
		s := string(schema)
		// sections[].content[] has a field property literally named description.
		if !strings.Contains(s, `"description":{"type":"string"}`) {
			t.Errorf("%s lost the template field's description property", method)
		}
		if strings.Contains(s, "Help text under the field") {
			t.Errorf("%s kept a nested description annotation", method)
		}
	}
}
