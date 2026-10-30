package openai

import (
	"crypto/tls"
	"errors"
	"net/http"
	"strings"

	"cercano/source/server/internal/secrets"
	goopenai "github.com/sashabaranov/go-openai"
)

// Each SDK Do is one attempt. No credential is retained between attempts.
type chatAttemptDoer struct {
	cfg  Config
	next goopenai.HTTPDoer
}

func (d chatAttemptDoer) Do(r *http.Request) (*http.Response, error) {
	if r.Context().Value(reasoningDiagnosticKey{}) != nil {
		return nil, errors.New("coordinated reasoning diagnostic unsupported")
	}
	if d.cfg.Route != "" || d.cfg.CredentialProfile == "" {
		return nil, errors.New("coordinated chat route unsupported")
	}
	if err := secrets.CheckAllowed(); err != nil {
		return nil, err
	}
	s, err := d.cfg.Credentials.CredentialSnapshot(r.Context(), d.cfg.CredentialProfile)
	if err != nil {
		return nil, err
	}
	if s.Payload == "" {
		return nil, errors.New("empty coordinated chat key")
	}
	if err = d.cfg.Credentials.AdmitAttempt(r.Context(), d.cfg.CredentialProfile, s.Revision); err != nil {
		return nil, err
	}
	wire := r.Clone(r.Context())
	wire.Header.Set("Authorization", "Bearer "+s.Payload)
	resp, err := d.next.Do(wire)
	// The SDK treats 3xx as success. CheckRedirect prevents following it,
	// but the retained response must also fail rather than decode as success.
	if err == nil && resp != nil && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, errors.New("coordinated chat redirect refused")
	}
	return resp, err
}

type chatNonReplay struct{ next http.RoundTripper }

func chatAttemptTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}
	}
	return chatNonReplay{next: base}
}

// Policy and normalization inspect GetBody before this innermost strip.
func (t chatNonReplay) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodPost || r.Body == nil || r.ContentLength <= 0 {
		return nil, errors.New("coordinated chat replayable request refused")
	}
	for key := range r.Header {
		if strings.EqualFold(key, "Idempotency-Key") || strings.EqualFold(key, "X-Idempotency-Key") {
			return nil, errors.New("coordinated chat replayable request refused")
		}
	}
	wire := r.Clone(r.Context())
	wire.GetBody = nil
	return t.next.RoundTrip(wire)
}
