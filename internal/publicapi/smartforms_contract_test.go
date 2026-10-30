package publicapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai/contracts"
)

// Every exposed DTO field must exist with the same wire type in the pinned
// authoritative schema. Deliberately omitted fields (notably credentials/data)
// stay omitted. Recursive snapshot children use the documented root node shape.
func TestSmartFormDTOsAgainstPinnedPublicSchemas(t *testing.T) {
	var doc struct {
		Methods []struct {
			Name   string `json:"name"`
			Params []struct {
				Name   string        `json:"name"`
				Schema contractShape `json:"schema"`
			} `json:"params"`
			Result struct {
				Schema contractShape `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(contracts.SmartForms, &doc); err != nil {
		t.Fatal(err)
	}
	projections := map[string][2]any{
		"application/create":        {CreateApplicationInput{}, CreatedApplication{}},
		"application/operation/get": {ApplicationOperationInput{}, CreatedApplication{}},
		"application/env/list":      {AppInput{}, Environments{}},
		"application/snapshot/get":  {SnapshotInput{}, Snapshot{}},
		"app_folder/create":         {CreateFoldersInput{}, Folders{}},
		"app_folder/operation/get":  {AppOperationInput{}, Folders{}},
		"app_file/create":           {CreateFilesInput{}, Files{}},
		"app_file/update":           {UpdateFilesInput{}, Files{}},
		"app_file/operation/get":    {FileOperationInput{}, Files{}},
		"release/create":            {CreateReleaseInput{}, Release{}},
		"release/operation/get":     {AppOperationInput{}, Release{}},
		"release/get":               {ReleaseInput{}, Release{}},
		"route/key/get":             {AppInput{}, RouteKey{}},
	}
	for _, m := range doc.Methods {
		pair, ok := projections[m.Name]
		if !ok {
			t.Fatalf("unmapped %s", m.Name)
		}
		input := contractShape{Type: "object", Properties: map[string]contractShape{}}
		for _, p := range m.Params {
			input.Properties[p.Name] = p.Schema
		}
		t.Run(m.Name, func(t *testing.T) {
			checkShape(t, reflect.TypeOf(pair[0]), input, "input", 0)
			checkShape(t, reflect.TypeOf(pair[1]), m.Result.Schema, "result", 0)
		})
	}
	if len(projections) != len(doc.Methods) {
		t.Fatal("unverified projection")
	}
}

type contractShape struct {
	Type       string                   `json:"type"`
	Properties map[string]contractShape `json:"properties"`
	Items      *contractShape           `json:"items"`
}

func checkShape(t *testing.T, typ reflect.Type, s contractShape, path string, depth int) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		if s.Type != "object" {
			t.Fatalf("%s expected object", path)
		}
		if typ == reflect.TypeOf(ContentNode{}) && depth >= 3 {
			return
		} // OpenRPC recursion boundary.
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Anonymous {
				checkShape(t, f.Type, s, path, depth)
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			child, ok := s.Properties[name]
			if !ok {
				t.Fatalf("%s.%s absent from source", path, name)
			}
			checkShape(t, f.Type, child, path+"."+name, depth+1)
		}
	case reflect.Slice:
		if s.Type != "array" || s.Items == nil {
			t.Fatalf("%s expected array", path)
		}
		checkShape(t, typ.Elem(), *s.Items, path+"[]", depth+1)
	case reflect.String:
		if s.Type != "string" {
			t.Fatalf("%s expected string", path)
		}
	case reflect.Int64:
		if s.Type != "integer" {
			t.Fatalf("%s expected integer", path)
		}
	case reflect.Bool:
		if s.Type != "boolean" {
			t.Fatalf("%s expected boolean", path)
		}
	default:
		t.Fatalf("unmodeled type %s at %s", typ, path)
	}
}
func TestMalformedSmartResultsAreRejected(t *testing.T) {
	for _, v := range []any{CreatedApplication{}, Environments{}, Files{}, Folders{}, Snapshot{}, Release{}, RouteKey{ActiveReleaseID: 1}} {
		if validateSmartResult(v) == nil {
			t.Fatalf("accepted empty %T", v)
		}
	}
	bad := Files{Files: []File{{ID: 1, FolderID: 2, Title: "config", ObjType: "file", Type: "application/json", Source: "changed", SourceHash: strings.Repeat("a", 64)}}}
	if validateSmartResult(bad) == nil {
		t.Fatal("source/hash mismatch accepted")
	}
}
