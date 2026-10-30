package publicapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func validHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == s
}

// Reject malformed success envelopes rather than letting zero-valued ids drive
// the next write. Unknown response fields are dropped by the named projections.
func validateSmartResult(value any) error {
	invalid := errors.New("invalid Smart Form result")
	switch r := value.(type) {
	case CreatedApplication:
		if r.Application.ID == nil || *r.Application.ID == "" || r.Application.Ref == nil || *r.Application.Ref == "" || len(r.Envs) != 2 {
			return invalid
		}
		seen := map[int64]bool{}
		for _, e := range r.Envs {
			if !validEnvironment(e) || e.ActorID != *r.Application.ID || seen[e.ID] {
				return invalid
			}
			seen[e.ID] = true
		}
	case Environments:
		if r.Envs == nil {
			return invalid
		}
		for _, e := range r.Envs {
			if !validEnvironment(e) {
				return invalid
			}
		}
	case Snapshot:
		if !validHash(r.EnvironmentHash) || r.Tree.ObjType != "folder" || !validContentNode(r.Tree, 0) {
			return invalid
		}
	case Files:
		if len(r.Files) == 0 {
			return invalid
		}
		seen := map[int64]bool{}
		for _, f := range r.Files {
			sum := sha256.Sum256([]byte(f.Source))
			if f.ID < 1 || f.FolderID < 1 || f.ObjType != "file" || f.Title == "" || f.Type == "" || f.SourceHash != hex.EncodeToString(sum[:]) || seen[f.ID] {
				return invalid
			}
			seen[f.ID] = true
		}
	case Folders:
		if len(r.Folders) == 0 {
			return invalid
		}
		seen := map[int64]bool{}
		for _, f := range r.Folders {
			if f.ID < 1 || f.FolderID < 1 || f.ObjType != "folder" || f.Title == "" || seen[f.ID] {
				return invalid
			}
			seen[f.ID] = true
		}
	case Release:
		if r.ID < 1 || r.ActorID == "" || r.EnvID < 1 {
			return invalid
		}
		switch r.Status {
		case "active", "archived", "rolled_back", "failed":
		default:
			return invalid
		}
	case RouteKey:
		if r.ActiveReleaseID < 0 || (r.ActiveReleaseID > 0 && r.AppKey == "") {
			return invalid
		}
	default:
		return invalid
	}
	return nil
}
func validEnvironment(e Environment) bool {
	return e.ID > 0 && e.ActorID != "" && e.Title != "" && e.RootFolderID > 0
}
func validContentNode(n ContentNode, depth int) bool {
	if depth > 128 || n.ID < 1 || n.Title == "" {
		return false
	}
	switch n.ObjType {
	case "folder":
		for _, child := range n.Children {
			if !validContentNode(child, depth+1) {
				return false
			}
		}
	case "file":
		source := ""
		if n.Source != nil {
			source = *n.Source
		}
		sum := sha256.Sum256([]byte(source))
		if n.Type == "" || n.SourceHash != hex.EncodeToString(sum[:]) || len(n.Children) > 0 {
			return false
		}
	default:
		return false
	}
	return true
}
