package mcpserver

import (
	"bytes"
	"encoding/json"
)

// The list compactors below shrink the rows of the reads a Console run makes most, unless the call
// sends full true: every byte of a page rides in the model context on every later step of the run.
// A key that is dropped stays one full call away; none of them changes the type of a key it keeps.

// formKeys are the keys of a compact template row: what names it (id, title, ref), its kind (rows
// of type all carry it), its place in the template tree, its approval status, the Default marker and
// a denied stub's flag. Owner, permission flags and timestamps are full only (~85% of a row).
var formKeys = keySet("id", "title", "ref", "type", "parentId", "status", "isDefault", "accessDenied")

// formRows keeps formKeys of each template row, each only when set (a false flag, an empty string
// or null says nothing).
func formRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", func(row jsonObj) {
		for k, x := range row {
			if !formKeys[k] || unset(x) {
				delete(row, k)
			}
		}
	})
}

// reactionRows drops from each reaction row the Console's reasoning trace (treeInfo.reasoning, the
// step list an AI reply shows folded; most of an AI comment's bytes) and treeInfo's copy of the root
// actor the call named; treeInfo's own placement keys (name, parentId, extra, hidden) stay, and a
// reply still being written keeps reasoning {inProgress: true}.
func reactionRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", func(row jsonObj) {
		var tree jsonObj
		if json.Unmarshal(row["treeInfo"], &tree) != nil || tree == nil {
			return
		}
		var reasoning struct {
			InProgress bool `json:"inProgress"`
		}
		_ = json.Unmarshal(tree["reasoning"], &reasoning)
		for _, k := range []string{"reasoning", "rootActorId", "rootActorTitle", "rootActorFormId"} {
			delete(tree, k)
		}
		if reasoning.InProgress {
			tree["reasoning"] = json.RawMessage(`{"inProgress":true}`)
		}
		if len(tree) == 0 {
			delete(row, "treeInfo")
			return
		}
		row["treeInfo"], _ = json.Marshal(tree)
	})
}

// accountRows drops from each account row of one actor what a balance answer does not need: the
// actor's id (the call's own), owner, search flag and timestamps; holdAmount when 0 and
// availableAmount when it equals amount. What with asked for (meta, triggers, tags) stays.
func accountRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", func(row jsonObj) {
		for _, k := range []string{"actorId", "userId", "search", "createdAt", "updatedAt"} {
			delete(row, k)
		}
		if isZero(row["holdAmount"]) {
			delete(row, "holdAmount")
		}
		if a, ok := row["availableAmount"]; ok && sameNumber(a, row["amount"]) {
			delete(row, "availableAmount")
		}
	})
}

func keySet(keys ...string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}

// unset is a JSON value that carries nothing: null, false or "".
func unset(v json.RawMessage) bool {
	s := string(bytes.TrimSpace(v))
	return s == "null" || s == "false" || s == `""`
}

func isZero(v json.RawMessage) bool {
	var f float64
	return v != nil && json.Unmarshal(v, &f) == nil && f == 0
}

// sameNumber compares two JSON numbers as written: a float would round apart large or precise
// amounts that differ.
func sameNumber(a, b json.RawMessage) bool {
	x, y := bytes.TrimSpace(a), bytes.TrimSpace(b)
	return len(x) > 0 && bytes.Equal(x, y)
}
