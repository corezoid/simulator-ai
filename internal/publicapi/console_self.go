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

// selfUserID is the caller's platform userId. From a Simulator JWT it is the member whose saId the
// token names (searched by its nick first, then the member list): trusted, and an argument that
// differs is refused. Any other credential takes the argument as it is, untrusted.
func (c *Client) selfUserID(ctx context.Context, claimed int64) (id int64, trusted bool, err error) {
	user, ok := c.credentialUser()
	if !ok {
		return claimed, false, nil
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
			return 0, 0, fmt.Errorf("user/list: %w", err)
		}
		for _, m := range page.List {
			if m.SAID == user.SAID && m.ID > 0 {
				return m.ID, len(page.List), nil
			}
		}
		return 0, len(page.List), nil
	}
	if user.Nick != "" {
		if id, _, err = find(user.Nick, 0); err != nil || id != 0 {
			return c.checkClaimed(id, claimed, err)
		}
	}
	for offset := 0; offset < 2000; offset += 200 {
		var n int
		if id, n, err = find("", offset); err != nil || id != 0 || n < 200 {
			break
		}
	}
	if err == nil && id == 0 {
		err = errors.New("the user your credential names is not a member of this workspace")
	}
	return c.checkClaimed(id, claimed, err)
}

func (c *Client) checkClaimed(id, claimed int64, err error) (int64, bool, error) {
	switch {
	case err != nil:
		return 0, false, err
	case claimed != 0 && claimed != id:
		return 0, false, fmt.Errorf("selfUserId %d is not you: you are user %d", claimed, id)
	}
	return id, true, nil
}

// isParticipant says whether the user holds an access rule on the actor (its owner included).
func (c *Client) isParticipant(ctx context.Context, actorID string, userID int64) (bool, error) {
	rules, err := call[map[string]any, struct {
		List []struct {
			UserID int64 `json:"userId"`
		} `json:"list"`
	}](ctx, c, "access_rule/actor/list", map[string]any{"accId": c.config.Workspace, "actorId": actorID})
	if err != nil {
		return false, fmt.Errorf("access_rule/actor/list: %w", err)
	}
	for _, r := range rules.List {
		if r.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}
