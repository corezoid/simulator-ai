package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const (
	pairShared = `{"list":[{"userId":1,"privs":{"view":true}}]}`
	factRows   = `{"id":"1","result":{"list":[{"id":"c1","incomeType":"credit"},{"id":"d1","incomeType":"debit"}]}}`
)

func batchOf(items ...string) string {
	return `{"isAtomic":true,"list":[` + strings.Join(items, ",") + `]}`
}

func TestOpenAccountByNameAsUIKit(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "account_name/list":
			return `{"list":[{"id":"n-default","name":"Default"}]}`
		case "account_currency/list":
			return `{"list":[{"id":3,"name":"USD"}]}`
		case "access_rule/account/list":
			return pairShared
		case "batch":
			return batchOf(`{"id":"0","result":{}}`, factRows, `{"id":"2","result":{"existed":true,"list":[{"id":"c2","incomeType":"credit"},{"id":"d2","incomeType":"debit"}]}}`)
		}
		return `{}`
	})
	min := 0.0
	out, err := c.OpenAccount(context.Background(), AccountOpenInput{ActorID: "a1", PairRef: PairRef{AccountName: "Default", CurrencyName: "USD"}, Plan: true, MinLimit: &min})
	if err != nil {
		t.Fatal(err)
	}
	if out.NameID != "n-default" || out.CurrencyID != 3 || len(out.Accounts) != 2 || out.Accounts[0] != (OpenedAccount{AccountType: "fact", Credit: "c1", Debit: "d1"}) || out.Accounts[1] != (OpenedAccount{AccountType: "plan", Credit: "c2", Debit: "d2", Existed: true}) {
		t.Fatalf("%+v", out)
	}
	last, _ := json.Marshal((*calls)[len(*calls)-1].Params)
	for _, want := range []string{
		`"isAtomic":true`,
		`"method":"account/create","params":{"accId":"ws","currencyId":3,"nameId":"n-default"}`,
		`"method":"account_actor/create","params":{"accId":"ws","accountType":"fact","actorId":"a1","currencyId":3,"ignoreIfExist":true,"minLimit":0,"nameId":"n-default","search":true}`,
		`"method":"account_actor/create","params":{"accId":"ws","accountType":"plan","actorId":"a1","currencyId":3,"ignoreIfExist":true,"nameId":"n-default","search":true}`,
	} {
		if !strings.Contains(string(last), want) {
			t.Errorf("batch lacks %s: %s", want, last)
		}
	}
	for _, bad := range []AccountOpenInput{
		{ActorID: "a1"},
		{ActorID: "a1", PairRef: PairRef{AccountName: "Default", NameID: "n"}},
		{ActorID: "a1", PairRef: PairRef{NameID: "n"}},
		{ActorID: "a1", PairRef: PairRef{NameID: "n", CurrencyID: 3}, AccountType: "plan", Plan: true},
	} {
		if _, err := c.OpenAccount(context.Background(), bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestOpenStatisticalAccountSendsNoSearch(t *testing.T) {
	c, calls := rpcBackend(t, func(string) string { return batchOf(`{"id":"0","result":{}}`, factRows) })
	if _, err := c.OpenAccount(context.Background(), AccountOpenInput{ActorID: "a1", PairRef: PairRef{NameID: "n", CurrencyID: 3}, AccountType: "max"}); err != nil {
		t.Fatal(err)
	}
	if last, _ := json.Marshal((*calls)[0].Params); len(*calls) != 1 || strings.Contains(string(last), "search") || !strings.Contains(string(last), `"accountType":"max"`) {
		t.Fatal(string(last))
	}
}

func TestBindFormAccountOpensItOnExistingActors(t *testing.T) {
	batches := 0
	c, calls := rpcBackend(t, func(method string) string {
		switch method {
		case "account_form/list":
			return `{"list":[]}`
		case "actor/filter/list":
			return `{"list":[{"id":"a1"},{"id":"a2"},{"id":"a3"}]}`
		case "batch":
			batches++
			if batches == 1 {
				return batchOf(`{"id":"0","result":{}}`, `{"id":"1","result":{"id":41}}`)
			}
			return `{"list":[{"id":"0","result":{"list":[]}},{"id":"1","result":{"existed":true,"list":[]}},{"id":"2","error":{"code":1,"message":"no modify","data":{"reason":"accessDenied"}}}]}`
		}
		return `{}`
	})
	low, high := int64(-5), int64(500)
	out, err := c.BindFormAccount(context.Background(), FormAccountInput{FormID: 7, PairRef: PairRef{NameID: "n", CurrencyID: 3}, MinLimit: &low, MaxLimit: &high})
	if err != nil {
		t.Fatal(err)
	}
	if out.BindingID != 41 || out.WasBound || out.Opened != 1 || out.HadIt != 1 || len(out.Failed) != 1 || out.Failed[0].ActorID != "a3" || out.NextOffset != 0 {
		t.Fatalf("%+v", out)
	}
	got := strings.Join(methods(*calls), "\n")
	for _, want := range []string{
		`"method":"account_form/create","params":{"accId":"ws","accountType":"fact","currencyId":3,"formId":7,"maxLimit":500,"minLimit":-5,"nameId":"n","search":true}`,
		`actor/filter/list {"accId":"ws","fields":["id"],"formId":7,"limit":100,"offset":0,"orderBy":"createdAt","orderValue":"ASC"}`,
		// Existing actors get the binding's limits, as one created from the template does.
		`"method":"account_actor/create","params":{"accId":"ws","accountType":"fact","actorId":"a3","currencyId":3,"ignoreIfExist":true,"maxLimit":500,"minLimit":-5,"nameId":"n","search":true}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
}

func TestBindFormAccountKeepsABinding(t *testing.T) {
	c, calls := rpcBackend(t, func(method string) string {
		if method == "account_form/list" {
			return `{"list":[{"id":41}]}`
		}
		return `{}`
	})
	out, err := c.BindFormAccount(context.Background(), FormAccountInput{FormID: 7, PairRef: PairRef{NameID: "n", CurrencyID: 3}, SkipExisting: true})
	if err != nil || !out.WasBound || out.BindingID != 41 || len(*calls) != 1 {
		t.Fatal(out, err, methods(*calls))
	}
}
