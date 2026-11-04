package publicapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCreateAccountPairByName(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "account_name/create":
			return `{"id":"name-7","name":"Revenue"}`
		case "account_currency/create":
			return `{"id":3,"name":"USD"}`
		}
		return `{}`
	})
	two := int64(2)
	pair, err := c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue", CurrencyName: "USD", Symbol: "$", Precision: &two, Type: "currency"})
	if err != nil || pair.NameID != "name-7" || pair.CurrencyID != 3 {
		t.Fatal(pair, err)
	}
	var sent []string
	for _, call := range *calls {
		params, _ := json.Marshal(call.Params)
		sent = append(sent, call.Method+" "+string(params))
	}
	want := []string{
		`account_name/create {"accId":"ws","name":"Revenue"}`,
		`account_currency/create {"accId":"ws","name":"USD","precision":2,"symbol":"$","type":"currency"}`,
		`account/create {"accId":"ws","currencyId":3,"nameId":"name-7"}`,
	}
	if len(sent) != len(want) {
		t.Fatal(sent)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Errorf("call %d: %s, want %s", i, sent[i], want[i])
		}
	}
	if _, err = c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue"}); err == nil {
		t.Error("a pair without a currency accepted")
	}
}
