package publicapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// pairBackend answers the dictionary lookups with the existing entries and the pair's rules; empty
// rules answer as sim-api does for a pair nobody was given access to, with notFound.
func pairBackend(t *testing.T, existing bool, rules string) (*Client, *[]rpcCall) {
	return rpcBackend(t, func(method string) string {
		switch {
		case method == "access_rule/account/list" && rules == "":
			return notFound
		case method == "access_rule/account/list":
			return rules
		case method == "account_name/list" && existing:
			return `{"list":[{"id":"name-0","name":"Revenue (old)"},{"id":"name-7","name":"revenue"}]}`
		case method == "account_currency/list" && existing: // the exact case wins over usd
			return `{"list":[{"id":2,"name":"usd"},{"id":3,"name":"USD"}]}`
		case strings.HasSuffix(method, "/list"):
			return `{"list":[]}`
		case method == "account_name/create":
			return `{"id":"name-9","name":"Revenue"}`
		case method == "account_currency/create":
			return `{"id":4,"name":"USD"}`
		}
		return `{}`
	})
}

func methods(calls []rpcCall) []string {
	var out []string
	for _, c := range calls {
		params, _ := json.Marshal(c.Params)
		out = append(out, c.Method+" "+string(params))
	}
	return out
}

func TestCreateAccountPairCreatesWhatIsMissing(t *testing.T) {
	c, calls := pairBackend(t, false, "")
	two := int64(2)
	pair, err := c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue", CurrencyName: "USD", Symbol: "$", Precision: &two, Type: "currency"})
	if err != nil || pair != (AccountPair{NameID: "name-9", CurrencyID: 4, Registered: true}) {
		t.Fatal(pair, err)
	}
	got := strings.Join(methods(*calls), "\n")
	for _, want := range []string{
		`account_name/create {"accId":"ws","name":"Revenue"}`,
		`account_currency/create {"accId":"ws","name":"USD","precision":2,"symbol":"$","type":"currency"}`,
		`account/create {"accId":"ws","currencyId":4,"nameId":"name-9"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
	if _, err = c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue"}); err == nil {
		t.Error("a pair without a currency accepted")
	}
}

// An existing shared pair is used as it is: nothing is created or registered again.
func TestCreateAccountPairKeepsASharedPair(t *testing.T) {
	c, calls := pairBackend(t, true, `{"list":[{"userId":5,"privs":{"view":true,"modify":true}}]}`)
	pair, err := c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue", CurrencyName: "USD"})
	if err != nil || pair != (AccountPair{NameID: "name-7", CurrencyID: 3}) {
		t.Fatal(pair, err)
	}
	for _, m := range methods(*calls) {
		if strings.Contains(m, "/create") {
			t.Error("called", m)
		}
	}
}

// An existing pair nobody was given access to is not claimed unless the caller says so.
func TestCreateAccountPairDoesNotClaimAnUnsharedPair(t *testing.T) {
	c, calls := pairBackend(t, true, "")
	if _, err := c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue", CurrencyName: "USD"}); err == nil || !strings.Contains(err.Error(), "takeOwnership") {
		t.Fatal("an unshared pair claimed:", err)
	}
	for _, m := range methods(*calls) {
		if strings.Contains(m, "/create") {
			t.Error("called", m)
		}
	}
	*calls = nil
	pair, err := c.CreateAccountPair(context.Background(), AccountPairInput{AccountName: "Revenue", CurrencyName: "USD", TakeOwnership: true})
	if err != nil || !pair.Registered || methods(*calls)[len(*calls)-1] != `account/create {"accId":"ws","currencyId":3,"nameId":"name-7"}` {
		t.Fatal(pair, err, methods(*calls))
	}
}
