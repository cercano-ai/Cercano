package anthropicauth

import (
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Pending is a started authorization awaiting user approval. It carries two
// URLs for the same grant (one PKCE pair, one state): AuthorizeURL redirects
// back to the loopback listener, ManualAuthorizeURL lands on a hosted page
// that shows the code for the user to copy. The caller opens AuthorizeURL in a
// browser and shows ManualAuthorizeURL for the headless/SSH case, then calls
// Wait, which completes from whichever arrives first: the loopback redirect or
// a pasted code. Redeem completes from a pasted code alone. Both close the
// loopback listener.
type Pending struct {
	// AuthorizeURL is the URL to open in a browser on this machine.
	AuthorizeURL string
	// ManualAuthorizeURL is the URL to show for copying to a browser anywhere;
	// approving it displays a code to paste back.
	ManualAuthorizeURL string

	state             string
	verifier          string
	redirectURI       string
	manualRedirectURI string
	listener          net.Listener
	flow              Flow
}

// Start binds a loopback listener on an ephemeral port, generates the PKCE
// pair and CSRF state, and builds both authorize URLs. The listener is held
// open until Wait/Redeem/Close.
func (f Flow) Start(ctx context.Context) (*Pending, error) {
	if err := f.Options.CheckMutation(); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("anthropic oauth: bind loopback: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	// Match Claude Code's redirect exactly: host "localhost", path "/callback".
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	pk, err := newPKCE()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	state, err := randomToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	manual := f.manualRedirectURL()
	return &Pending{
		AuthorizeURL:       f.authorizeURL(redirectURI, pk.challenge, state),
		ManualAuthorizeURL: f.authorizeURL(manual, pk.challenge, state),
		state:              state,
		verifier:           pk.verifier,
		redirectURI:        redirectURI,
		manualRedirectURI:  manual,
		listener:           ln,
		flow:               f,
	}, nil
}

// PastedCode is a code the user pasted while Wait is running. Wait reports the
// outcome of redeeming it on Result (buffered by the sender): nil means the
// sign-in completed and Wait returns the token set; an error means the paste
// was rejected and Wait keeps waiting, so the user can paste again.
type PastedCode struct {
	Code   string
	Result chan<- error
}

// ErrInvalidPastedCode reports a paste that is not a code for this sign-in
// (empty, malformed, or carrying another attempt's state). Nothing was sent to
// the token endpoint.
var ErrInvalidPastedCode = errors.New("anthropic oauth: pasted value is not a code for this sign-in")

type callbackResult struct {
	code string
	err  error
}

// Wait serves the loopback listener until the browser hits /callback, renders
// a success (or failure) page to that browser, then exchanges the returned
// code for a token set. Codes arriving on pasted (nil disables pasting) are
// redeemed as they come; a rejected paste is reported on its Result and the
// wait continues. It returns ctx.Err() if the context is canceled first. The
// listener is always closed on return.
//
// Teardown is graceful: the handler flushes its response with Connection:
// close, and Wait shuts the server down with Shutdown (not Close) so the
// browser fully receives the page instead of a reset connection — the reset
// is what renders as "localhost refused to connect".
func (p *Pending) Wait(ctx context.Context, pasted <-chan PastedCode) (*TokenSet, error) {
	if err := secrets.CheckAllowed(); err != nil {
		return nil, err
	}
	if err := p.flow.Options.CheckMutation(); err != nil {
		return nil, err
	}
	done := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var cb callbackResult
		switch {
		case q.Get("error") != "":
			e := q.Get("error")
			writeBrowserPage(w, false, "Sign-in failed", "Anthropic returned an error: "+e+". You can close this window and try again in Cercano.")
			cb = callbackResult{err: fmt.Errorf("anthropic oauth: authorize error: %s", e)}
		case q.Get("code") == "":
			writeBrowserPage(w, false, "Sign-in failed", "No authorization code was returned. You can close this window and try again in Cercano.")
			cb = callbackResult{err: fmt.Errorf("anthropic oauth: callback missing code")}
		case q.Get("state") != p.state:
			writeBrowserPage(w, false, "Sign-in failed", "The sign-in could not be verified (state mismatch). You can close this window and try again in Cercano.")
			cb = callbackResult{err: fmt.Errorf("anthropic oauth: state mismatch")}
		default:
			writeBrowserPage(w, true, "Successfully authenticated", "You're signed in to Claude. You may close this window or tab and return to Cercano.")
			cb = callbackResult{code: q.Get("code")}
		}
		// Only the first callback counts; a repeat must not block the handler.
		select {
		case done <- cb:
		default:
		}
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(p.listener) }()
	// Drain the response gracefully before returning; the handler already set
	// Connection: close, so the browser drops its socket and Shutdown returns
	// promptly rather than blocking on a keep-alive connection.
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	for {
		select {
		case <-ctx.Done():
			_ = srv.Close()
			return nil, ctx.Err()
		case cb := <-done:
			if cb.err != nil {
				return nil, cb.err
			}
			return p.flow.exchange(ctx, cb.code, p.state, p.verifier, p.redirectURI)
		case pc := <-pasted:
			ts, err := p.redeem(ctx, pc.Code)
			if pc.Result != nil {
				pc.Result <- err
			}
			if err == nil {
				return ts, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
	}
}

// Redeem completes the flow from a manually pasted code alone, without
// serving the loopback. The listener is closed on return.
func (p *Pending) Redeem(ctx context.Context, pasted string) (*TokenSet, error) {
	if err := secrets.CheckAllowed(); err != nil {
		return nil, err
	}
	if err := p.flow.Options.CheckMutation(); err != nil {
		return nil, err
	}
	defer p.listener.Close()
	return p.redeem(ctx, pasted)
}

// redeem exchanges a pasted value. Two shapes are accepted:
//
//   - "<code>#<state>", as displayed by the manual redirect page (a bare code
//     is taken to belong to this attempt). Exchanged with the manual
//     redirect_uri, which is what that code was issued against.
//   - The full loopback callback URL, for a user who opened AuthorizeURL in a
//     browser that cannot reach this machine and copied the address of the
//     page that failed to load. Exchanged with the loopback redirect_uri.
//
// Either way the state must match this attempt's.
func (p *Pending) redeem(ctx context.Context, pasted string) (*TokenSet, error) {
	v := strings.TrimSpace(pasted)
	code, state, redirectURI := v, p.state, p.manualRedirectURI
	if strings.HasPrefix(v, p.redirectURI+"?") {
		u, err := url.Parse(v)
		if err != nil {
			return nil, ErrInvalidPastedCode
		}
		code, state, redirectURI = u.Query().Get("code"), u.Query().Get("state"), p.redirectURI
	} else if i := strings.IndexByte(v, '#'); i >= 0 {
		code, state = v[:i], v[i+1:]
	}
	if code == "" || strings.ContainsAny(code, " \t\r\n") || state != p.state {
		return nil, ErrInvalidPastedCode
	}
	return p.flow.exchange(ctx, code, state, p.verifier, redirectURI)
}

// Close releases the loopback listener without completing the flow (e.g. the
// user canceled). Safe to call more than once.
func (p *Pending) Close() error {
	return p.listener.Close()
}

// writeBrowserPage renders the page shown in the user's browser after the
// redirect, then flushes it so the bytes reach the browser before the server
// is torn down. Connection: close tells the browser to drop the socket, which
// keeps graceful shutdown from racing a lingering keep-alive connection.
//
// title and msg are plain text and are HTML-escaped before interpolation: the
// failure page reflects the untrusted "error" query parameter into msg, and
// that branch runs before the state check, so an unescaped value would let any
// local process that finds the loopback port inject markup into this page.
func writeBrowserPage(w http.ResponseWriter, success bool, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Connection", "close")
	accent := "#BDF000" // lime — success
	if !success {
		accent = "#EA5555" // red — failure
	}
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><meta charset=utf-8>"+
		"<title>Cercano — %s</title></head>"+
		"<body style=\"font-family:system-ui,-apple-system,sans-serif;background:#1A1A1A;"+
		"color:#EA8212;display:flex;flex-direction:column;align-items:center;"+
		"justify-content:center;height:100vh;margin:0;text-align:center;padding:0 1.5rem\">"+
		"<div style=\"font-size:1.5rem;color:%s;font-weight:600;margin-bottom:.6rem\">%s</div>"+
		"<div style=\"color:#C8C8C8;max-width:32rem;line-height:1.5\">%s</div></body></html>",
		html.EscapeString(title), accent, html.EscapeString(title), html.EscapeString(msg))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
