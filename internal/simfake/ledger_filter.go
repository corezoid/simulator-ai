package simfake

import "encoding/json"

func init() { kbMethods["transaction/filter"] = (*Server).transactionFilter }

// transactionFilter answers the stored postings of the named pairs and account type, newest first,
// each with its actor and side (the row it went to); the window and paging are not modelled.
func (s *Server) transactionFilter(params json.RawMessage) (any, *rpcError) {
	var p struct {
		AccountType string `json:"accountType"`
		Accounts    []struct {
			NameID     string `json:"nameId"`
			CurrencyID int64  `json:"currencyId"`
		} `json:"accounts"`
	}
	_ = json.Unmarshal(params, &p)
	if p.AccountType == "" {
		p.AccountType = "fact"
	}
	list := []any{}
	for i := len(s.Ledger.Postings) - 1; i >= 0; i-- {
		t := s.Ledger.Postings[i]
		for _, a := range s.Ledger.Accounts {
			side := map[string]string{a.Credit: "credit", a.Debit: "debit"}[t.AccountID]
			if side == "" || a.Type != p.AccountType {
				continue
			}
			for _, want := range p.Accounts {
				if want.NameID == a.NameID && want.CurrencyID == a.CurrencyID {
					list = append(list, map[string]any{"id": t.ID, "actorId": a.ActorID, "accountId": t.AccountID, "incomeType": side, "amount": t.Amount, "transferId": t.TransferID, "type": "completed"})
				}
			}
		}
	}
	return map[string]any{"list": list, "hasMore": false}, nil
}
