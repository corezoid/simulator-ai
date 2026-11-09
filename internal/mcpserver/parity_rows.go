package mcpserver

import "encoding/json"

// The list compactors below shrink the rows of the reads a Console run makes most, unless the call
// sends full true: every byte of a page rides in the model context on every later step of the run.
// A key that is dropped stays one full call away; none of them changes the type of a key it keeps.

// formRows keeps of each template row what names it: id, title, its kind (rows of type all carry
// it) and the Default marker; owner, permission flags and timestamps are full only (~85% of a row).
func formRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", func(row jsonObj) {
		keepOnly(row, "id", "title", "type", "isDefault")
		if string(row["isDefault"]) != "true" {
			delete(row, "isDefault")
		}
	})
}

// reactionRows drops from each reaction row the Console's reasoning trace (treeInfo.reasoning, the
// step list an AI reply shows folded; most of an AI comment's bytes) and treeInfo's copy of the root
// actor the call named; treeInfo's own placement keys (name, parentId, extra, hidden) stay.
func reactionRows(v json.RawMessage) json.RawMessage {
	return eachRow(v, "list", func(row jsonObj) {
		var tree jsonObj
		if json.Unmarshal(row["treeInfo"], &tree) != nil || tree == nil {
			return
		}
		for _, k := range []string{"reasoning", "rootActorId", "rootActorTitle", "rootActorFormId"} {
			delete(tree, k)
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

// keepOnly deletes every key of row outside keys.
func keepOnly(row jsonObj, keys ...string) {
	keep := make(map[string]bool, len(keys))
	for _, k := range keys {
		keep[k] = true
	}
	for k := range row {
		if !keep[k] {
			delete(row, k)
		}
	}
}

func isZero(v json.RawMessage) bool {
	var f float64
	return v != nil && json.Unmarshal(v, &f) == nil && f == 0
}

func sameNumber(a, b json.RawMessage) bool {
	var x, y float64
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && x == y
}
