package publicapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// A layer's settings are keys of the layer actor's own data (a Layers system-form actor): the
// Graph Editor's settings popover writes them with actor/update, which sim-api merges one level
// deep (a sent key replaces the stored one, the rest stay). No server schema types them; the
// writers are pong-front-end's (CommonLayer.js, UserSettingsBtn.js, LayerAccounts.js) and
// simulator-ui-kit graph-layer server/settings/layerData.ts, whose shapes these follow:
//   - defForm [{title, value: formId, type: "form"}], [] clears: the template new canvas objects get.
//   - edgeCurveStyle [{value}] (legacy also writes title): the default shape of the layer's links;
//     the Layers form offers curved, rounded and straight, both editors also roundedDownward.
//   - anisotropy (default true), outerEdges (default false): booleans.
//   - initX, initY: the point the layer opens centred on, graph units.
//   - accounts [{value: "<nameId>_<currencyId>", title: "<name>, <currency>", ...}]: the balances
//     drawn on its actors (sim-api layer_balances.go reads value and title, at most 32); the old
//     accountName x currency pair of lists is cleared to null on write, as both editors do.
//     ui-kit adds pair fields beside value and title; nameId, accountName, currencyId and
//     currencyName are written here too, its currency metadata is not (its reader fills it).
//   - accountsPeriod {startDate?, endDate?, timeZoneOffset}: the balances' window, Unix seconds.
//     A cleared period is {timeZoneOffset} alone, never {} (layerData.ts encodeAccountsPeriod).
//   - maxVisibleChildren: "show at most N children" (CE-16245), one of 0 (off), 10, 25, 50
//     (ui-kit MAX_VISIBLE_CHILDREN_OPTIONS; sim-api e_graph.MaxVisibleChildrenOptions): an actor with
//     more direct children shows the first N and folds the rest for every viewer. Anything else
//     stored there reads as off.

type LayerSettingsInput struct {
	LayerID string `json:"layerId"`
	// DefaultFormID sets defForm; 0 clears it.
	DefaultFormID  *int64          `json:"defaultFormId,omitempty"`
	EdgeCurveStyle string          `json:"edgeCurveStyle,omitempty"`
	Anisotropy     *bool           `json:"anisotropy,omitempty"`
	OuterEdges     *bool           `json:"outerEdges,omitempty"`
	InitialCell    string          `json:"initialCell,omitempty"`
	Accounts       *[]LayerAccount `json:"accounts,omitempty"`
	AccountsPeriod *LayerPeriod    `json:"accountsPeriod,omitempty"`
	// MaxVisibleChildren is the layer's fan limit; 0 turns it off.
	MaxVisibleChildren *int `json:"maxVisibleChildren,omitempty"`
}

// LayerAccount names an account pair shown on the layer's actors.
type LayerAccount struct {
	AccountName string `json:"accountName"`
	Currency    string `json:"currency"`
}

// LayerPeriod is the balances' window in Unix seconds; neither bound clears it.
type LayerPeriod struct {
	StartDate      *int64 `json:"startDate,omitempty"`
	EndDate        *int64 `json:"endDate,omitempty"`
	TimeZoneOffset int    `json:"timeZoneOffset,omitempty"`
}

type LayerSettingsSaved struct {
	LayerID string   `json:"layerId"`
	Keys    []string `json:"keys"`
}

// maxLayerAccounts is what sim-api draws (layer_balances.go MaxLayerFinancialAccountPairs).
const maxLayerAccounts = 32

var curveStyles = []string{"curved", "rounded", "roundedDownward", "straight"}

// maxVisibleChildrenOptions are the fan limits the Graph Editor offers and reads (0 is off).
var maxVisibleChildrenOptions = []int{0, 10, 25, 50}

// UpdateLayerSettings writes the sent settings into the layer actor's data in one actor/update.
func (c *Client) UpdateLayerSettings(ctx context.Context, in LayerSettingsInput) (LayerSettingsSaved, error) {
	if e := consoleOnly(c); e != nil {
		return LayerSettingsSaved{}, e
	}
	if !validID(in.LayerID) {
		return LayerSettingsSaved{}, errors.New("layerId required")
	}
	ws := c.workspace()
	data := map[string]any{}
	if in.DefaultFormID != nil {
		switch id := *in.DefaultFormID; {
		case id == 0:
			data["defForm"] = []any{}
		case id > 0:
			form, err := call[map[string]any, struct {
				Title string `json:"title"`
			}](ctx, c, "form/get", map[string]any{"accId": ws, "formId": id})
			if err != nil {
				return LayerSettingsSaved{}, fmt.Errorf("form/get %d: %w", id, err)
			}
			data["defForm"] = []any{map[string]any{"title": form.Title, "value": id, "type": "form"}}
		default:
			return LayerSettingsSaved{}, errors.New("defaultFormId: a template id, or 0 to clear")
		}
	}
	if in.EdgeCurveStyle != "" {
		if !slices.Contains(curveStyles, in.EdgeCurveStyle) {
			return LayerSettingsSaved{}, errors.New("edgeCurveStyle is curved, rounded, roundedDownward or straight")
		}
		data["edgeCurveStyle"] = []any{map[string]any{"value": in.EdgeCurveStyle}}
	}
	if in.Anisotropy != nil {
		data["anisotropy"] = *in.Anisotropy
	}
	if in.OuterEdges != nil {
		data["outerEdges"] = *in.OuterEdges
	}
	if in.InitialCell != "" {
		at, err := CellCenter(in.InitialCell)
		if err != nil {
			return LayerSettingsSaved{}, err
		}
		data["initX"], data["initY"] = at.X, at.Y
	}
	if in.Accounts != nil {
		entries, err := c.layerAccounts(ctx, *in.Accounts)
		if err != nil {
			return LayerSettingsSaved{}, err
		}
		data["accounts"], data["accountName"], data["currency"] = entries, nil, nil
	}
	if p := in.AccountsPeriod; p != nil {
		if p.StartDate != nil && p.EndDate != nil && *p.EndDate < *p.StartDate {
			return LayerSettingsSaved{}, errors.New("accountsPeriod: endDate before startDate")
		}
		period := map[string]any{"timeZoneOffset": p.TimeZoneOffset}
		if p.StartDate != nil {
			period["startDate"] = *p.StartDate
		}
		if p.EndDate != nil {
			period["endDate"] = *p.EndDate
		}
		data["accountsPeriod"] = period
	}
	if v := in.MaxVisibleChildren; v != nil {
		if !slices.Contains(maxVisibleChildrenOptions, *v) {
			return LayerSettingsSaved{}, errors.New("maxVisibleChildren is 0 (off), 10, 25 or 50")
		}
		data["maxVisibleChildren"] = *v
	}
	if len(data) == 0 {
		return LayerSettingsSaved{}, errors.New("nothing to change: send at least one setting")
	}
	if _, err := call[map[string]any, named](ctx, c, "actor/update", map[string]any{"accId": ws, "actorId": in.LayerID, "data": data, "fields": []string{"id"}}); err != nil {
		return LayerSettingsSaved{}, fmt.Errorf("actor/update: %w", err)
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return LayerSettingsSaved{LayerID: in.LayerID, Keys: keys}, nil
}

// layerAccounts resolves account names and currencies to the stored entries, in order.
func (c *Client) layerAccounts(ctx context.Context, accounts []LayerAccount) ([]any, error) {
	if len(accounts) > maxLayerAccounts {
		return nil, fmt.Errorf("at most %d accounts are drawn on a layer", maxLayerAccounts)
	}
	out := make([]any, 0, len(accounts))
	seen := map[string]bool{}
	for i, a := range accounts {
		if a.AccountName == "" || a.Currency == "" {
			return nil, fmt.Errorf("accounts[%d]: accountName and currency required", i)
		}
		nameID, err := lookup[string](ctx, c, "account_name/list", a.AccountName)
		if err != nil {
			return nil, fmt.Errorf("account_name/list: %w", err)
		}
		currencyID, err := lookup[int64](ctx, c, "account_currency/list", a.Currency)
		if err != nil {
			return nil, fmt.Errorf("account_currency/list: %w", err)
		}
		if nameID == "" || currencyID == 0 {
			return nil, fmt.Errorf("accounts[%d]: no account name %q or currency %q in the workspace (account_names_list, currencies_list)", i, a.AccountName, a.Currency)
		}
		value := nameID + "_" + strconv.FormatInt(currencyID, 10)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, map[string]any{"value": value, "title": a.AccountName + ", " + a.Currency,
			"nameId": nameID, "accountName": a.AccountName, "currencyId": currencyID, "currencyName": a.Currency})
	}
	return out, nil
}
