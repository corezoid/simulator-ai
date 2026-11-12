package auth

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// flowTimeout bounds how long a browser sign-in may take.
const flowTimeout = 5 * time.Minute

// flow is one browser sign-in: a loopback callback server waiting for SuperAdmin's redirect.
// It finishes on its own (saved, refused or timed out) even after the login tool call returned.
type flow struct {
	authURL string
	done    chan struct{}
	err     error // set before done closes
	cancel  context.CancelFunc
}

func (f *flow) finished() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

type callbackResult struct {
	code string
	err  error
}

// startFlow listens on a random loopback port and returns at once; the exchange and save happen in
// the background through finish.
func startFlow(p provider, finish func(context.Context, Entry) error) (*flow, error) {
	verifier, err := randomToken()
	if err != nil {
		return nil, err
	}
	state, err := randomToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, errors.New("cannot start the local sign-in callback listener")
	}
	redirect := fmt.Sprintf("http://localhost:%d/callback", listener.Addr().(*net.TCPAddr).Port)
	results := make(chan callbackResult, 1)
	deliver := func(r callbackResult) {
		select {
		case results <- r:
		default: // Only the first matching callback counts.
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// A request without this login's state is not ours (or a forged code): refuse it and keep
		// waiting, so a stray request cannot abort the person's sign-in.
		if q.Get("state") != state {
			writePage(w, http.StatusBadRequest, false, "This sign-in link does not belong to the current login. Start login again from your agent.")
			return
		}
		if code := q.Get("error"); code != "" {
			writePage(w, http.StatusOK, false, "Sign-in was not completed ("+html.EscapeString(safeCode(code, 0))+"). You may close this tab.")
			deliver(callbackResult{err: errRejected{safeCode(code, 0)}})
			return
		}
		code := q.Get("code")
		if code == "" {
			writePage(w, http.StatusBadRequest, false, "No authorization code was received. Start login again from your agent.")
			deliver(callbackResult{err: errors.New("no authorization code in the sign-in callback")})
			return
		}
		writePage(w, http.StatusOK, true, "You are signed in to Simulator.Company. You may close this tab and return to your agent.")
		deliver(callbackResult{code: code})
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(listener) }() // Serve returns ErrServerClosed on shutdown.

	ctx, cancel := context.WithTimeout(context.Background(), flowTimeout)
	f := &flow{authURL: p.authorizeURL(redirect, challengeOf(verifier), state), done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(f.done)
		defer cancel()
		var r callbackResult
		select {
		case r = <-results:
		case <-ctx.Done():
			r.err = errors.New("the browser sign-in was not completed within 5 minutes")
			if errors.Is(ctx.Err(), context.Canceled) {
				r.err = errors.New("the browser sign-in was cancelled")
			}
		}
		shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(shutdown) // The callback page has been written already.
		stop()
		if r.err != nil {
			f.err = r.err
			return
		}
		exchange, stopExchange := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopExchange()
		e, err := p.exchange(exchange, r.code, verifier, redirect)
		if err == nil {
			err = finish(exchange, e)
		}
		f.err = err
	}()
	return f, nil
}

func writePage(w http.ResponseWriter, status int, ok bool, message string) {
	title, color := "Sign-in failed", "#c62828"
	if ok {
		title, color = "Signed in", "#2e7d32"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Simulator AI — %s</title>
<style>body{font-family:system-ui,sans-serif;background:#f4f6fb;color:#1a1a2e;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
main{background:#fff;border-radius:12px;box-shadow:0 6px 30px rgba(0,0,0,.08);padding:40px 48px;max-width:440px;text-align:center;border-bottom:4px solid %s}
h1{font-size:22px;margin:0 0 12px}p{color:#555;line-height:1.5;margin:0}</style></head>
<body><main><h1>%s</h1><p>%s</p></main></body></html>`, title, color, title, message)
}

// openBrowser asks the desktop to open the sign-in page. The child never inherits the MCP stdio.
func openBrowser(u string) error {
	name, args := "xdg-open", []string{u}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", u}
	}
	// u is the authorize URL built here from the validated account origin and query-escaped
	// parameters; it is a single argument, never parsed by a shell.
	cmd := exec.Command(name, args...) //nolint:gosec // G204: fixed launcher, URL built locally (see above).
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // Reap the launcher; its exit status says nothing about the login.
	return nil
}
