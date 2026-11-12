package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const canary = "secret-canary-token"

func jwt(sub int64, nick string, exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	payload, _ := json.Marshal(map[string]any{"sub": fmt.Sprint(sub), "nick": nick, "exp": exp.Unix()})
	return enc([]byte(`{"alg":"HS256"}`)) + "." + enc(payload) + "." + enc([]byte(canary))
}

// fakeSA is SuperAdmin's token endpoint: it checks PKCE and answers grants from its fields.
type fakeSA struct {
	t         *testing.T
	mu        sync.Mutex
	challenge string
	redirect  string
	token     string
	refresh   string
	refreshes int
	status    int
	body      string
	during    func() // runs while a refresh is in flight
}

func (f *fakeSA) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/oauth2/token" || r.ParseForm() != nil || r.PostForm.Get("client_id") != DefaultClientID {
		f.t.Error("unexpected token request")
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if r.PostForm.Get("code") != "code-1" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.PostForm.Get("redirect_uri") != f.redirect {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
	case "refresh_token":
		f.refreshes++
		if f.during != nil {
			f.during()
		}
		if r.PostForm.Get("refresh_token") != "refresh-1" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"` + canary + `"}`))
			return
		}
	default:
		f.t.Error("unexpected grant")
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"simulator_token": f.token, "refresh_token": f.refresh})
}

func newTestSession(t *testing.T) (*Session, *fakeSA, string) {
	t.Helper()
	sa := &fakeSA{t: t, token: jwt(42, "Ada", time.Now().Add(time.Hour)), refresh: "refresh-1"}
	srv := httptest.NewServer(http.HandlerFunc(sa.serve))
	t.Cleanup(srv.Close)
	dir := filepath.Join(t.TempDir(), "simulator-ai")
	s := NewSession(Options{Origin: "https://api.example", ConfigDir: dir, AccountURL: srv.URL})
	s.open = func(string) error { return errors.New("no browser in tests") }
	return s, sa, dir
}

// browser completes the sign-in the way SuperAdmin's redirect would; extra callbacks go first.
func browser(t *testing.T, sa *fakeSA, before ...string) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil || u.Path != "/oauth2/authorize" {
			t.Error("bad authorize URL")
		}
		q := u.Query()
		sa.mu.Lock()
		sa.challenge, sa.redirect = q.Get("code_challenge"), q.Get("redirect_uri")
		sa.mu.Unlock()
		if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != DefaultClientID || !strings.HasPrefix(sa.redirect, "http://localhost:") {
			t.Error("authorize URL lacks PKCE")
		}
		go func() {
			for _, query := range append(before, "code=code-1&state="+url.QueryEscape(q.Get("state"))) {
				resp, err := http.Get(sa.redirect + "?" + query)
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}()
		return nil
	}
}

func TestLoginExchangesCodeAndStoresPrivately(t *testing.T) {
	s, sa, dir := newTestSession(t)
	// A forged callback without this login's state must neither abort nor complete it.
	s.open = browser(t, sa, "code=forged&state=wrong")
	res, err := s.Login(context.Background(), 10*time.Second)
	if err != nil || res.Status != "signedIn" || !res.BrowserOpened || res.AuthorizationURL != "" || res.User.SAID != 42 || res.User.Nick != "Ada" {
		t.Fatalf("login: %+v %v", res, err)
	}
	h, err := s.Authorization(context.Background())
	if err != nil || h != "Simulator "+sa.token {
		t.Fatalf("authorization: %v", err)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dir: 0o700, s.StorePath(): 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("%s mode %v %v", path, info.Mode().Perm(), err)
			}
		}
	}
	encoded, _ := json.Marshal(res)
	if strings.Contains(string(encoded), sa.token) || strings.Contains(string(encoded), "refresh-1") {
		t.Fatal("login result leaks a token")
	}
}

func TestLoginPendingCompletesInBackground(t *testing.T) {
	s, sa, _ := newTestSession(t)
	var authURL string
	s.open = func(u string) error { authURL = u; return errors.New("cannot open") }
	res, err := s.Login(context.Background(), 10*time.Millisecond)
	if err != nil || res.Status != "pending" || res.BrowserOpened || res.AuthorizationURL != authURL || !res.User.Pending {
		t.Fatalf("pending: %+v %v", res, err)
	}
	// A second login rejoins the same sign-in instead of starting another.
	again, err := s.Login(context.Background(), 10*time.Millisecond)
	if err != nil || again.AuthorizationURL != authURL {
		t.Fatal("second login started a new flow")
	}
	_ = browser(t, sa)(authURL)
	deadline := time.Now().Add(10 * time.Second)
	for {
		st, _ := s.Status()
		if st.SignedIn {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background sign-in not saved")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRefreshBeforeExpiry(t *testing.T) {
	s, sa, _ := newTestSession(t)
	if err := s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", time.Now().Add(30*time.Second)), RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(30 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	sa.refresh = "" // SuperAdmin may omit a new refresh token: the old one is kept.
	h, err := s.Authorization(context.Background())
	if err != nil || h != "Simulator "+sa.token || sa.refreshes != 1 {
		t.Fatalf("refresh: %v %d", err, sa.refreshes)
	}
	e, _ := s.store.Get(s.origin)
	if e.RefreshToken != "refresh-1" || !s.fresh(e) {
		t.Fatal("renewed entry not stored")
	}
	if _, err = s.Authorization(context.Background()); err != nil || sa.refreshes != 1 {
		t.Fatal("fresh token renewed again")
	}
}

func TestExpiryAndRejectedRefresh(t *testing.T) {
	s, sa, _ := newTestSession(t)
	old := time.Now().Add(-time.Minute)
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", old), ExpiresAt: old})
	_ = s.SetWorkspace("ws-1")
	if _, err := s.Authorization(context.Background()); !errors.Is(err, ErrLoginRequired) || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired without refresh: %v", err)
	}
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", old), RefreshToken: "revoked", ExpiresAt: old})
	_, err := s.Authorization(context.Background())
	if !errors.Is(err, ErrLoginRequired) || strings.Contains(err.Error(), canary) || sa.refreshes != 1 {
		t.Fatalf("rejected refresh: %v", err)
	}
	if e, _ := s.store.Get(s.origin); e.AccessToken != "" || e.Workspace != "ws-1" {
		t.Fatal("rejected refresh must drop the token and keep the workspace")
	}
	if _, err = s.Authorization(context.Background()); !errors.Is(err, ErrLoginRequired) || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("after drop: %v", err)
	}
}

func TestTokenEndpointFailuresNeverEchoBodies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{500, `{"simulator_token":"` + canary + `"}`}, {400, `{"error":"bad <b>` + canary + `","error_description":"` + canary + `"}`}, {200, `{"result":"ok"}`}, {502, canary}} {
		s, sa, _ := newTestSession(t)
		sa.status, sa.body = tc.status, tc.body
		_, err := s.provider.exchange(context.Background(), "c", "v", "http://localhost/callback")
		if err == nil || strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), "<") {
			t.Fatalf("%d: %v", tc.status, err)
		}
	}
}

func TestDifferentUserForgetsWorkspaceAndLogout(t *testing.T) {
	s, _, _ := newTestSession(t)
	exp := time.Now().Add(time.Hour)
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", exp), ExpiresAt: exp})
	_ = s.SetWorkspace("ws-1")
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", exp.Add(time.Minute)), ExpiresAt: exp})
	if w, _ := s.Workspace(); w != "ws-1" {
		t.Fatal("same user lost the workspace")
	}
	_ = s.save(context.Background(), Entry{AccessToken: jwt(7, "Bob", exp), ExpiresAt: exp})
	if w, _ := s.Workspace(); w != "" {
		t.Fatal("another user inherited the workspace")
	}
	removed, err := s.Logout()
	if err != nil || !removed {
		t.Fatal("logout", err)
	}
	if st, _ := s.Status(); st.SignedIn {
		t.Fatal("still signed in")
	}
	if removed, _ = s.Logout(); removed {
		t.Fatal("second logout removed something")
	}
}

func TestStoreKeysByOriginAndRefusesOpenFile(t *testing.T) {
	dir := t.TempDir()
	a := NewSession(Options{Origin: "https://a.example", ConfigDir: dir})
	b := NewSession(Options{Origin: "https://b.example", ConfigDir: dir})
	_ = a.SetWorkspace("ws-a")
	if w, _ := b.Workspace(); w != "" {
		t.Fatal("origins share an entry")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(a.StorePath(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Status(); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("group-readable store accepted: %v", err)
	}
}

func TestParseClaimsAndSafeCode(t *testing.T) {
	exp := time.Unix(2000000000, 0)
	c, ok := parseClaims(jwt(42, "Ada", exp))
	if !ok || c.sub != 42 || c.nick != "Ada" || !c.exp.Equal(exp) {
		t.Fatalf("%+v", c)
	}
	if _, ok = parseClaims("not-a-jwt"); ok {
		t.Fatal("accepted garbage")
	}
	if safeCode(`<script>x</script>`, 400) != "scriptxscript" || safeCode("", 401) != "http 401" {
		t.Fatal("safeCode")
	}
}

func TestSameUserKeepsWorkspaceAfterRevokedRefresh(t *testing.T) {
	s, sa, _ := newTestSession(t)
	old := time.Now().Add(-time.Minute)
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", old), RefreshToken: "revoked", ExpiresAt: old})
	_ = s.SetWorkspace("ws-1")
	if _, err := s.Authorization(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatal(err)
	}
	s.open = browser(t, sa)
	if res, err := s.Login(context.Background(), 10*time.Second); err != nil || res.Status != "signedIn" {
		t.Fatal(res, err)
	}
	if w, _ := s.Workspace(); w != "ws-1" {
		t.Fatal("the same person lost the workspace on signing in again")
	}
}

func TestTransientRefreshFailureKeepsSession(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusBadGateway} {
		s, sa, _ := newTestSession(t)
		soon := time.Now().Add(10 * time.Second)
		_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", soon), RefreshToken: "refresh-1", ExpiresAt: soon})
		sa.status, sa.body = status, `{"error":"slow_down"}`
		if _, err := s.Authorization(context.Background()); err == nil || errors.Is(err, ErrLoginRequired) {
			t.Fatalf("%d: %v", status, err)
		}
		if e, _ := s.store.Get(s.origin); e.RefreshToken != "refresh-1" {
			t.Fatalf("%d dropped the session", status)
		}
	}
	s, sa, _ := newTestSession(t)
	soon := time.Now().Add(10 * time.Second)
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", soon), RefreshToken: "refresh-1", ExpiresAt: soon})
	sa.status, sa.body = http.StatusOK, `{"error":"invalid_grant"}`
	if _, err := s.Authorization(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("an OAuth error with 200 must end the session: %v", err)
	}
}

func TestBackgroundLoginFailureIsReported(t *testing.T) {
	s, _, _ := newTestSession(t)
	var authURL string
	s.open = func(u string) error { authURL = u; return nil }
	if res, err := s.Login(context.Background(), 10*time.Millisecond); err != nil || res.Status != "pending" {
		t.Fatal(res, err)
	}
	u, _ := url.Parse(authURL)
	resp, err := http.Get(u.Query().Get("redirect_uri") + "?error=access_denied&state=" + url.QueryEscape(u.Query().Get("state")))
	if err == nil {
		_ = resp.Body.Close()
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		st, _ := s.Status()
		if st.LoginError != "" {
			if !strings.Contains(st.LoginError, "access_denied") || st.Pending || st.SignedIn {
				t.Fatalf("%+v", st)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("failure not reported")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWorkspaceIsKeptPerProcess(t *testing.T) {
	dir := t.TempDir()
	a := NewSession(Options{Origin: "https://a.example", ConfigDir: dir})
	b := NewSession(Options{Origin: "https://a.example", ConfigDir: dir}) // another client process
	_ = a.SetWorkspace("ws-a")
	if w, _ := b.Workspace(); w != "ws-a" {
		t.Fatal("a new process does not start from the stored choice")
	}
	_ = b.SetWorkspace("ws-b")
	if w, _ := a.Workspace(); w != "ws-a" {
		t.Fatal("another process switched this one's workspace")
	}
}

func TestStoreLockSerializesAndBreaksStale(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := NewStore(dir) // separate in-process mutex: only the lock file serializes
			_ = other.Update(fmt.Sprint("https://", i, ".example"), func(e *Entry) { e.Workspace = "w" })
		}()
	}
	wg.Wait()
	for i := range 8 {
		if e, _ := st.Get(fmt.Sprint("https://", i, ".example")); e.Workspace != "w" {
			t.Fatalf("lost update %d", i)
		}
	}
	lock := st.Path() + ".lock"
	_ = os.WriteFile(lock, nil, 0o600)
	past := time.Now().Add(-time.Minute)
	_ = os.Chtimes(lock, past, past)
	if err := st.Update("https://x.example", func(e *Entry) { e.Workspace = "x" }); err != nil {
		t.Fatal("stale lock not broken:", err)
	}
	if _, err := os.Stat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("lock left behind")
	}
}

func TestRenewalNeverOverwritesAnotherProcess(t *testing.T) {
	soon := time.Now().Add(10 * time.Second)
	// Another process logs out while this one renews: the logout stands.
	s, sa, dir := newTestSession(t)
	other := NewSession(Options{Origin: s.origin, ConfigDir: dir})
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", soon), RefreshToken: "refresh-1", ExpiresAt: soon})
	sa.during = func() { _, _ = other.Logout() }
	if _, err := s.Authorization(context.Background()); !errors.Is(err, ErrLoginRequired) || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("renewal after a logout: %v", err)
	}
	if e, _ := s.store.Get(s.origin); e.AccessToken != "" {
		t.Fatal("renewal resurrected a logout")
	}
	// Another person signs in meanwhile: their sign-in stays, with their own id.
	s, sa, dir = newTestSession(t)
	other = NewSession(Options{Origin: s.origin, ConfigDir: dir})
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", soon), RefreshToken: "refresh-1", ExpiresAt: soon})
	_ = s.SetWorkspace("ws-ada")
	bob := jwt(77, "Bob", time.Now().Add(time.Hour))
	sa.during = func() {
		_ = other.save(context.Background(), Entry{AccessToken: bob, ExpiresAt: time.Now().Add(time.Hour)})
	}
	h, err := s.Authorization(context.Background())
	if err != nil || h != "Simulator "+bob {
		t.Fatalf("renewal raced a sign-in: %v", err)
	}
	if e, _ := s.store.Get(s.origin); e.AccessToken != bob || e.Subject != 77 {
		t.Fatal("another person's sign-in was overwritten")
	}
	if w, _ := s.Workspace(); w != "" {
		t.Fatal("the previous person's workspace followed the new sign-in")
	}
}

func TestProxyErrorPageIsNotARefusal(t *testing.T) {
	s, sa, _ := newTestSession(t)
	soon := time.Now().Add(10 * time.Second)
	_ = s.save(context.Background(), Entry{AccessToken: jwt(42, "Ada", soon), RefreshToken: "refresh-1", ExpiresAt: soon})
	sa.status, sa.body = http.StatusForbidden, "<html>blocked</html>"
	if _, err := s.Authorization(context.Background()); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("WAF page ended the session: %v", err)
	}
	if e, _ := s.store.Get(s.origin); e.RefreshToken != "refresh-1" {
		t.Fatal("session dropped")
	}
}

func TestFloatExpClaim(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	c, ok := parseClaims(enc([]byte("{}")) + "." + enc([]byte(`{"sub":42,"exp":2000000000.5}`)) + ".x")
	if !ok || c.sub != 42 || c.exp.Unix() != 2000000000 {
		t.Fatalf("%+v %v", c, ok)
	}
}
