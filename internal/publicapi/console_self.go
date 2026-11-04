package publicapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// No public method says who the caller is. A "Simulator <jwt>" credential (the AI Console run's,
// minted by sim-api sajwt in pong-server's shape) names its user in its claims: sub is the
// SuperAdmin id, nick the display name. The payload is read without checking the signature, only
// to learn whose token it is: the server authenticates every call with it anyway, and nothing here
// is granted on the claim. The token itself never leaves the process.

type tokenUser struct {
	SAID int64
	Nick string
}

// credentialUser is the user a Simulator JWT names; ok false for any other credential.
func (c *Client) credentialUser() (tokenUser, bool) {
	scheme, token, found := strings.Cut(c.config.Authorization, " ")
	if !found || scheme != "Simulator" {
		return tokenUser{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenUser{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenUser{}, false
	}
	var claims struct {
		Sub  string `json:"sub"`
		Nick string `json:"nick"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return tokenUser{}, false
	}
	id, err := strconv.ParseInt(claims.Sub, 10, 64)
	if err != nil || id < 1 {
		return tokenUser{}, false
	}
	return tokenUser{SAID: id, Nick: claims.Nick}, true
}

type memberRow struct {
	ID   int64 `json:"id"`
	SAID int64 `json:"saId"`
}

// caller is who chat_open acts as. Trusted: the id came from the credential's own member row.
// Otherwise ID is the selfUserId the model passed, and SAID, when the credential names one, still
// tells whose rule on a chat is the caller's.
type caller struct {
	ID      int64
	Trusted bool
	SAID    int64
}

// selfUserID is the caller's platform userId. From a Simulator JWT it is the member whose saId the
// token names (searched by its nick first, then the member list): trusted, and an argument that
// differs is refused. user/list needs the viewUsers permission, which an ordinary member lacks: on
// accessDenied, and with any other credential, the argument is taken as it is, untrusted.
func (c *Client) selfUserID(ctx context.Context, claimed int64) (caller, error) {
	user, ok := c.credentialUser()
	if !ok {
		return caller{ID: claimed}, nil
	}
	find := func(query string, offset int) (int64, int, error) {
		params := map[string]any{"accId": c.config.Workspace, "limit": 200, "offset": offset}
		if query != "" {
			params["query"] = query
		}
		page, err := call[map[string]any, struct {
			List []memberRow `json:"list"`
		}](ctx, c, "user/list", params)
		if err != nil {
			return 0, 0, err
		}
		for _, m := range page.List {
			if m.SAID == user.SAID && m.ID > 0 {
				return m.ID, len(page.List), nil
			}
		}
		return 0, len(page.List), nil
	}
	var id int64
	var err error
	if user.Nick != "" {
		id, _, err = find(user.Nick, 0)
	}
	for offset := 0; err == nil && id == 0 && offset < 2000; offset += 200 {
		var n int
		if id, n, err = find("", offset); n < 200 {
			break
		}
	}
	var api *APIError
	switch {
	case errors.As(err, &api) && api.Reason == "accessDenied":
		return caller{ID: claimed, SAID: user.SAID}, nil
	case err != nil:
		return caller{}, fmt.Errorf("user/list: %w", err)
	case id == 0:
		return caller{}, errors.New("the user your credential names is not a member of this workspace")
	case claimed != 0 && claimed != id:
		return caller{}, fmt.Errorf("selfUserId %d is not you: you are user %d", claimed, id)
	}
	return caller{ID: id, Trusted: true, SAID: user.SAID}, nil
}

// isParticipant says whether the caller holds an access rule on the actor (its owner included):
// the rule of its userId and, when the credential names a SuperAdmin id, of that saId too.
func (c *Client) isParticipant(ctx context.Context, actorID string, who caller) (bool, error) {
	rules, err := call[map[string]any, struct {
		List []struct {
			UserID int64 `json:"userId"`
			SAID   int64 `json:"saId"`
		} `json:"list"`
	}](ctx, c, "access_rule/actor/list", map[string]any{"accId": c.config.Workspace, "actorId": actorID})
	if err != nil {
		return false, fmt.Errorf("access_rule/actor/list: %w", err)
	}
	for _, r := range rules.List {
		if r.UserID == who.ID && (who.SAID == 0 || r.SAID == who.SAID) {
			return true, nil
		}
	}
	return false, nil
}
