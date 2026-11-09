package mcpserver

import (
	"encoding/json"
	"testing"
)

func TestFormRowsKeepNameKindAndDefault(t *testing.T) {
	in := `{"list":[{"id":71,"title":"Default","collapsed":false,"ownerId":1,"ownerName":"A","ownerSaId":45,"isTemplate":true,"isDefault":true,"createdAt":1,"updatedAt":1,"canModify":true,"canRemove":false},` +
		`{"id":2,"title":"Scripts","type":"system","picture":"/p.png","isDefault":false,"canModify":false,"ref":"","parentId":null},` +
		`{"id":9,"title":"Child","ref":"child","parentId":71,"status":"pending","accessDenied":true,"description":"long"}],"hasMore":true}`
	got := string(formRows(json.RawMessage(in)))
	if got != `{"hasMore":true,"list":[{"id":71,"isDefault":true,"title":"Default"},{"id":2,"title":"Scripts","type":"system"},`+
		`{"accessDenied":true,"id":9,"parentId":71,"ref":"child","status":"pending","title":"Child"}]}` {
		t.Fatal(got)
	}
	if string(formRows(json.RawMessage(`{"count":3}`))) != `{"count":3}` {
		t.Error("an answer without list changed")
	}
}

func TestReactionRowsDropReasoningAndRootCopy(t *testing.T) {
	in := `{"list":[{"id":"r1","description":"hi","treeInfo":{"reasoning":{"thoughts":[{"text":"step"}]},"rootActorId":"a","rootActorTitle":"A","rootActorFormId":3}},` +
		`{"id":"r2","treeInfo":{"rootActorId":"a","name":"comment","hidden":true,"extra":{"anchor":1}}},{"id":"r3"},` +
		`{"id":"r4","treeInfo":{"rootActorId":"a","reasoning":{"inProgress":true,"thoughts":[{"text":"step"}]}}}],"readTime":5}`
	got := string(reactionRows(json.RawMessage(in)))
	want := `{"list":[{"description":"hi","id":"r1"},{"id":"r2","treeInfo":{"extra":{"anchor":1},"hidden":true,"name":"comment"}},{"id":"r3"},{"id":"r4","treeInfo":{"reasoning":{"inProgress":true}}}],"readTime":5}`
	if got != want {
		t.Fatalf("%s\nwant %s", got, want)
	}
}

func TestAccountRowsDropNoiseKeepBalancesAndWith(t *testing.T) {
	in := `{"list":[{"id":"x","actorId":"a","userId":1,"type":"fact","nameId":"n","incomeType":"debit","currencyId":6,"amount":10,"holdAmount":0,"availableAmount":10,"status":"active","counterType":"systemCounter","createdAt":1,"updatedAt":1,"search":true,"meta":{"accountName":"Default"}},` +
		`{"id":"y","amount":10,"holdAmount":4,"availableAmount":6},{"id":"z","amount":12345678901234567.01,"availableAmount":12345678901234567.00}]}`
	got := string(accountRows(json.RawMessage(in)))
	want := `{"list":[{"amount":10,"counterType":"systemCounter","currencyId":6,"id":"x","incomeType":"debit","meta":{"accountName":"Default"},"nameId":"n","status":"active","type":"fact"},` +
		`{"amount":10,"availableAmount":6,"holdAmount":4,"id":"y"},{"amount":12345678901234567.01,"availableAmount":12345678901234567.00,"id":"z"}]}`
	if got != want {
		t.Fatalf("%s\nwant %s", got, want)
	}
}

func TestCompactListsTakeFull(t *testing.T) {
	seen := 0
	for _, tools := range [][]parityTool{formTools, reactionTools, accountTools} {
		for _, tool := range tools {
			if tool.Name != "forms_list" && tool.Name != "reactions_list" && tool.Name != "accounts_list" {
				continue
			}
			seen++
			raw := json.RawMessage(`{"list":[{"id":1,"createdAt":1,"treeInfo":{"reasoning":{}}}]}`)
			full, err := tool.Shape(map[string]json.RawMessage{"full": json.RawMessage("true")}, raw)
			if err != nil || string(full) != string(raw) {
				t.Errorf("%s full changed the answer: %s", tool.Name, full)
			}
			if compact, _ := tool.Shape(map[string]json.RawMessage{}, raw); string(compact) == string(raw) {
				t.Errorf("%s did not compact: %s", tool.Name, compact)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("checked %d of the 3 compacted lists", seen)
	}
}
