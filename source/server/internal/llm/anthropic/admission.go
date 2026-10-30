package anthropic

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/modelpolicy"
	"cercano/source/server/internal/secrets"
)

func messagesHTTPClient(cfg Config) *http.Client {
	h := &http.Client{Transport: cfg.BaseTransport}
	auth := authenticatorForRoute(cfg)
	if cfg.Credentials != nil {
		// Same bounded HTTP/1 fresh-connection constraints as coordinated Responses.
		// SDK retries are disabled separately. Never alter legacy transports.
		if cfg.BaseTransport == nil {
			h.Transport = messagesAttemptTransport()
		} else {
			h.Transport = messagesNonReplay{next: cfg.BaseTransport}
		}
		h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		auth = staticAttemptAuth{cfg: cfg}
	}
	h = modelpolicy.Client(h, "anthropic", "external", modelpolicy.Anthropic)
	h.Transport = &authRoundTripper{base: h.Transport, ua: cfg.UserAgent, auth: auth}
	return h
}

type SubscriptionCredentials interface {
	CredentialProfile() string
	SubscriptionSnapshot(context.Context) (secrets.CredentialSnapshot, error)
}

type staticAttemptAuth struct{ cfg Config }

func (a staticAttemptAuth) decorate(r *http.Request) error {
	c := a.cfg
	if c.CredentialProfile == "" {
		return errors.New("coordinated messages route unsupported")
	}
	var s secrets.CredentialSnapshot
	var err error
	var access string
	switch {
	case c.Route == RouteSubscription:
		source, ok := c.TokenSource.(SubscriptionCredentials)
		if !ok || source.CredentialProfile() != c.CredentialProfile {
			return errors.New("coordinated subscription source required")
		}
		s, err = source.SubscriptionSnapshot(r.Context())
		if err == nil {
			var ts anthropicauth.TokenSet
			ts, err = anthropicauth.DecodeTokenSet(s.Payload)
			access = ts.Access
			if err == nil && access == "" {
				err = errors.New("empty subscription access token")
			}
		}
	case (c.Route == "" || c.Route == routeDirect) && c.TokenSource == nil:
		s, err = c.Credentials.CredentialSnapshot(r.Context(), c.CredentialProfile)
		if err == nil && s.Payload == "" {
			err = errors.New("empty coordinated messages key")
		}
	default:
		return errors.New("coordinated messages route unsupported")
	}
	if err != nil {
		return err
	}
	if err = c.Credentials.AdmitAttempt(r.Context(), c.CredentialProfile, s.Revision); err != nil {
		return err
	}
	if c.Route == RouteSubscription {
		r.Header.Del("X-Api-Key")
		r.Header.Set("Authorization", "Bearer "+access)
		r.Header.Set("anthropic-beta", oauthBetaHeader)
		return nil
	}
	// SDK/environment auth must not survive alongside the admitted static key.
	r.Header.Del("Authorization")
	r.Header.Set("X-Api-Key", s.Payload)
	return nil
}

// Innermost, AFTER model policy has inspected the SDK body via GetBody.
// Removing GetBody alone is insufficient: keep-alives and HTTP/2 are disabled,
// and only nonempty POSTs without idempotency markers are permitted.
type messagesNonReplay struct{ next http.RoundTripper }

func messagesAttemptTransport() messagesNonReplay {
	return messagesNonReplay{next: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}}
}

func (t messagesNonReplay) RoundTrip(r *http.Request) (*http.Response, error) {
	_, idempotent := r.Header["Idempotency-Key"]
	_, xIdempotent := r.Header["X-Idempotency-Key"]
	if r.Method != http.MethodPost || r.Body == nil || r.ContentLength <= 0 || idempotent || xIdempotent {
		return nil, errors.New("coordinated messages replayable request refused")
	}
	wire := r.Clone(r.Context())
	wire.GetBody = nil
	return t.next.RoundTrip(wire)
}
