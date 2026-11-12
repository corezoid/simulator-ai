package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// renewBefore renews (or reports expired) a token this long before its exp, so a request started
// now does not reach the server with a token that lapses in flight.
const renewBefore = time.Minute

// ErrLoginRequired marks a missing or expired sign-in; its message tells the agent what to do.
var ErrLoginRequired = errors.New("login required")

type loginError struct{ message string }

func (e loginError) Error() string        { return e.message }
func (e loginError) Is(target error) bool { return target == ErrLoginRequired }

// Options configures a Session. Empty AccountURL and ClientID use the production defaults.
type Options struct {
	Origin     string // API origin the credential belongs to (the store key)
	ConfigDir  string // directory of credentials.json
	AccountURL string
	ClientID   string
}

// Session is one API origin's human login. It is safe for concurrent use.
type Session struct {
	origin   string
	store    *Store
	provider provider
	open     func(string) error
	now      func() time.Time

	renewMu sync.Mutex // serializes renewal and logout, so a renewal never resurrects a logout

	mu      sync.Mutex // guards pending and workspace
	pending *flow
	// workspace is this process's choice for subject (the signed-in SuperAdmin id), read from the
	// store once and then kept, so another client process choosing a workspace never switches this
	// one in the middle of its work.
	workspace string
	subject   int64
}

func NewSession(o Options) *Session {
	account := strings.TrimRight(o.AccountURL, "/")
	if account == "" {
		account = DefaultAccountURL
	}
	client := o.ClientID
	if client == "" {
		client = DefaultClientID
	}
	h := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Session{origin: o.Origin, store: NewStore(o.ConfigDir), provider: provider{account, client, h}, open: openBrowser, now: time.Now}
}

// Origin is the API origin this session signs in to.
func (s *Session) Origin() string { return s.origin }

// AccountURL is the SuperAdmin the browser sign-in goes to.
func (s *Session) AccountURL() string { return s.provider.accountURL }

// StorePath is where the credential is kept (no secret).
func (s *Session) StorePath() string { return s.store.Path() }

// Authorization returns the "Simulator <jwt>" header for the next request, renewing the token with
// its refresh token when it is about to expire.
func (s *Session) Authorization(ctx context.Context) (string, error) {
	s.renewMu.Lock()
	defer s.renewMu.Unlock()
	e, err := s.store.Get(s.origin)
	if err != nil {
		return "", err
	}
	if e.AccessToken == "" {
		return "", loginError{"not signed in to " + s.origin + ": call the login tool (it opens the browser), then pick a workspace"}
	}
	if s.fresh(e) {
		return "Simulator " + e.AccessToken, nil
	}
	if e.RefreshToken == "" {
		return "", s.expired()
	}
	// Detached from the tool call: a renewal SuperAdmin answered (and may have rotated) must be saved
	// even when the caller gives up.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	renewed, err := s.provider.refresh(rctx, e.RefreshToken)
	// Every write below is a compare-and-set on the pair this renewal started from: another process
	// may meanwhile have renewed, logged out or signed in as someone else, and that wins.
	same := func(x *Entry) bool { return x.AccessToken == e.AccessToken && x.RefreshToken == e.RefreshToken }
	if err != nil {
		var rejected errRejected
		if !errors.As(err, &rejected) {
			return "", errors.New("cannot renew the sign-in: " + err.Error())
		}
		// The person and their workspace choice stay; only the dead grant goes.
		_ = s.store.Update(s.origin, func(x *Entry) {
			if same(x) {
				x.AccessToken, x.RefreshToken, x.ExpiresAt = "", "", time.Time{}
			}
		})
		return s.current(e)
	}
	if renewed.RefreshToken == "" {
		renewed.RefreshToken = e.RefreshToken
	}
	applied := false
	err = s.store.Update(s.origin, func(x *Entry) {
		// A concurrent renewal of the same pair that SuperAdmin refused (rotation) cleared the grant
		// but kept the person: this successful renewal restores it. A logout empties the whole entry
		// (no Subject), so it still stands.
		refusedMeanwhile := x.AccessToken == "" && x.RefreshToken == "" && x.Subject != 0 && x.Subject == e.Subject
		if applied = same(x) || refusedMeanwhile; applied {
			x.AccessToken, x.RefreshToken, x.ExpiresAt = renewed.AccessToken, renewed.RefreshToken, renewed.ExpiresAt
		}
	})
	if err != nil {
		return "Simulator " + renewed.AccessToken, nil // Valid now; the next request renews again.
	}
	if !applied {
		return s.current(e)
	}
	return "Simulator " + renewed.AccessToken, nil
}

// current is what another process left in the store after this one's renewal lost the race: its
// fresh token, or the sign-in to repeat.
func (s *Session) current(was Entry) (string, error) {
	again, err := s.store.Get(s.origin)
	if err != nil {
		return "", err
	}
	if again.AccessToken != "" && again.AccessToken != was.AccessToken && s.fresh(again) {
		return "Simulator " + again.AccessToken, nil
	}
	if again.AccessToken == "" && again.Subject == 0 {
		return "", loginError{"not signed in to " + s.origin + ": call the login tool (it opens the browser), then pick a workspace"}
	}
	return "", s.expired()
}

func (s *Session) fresh(e Entry) bool { return e.ExpiresAt.After(s.now().Add(renewBefore)) }

func (s *Session) expired() error {
	return loginError{"the sign-in to " + s.origin + " expired: call the login tool again"}
}

// Status describes the sign-in without any secret.
type Status struct {
	SignedIn  bool       `json:"signedIn"`
	Expired   bool       `json:"expired,omitempty"`
	Renewable bool       `json:"renewable,omitempty"`
	SAID      int64      `json:"saId,omitempty"`
	Nick      string     `json:"nick,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Pending   bool       `json:"loginPending,omitempty"`
	// LoginError is why the last browser sign-in that finished in the background failed.
	LoginError string `json:"loginError,omitempty"`
}

func (s *Session) Status() (Status, error) {
	e, err := s.store.Get(s.origin)
	if err != nil {
		return Status{}, err
	}
	st := Status{SignedIn: e.AccessToken != "", Renewable: e.RefreshToken != ""}
	s.mu.Lock()
	if f := s.pending; f != nil {
		st.Pending = !f.finished()
		if !st.Pending && f.err != nil && !st.SignedIn {
			st.LoginError = f.err.Error()
		}
	}
	s.mu.Unlock()
	if !st.SignedIn {
		return st, nil
	}
	st.Expired = !s.fresh(e)
	exp := e.ExpiresAt
	st.ExpiresAt = &exp
	if c, ok := parseClaims(e.AccessToken); ok {
		st.SAID, st.Nick = c.sub, c.nick
	}
	return st, nil
}

// LoginResult reports a browser sign-in. AuthorizationURL is set when the person has to open the
// page themselves (the browser did not start, or the sign-in is still waiting).
type LoginResult struct {
	Status           string `json:"status"` // signedIn or pending
	BrowserOpened    bool   `json:"browserOpened"`
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	User             Status `json:"user"`
}

// Login starts (or rejoins) the browser sign-in and waits up to wait for it to finish. A sign-in
// still open after wait keeps running in the background and is saved when the person completes it.
func (s *Session) Login(ctx context.Context, wait time.Duration) (LoginResult, error) {
	s.mu.Lock()
	f, started := s.pending, false
	if f == nil || f.finished() {
		var err error
		if f, err = startFlow(s.provider, s.save); err != nil {
			s.mu.Unlock()
			return LoginResult{}, err
		}
		s.pending, started = f, true
	}
	s.mu.Unlock()
	res := LoginResult{Status: "pending"}
	if started {
		res.BrowserOpened = s.open(f.authURL) == nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-f.done:
		if f.err != nil {
			return LoginResult{}, errors.New("login failed: " + f.err.Error())
		}
		st, err := s.Status()
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{Status: "signedIn", BrowserOpened: res.BrowserOpened, User: st}, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	if !res.BrowserOpened {
		res.AuthorizationURL = f.authURL // The person opens it themselves.
	}
	res.User, _ = s.Status()
	return res, nil
}

// save stores a new sign-in. A different person than before loses the previous workspace choice;
// the same person keeps it, also after an expired or revoked sign-in.
func (s *Session) save(_ context.Context, e Entry) error {
	after, _ := parseClaims(e.AccessToken)
	changed := false
	err := s.store.Update(s.origin, func(x *Entry) {
		if x.Subject != after.sub {
			x.Workspace, changed = "", true
		}
		x.AccessToken, x.RefreshToken, x.ExpiresAt, x.Subject = e.AccessToken, e.RefreshToken, e.ExpiresAt, after.sub
	})
	if err == nil && changed {
		s.mu.Lock()
		s.workspace = ""
		s.mu.Unlock()
	}
	return err
}

// Logout cancels a pending sign-in and forgets this origin's token and workspace choice locally.
// It reports whether a credential was stored.
func (s *Session) Logout() (bool, error) {
	s.mu.Lock()
	f := s.pending
	s.pending, s.workspace = nil, ""
	s.mu.Unlock()
	if f != nil {
		f.cancel()
		<-f.done // A code already received is exchanged and saved first, then removed below.
	}
	s.renewMu.Lock()
	defer s.renewMu.Unlock()
	had := false
	err := s.store.Update(s.origin, func(x *Entry) { had = x.AccessToken != ""; *x = Entry{} })
	return had, err
}

// Workspace is the workspace the person chose for this origin ("" when none). Once known it is kept
// for the life of the process while the same person is signed in: another client process choosing
// a workspace never switches this one mid-work, but a sign-in as someone else does.
func (s *Session) Workspace() (string, error) {
	e, err := s.store.Get(s.origin)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if s.workspace != "" {
			return s.workspace, nil // A passing read failure must not change the workspace mid-operation.
		}
		return "", err
	}
	if s.workspace == "" || s.subject != e.Subject {
		s.workspace, s.subject = e.Workspace, e.Subject
	}
	return s.workspace, nil
}

// SetWorkspace remembers the person's workspace for this origin.
func (s *Session) SetWorkspace(id string) error {
	subject := int64(0)
	if err := s.store.Update(s.origin, func(x *Entry) { x.Workspace, subject = id, x.Subject }); err != nil {
		return err
	}
	s.mu.Lock()
	s.workspace, s.subject = id, subject
	s.mu.Unlock()
	return nil
}

// User is the SuperAdmin id and nick the stored token names (unverified, for display and for
// recognising the caller's own member row); ok false when not signed in.
func (s *Session) User() (int64, string, bool) {
	e, err := s.store.Get(s.origin)
	if err != nil || e.AccessToken == "" {
		return 0, "", false
	}
	return UserOf(e.AccessToken)
}
