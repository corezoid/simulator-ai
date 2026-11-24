package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

// The walks name the edge types they answer (eval multihop-08 guessed what type 3 was): the base
// types from ontology/get, a per-account money type by its weight; without ontology/get the answer
// stays as it was.
func TestWalksNameEdgeTypes(t *testing.T) {
	ontology := `{"version":"v","edgeTypes":[{"id":1,"name":"hierarchy","role":"hierarchy"},{"id":3,"name":"directedInteraction","role":"dependency"}]}`
	send, calls := kbServer(t, func(method string, _ map[string]json.RawMessage) string {
		switch method {
		case "ontology/get":
			return ontology
		case "graph/traverse", "graph/path/find":
			return `{"nodes":[{"id":"a"},{"id":"b"}],"edges":[{"id":"e1","source":"b","target":"a","edgeTypeId":3},{"id":"e2","source":"b","target":"a","edgeTypeId":9,"weight":"5","linkedActorId":"x"}]}`
		}
		return `{}`
	})
	for _, tool := range []string{"graph_traverse", "graph_path_find"} {
		args := `{"actorIds":["a"],"direction":"up","edgeTypeIds":[3,9]}`
		if tool == "graph_path_find" {
			args = `{"fromActorId":"a","toActorId":"b"}`
		}
		r := send(tool, args)
		if !strings.Contains(r, `"edgeTypes":[{"id":3,"name":"directedInteraction","role":"dependency"},{"id":9,"role":"transactions"}]`) {
			t.Fatalf("%s: %s", tool, r)
		}
		if last := (*calls)[len(*calls)-1]; last.Method != "ontology/get" || string(last.Params["fields"]) != `["edgeTypes"]` {
			t.Fatalf("%s read %s %s", tool, last.Method, last.Params["fields"])
		}
	}
	ontology = `{"error":"no"}` // not a digest: the answer is left as is
	if r := send("graph_traverse", `{"actorIds":["a"]}`); strings.Contains(r, `"edgeTypes"`) || !strings.Contains(r, `"e1"`) {
		t.Fatal(r)
	}
}
