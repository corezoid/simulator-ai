package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAccountURL is the production SuperAdmin.
	DefaultAccountURL = "https://account.corezoid.com"
	// DefaultClientID is the public PKCE client the legacy Simulator plugin registered in SuperAdmin.
	DefaultClientID = "5ec679f5a2710f0da6000005"

	maxTokenResponse = 64 << 10
	// noExpiryWindow bounds a token without an exp claim, so it is re-validated, never trusted forever.
	noExpiryWindow = 12 * time.Hour
)

// provider talks to SuperAdmin's OAuth2 token endpoint.
type provider struct {
	accountURL string
	clientID   string
	http       *http.Client
}

// errRejected is a refusal by SuperAdmin (invalid or revoked grant), as opposed to a transport failure.
type errRejected struct{ code string }

func (e errRejected) Error() string {
	return "the account service refused the sign-in (" + e.code + ")"
}

type tokenResponse struct {
	SimulatorToken string `json:"simulator_token"`
	RefreshToken   string `json:"refresh_token"`
	Error          string `json:"error"`
}

func (p provider) authorizeURL(redirect, challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", redirect)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	return p.accountURL + "/oauth2/authorize?" + q.Encode()
}

func (p provider) exchange(ctx context.Context, code, verifier, redirect string) (Entry, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", p.clientID)
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", redirect)
	return p.token(ctx, form)
}

func (p provider) refresh(ctx context.Context, refreshToken string) (Entry, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", p.clientID)
	form.Set("refresh_token", refreshToken)
	return p.token(ctx, form)
}

// token posts one grant. Neither the response body nor a token ever enters an error.
func (p provider) token(ctx context.Context, form url.Values) (Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.accountURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Entry{}, errors.New("invalid account service request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Entry{}, ctx.Err()
		}
		return Entry{}, errors.New("the account service is unreachable")
	}
	defer func() { _ = resp.Body.Close() }() // Read-only response cleanup.
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponse+1))
	if err != nil || len(b) > maxTokenResponse {
		return Entry{}, errors.New("cannot read the account service response")
	}
	// A refusal of the grant itself (an OAuth error with 200/400/401/403) is final; 408, 429 and 5xx
	// are worth another try later and never end the session.
	var tr tokenResponse
	decoded := json.Unmarshal(b, &tr) == nil
	// A non-JSON 4xx is a proxy or WAF page, not SuperAdmin's answer about the grant.
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		if decoded {
			return Entry{}, errRejected{safeCode(tr.Error, resp.StatusCode)}
		}
	case http.StatusOK:
		if decoded && tr.Error != "" {
			return Entry{}, errRejected{safeCode(tr.Error, resp.StatusCode)}
		}
	}
	if !decoded || tr.Error != "" {
		return Entry{}, errors.New("the account service is unavailable (http " + strconv.Itoa(resp.StatusCode) + ")")
	}
	if resp.StatusCode != http.StatusOK || tr.SimulatorToken == "" || strings.ContainsAny(tr.SimulatorToken, " \t\r\n") {
		return Entry{}, errors.New("the account service returned no Simulator token (http " + strconv.Itoa(resp.StatusCode) + ")")
	}
	e := Entry{AccessToken: tr.SimulatorToken, RefreshToken: tr.RefreshToken, ExpiresAt: time.Now().Add(noExpiryWindow)}
	if c, ok := parseClaims(tr.SimulatorToken); ok && !c.exp.IsZero() {
		e.ExpiresAt = c.exp
	}
	return e, nil
}

// safeCode keeps an OAuth error code printable: letters, digits and _ only, bounded.
func safeCode(code string, status int) string {
	var b strings.Builder
	for _, r := range code {
		if b.Len() >= 64 {
			break
		}
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "http " + strconv.Itoa(status)
	}
	return b.String()
}

// UserOf reads the SuperAdmin id (sub) and nick of a Simulator JWT without checking its signature;
// ok false unless sub is a positive id.
func UserOf(token string) (int64, string, bool) {
	c, ok := parseClaims(token)
	return c.sub, c.nick, ok && c.sub > 0
}

type claims struct {
	sub  int64
	nick string
	exp  time.Time
}

// parseClaims reads a JWT payload without checking the signature: only to show whose token it is
// and when it expires. Nothing is granted on it; the API authenticates every request itself.
func parseClaims(token string) (claims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}, false
	}
	var raw struct {
		Sub  json.RawMessage `json:"sub"`
		Nick string          `json:"nick"`
		Exp  float64         `json:"exp"`
	}
	if json.Unmarshal(payload, &raw) != nil {
		return claims{}, false
	}
	c := claims{nick: raw.Nick}
	sub := strings.Trim(string(raw.Sub), `"`)
	if id, err := strconv.ParseInt(sub, 10, 64); err == nil && id > 0 {
		c.sub = id
	}
	if raw.Exp > 0 {
		c.exp = time.Unix(int64(raw.Exp), 0)
	}
	return c, true
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("cannot generate a random login secret")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func challengeOf(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
