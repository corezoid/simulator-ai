// gen-graph-contract pins only the reviewed public graph-read profile descriptors.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

var createNames = []string{"actor/create", "actor/operation/get", "edge/create", "edge/operation/get", "graph/layer/actors/manage", "graph/layer/operation/get"}

var names = []string{"edge_type/list", "form/flowchart-block/search", "form/system/get"}

type descriptor struct {
	Name     string          `json:"name"`
	Params   json.RawMessage `json:"params"`
	Result   json.RawMessage `json:"result"`
	Audience string          `json:"x-audience"`
	Scope    string          `json:"x-scope"`
}
type document struct {
	Methods []descriptor `json:"methods"`
}

func project(raw []byte) ([]byte, error) { return projectNames(raw, names) }

func projectNames(raw []byte, selectedNames []string) ([]byte, error) {
	var source document
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, err
	}
	byName := map[string]descriptor{}
	for _, method := range source.Methods {
		if _, exists := byName[method.Name]; exists {
			return nil, fmt.Errorf("duplicate method: %s", method.Name)
		}
		byName[method.Name] = method
	}
	selected := document{Methods: make([]descriptor, 0, len(selectedNames))}
	for _, name := range selectedNames {
		method, ok := byName[name]
		if !ok || method.Audience != "public" || method.Scope == "" || len(method.Params) == 0 || len(method.Result) == 0 {
			return nil, fmt.Errorf("missing or unsupported public descriptor: %s", name)
		}
		selected.Methods = append(selected.Methods, method)
	}
	return json.MarshalIndent(selected, "", "  ")
}

func main() {
	profile := flag.String("profile", "read", "read, create, elements or reactions-read")
	input := flag.String("input", "", "reviewed local public OpenRPC artifact")
	revision := flag.String("revision", "", "reviewed backend commit")
	output := flag.String("output", "contracts/graph-openrpc.json", "projection destination")
	flag.Parse()
	if *input == "" || len(*revision) != 40 {
		fail(fmt.Errorf("input and full reviewed revision required"))
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fail(err)
	}
	selected := names
	if *profile == "create" {
		selected = createNames
	} else if *profile == "elements" {
		selected = []string{"graph/layer/elements/list"}
	} else if *profile == "reactions-read" {
		selected = []string{"reaction/stats/get"}
	} else if *profile != "read" {
		fail(fmt.Errorf("unsupported profile"))
	}
	result, err := projectNames(raw, selected)
	if err != nil {
		fail(err)
	}
	result = append(result, '\n')
	if err = os.WriteFile(*output, result, 0644); err != nil {
		fail(err)
	}
	fmt.Printf("%d public graph-%s methods; backend %s; projection sha256 %x\n", len(selected), *profile, *revision, sha256.Sum256(result))
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
