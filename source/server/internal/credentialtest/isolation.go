// Package credentialtest provides scoped isolation for credential unit tests.
// Only use Enable in tests that inject in-memory stores and fake providers;
// it does NOT make OpenKeychain safe. Never use it around production startup.
package credentialtest

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Enable exercises enabled admission without inheriting user state or allowing
// default HTTP clients to contact providers. Call before constructing clients.
// t.Setenv deliberately prohibits parallel use and restores the outer policy.
// Explicit custom transports must themselves be fake (never a live transport).
func Enable(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, t.TempDir())
	}
	for _, key := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE"} {
		t.Setenv(key, "")
	}
	// AWS default config can read explicit files outside HOME, execute a
	// credential_process, or consult container/web-identity providers. Clear
	// the entire AWS namespace, including endpoint and custom CA overrides.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "AWS_") {
			t.Setenv(key, "")
		}
	}
	for _, key := range []string{"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE"} {
		path := filepath.Join(t.TempDir(), "empty")
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key, path)
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	previous := http.DefaultTransport
	// Do not inherit proxy settings, custom dialing, or user client certificates.
	transport := &http.Transport{DialContext: (&net.Dialer{}).DialContext}
	http.DefaultTransport = loopbackOnly{transport}
	t.Cleanup(func() { http.DefaultTransport = previous; transport.CloseIdleConnections() })
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
}

// EnableHermetic layers deny-all default HTTP isolation over Enable, including
// loopback, and clears all inherited ANTHROPIC_* SDK configuration. Use only with
// explicitly injected in-memory transports and stores. Like Enable, it does not
// make native credential constructors safe.
func EnableHermetic(t *testing.T) {
	t.Helper()
	Enable(t)
	// Clear alternate auth, base URL and any other inherited SDK overrides,
	// not just the API key. t.Setenv restores each value during cleanup.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "OPENAI_") || strings.HasPrefix(key, "AZURE_OPENAI_") || strings.Contains(key, "INTEGRATION") {
			t.Setenv(key, "")
		}
	}
	previous := http.DefaultTransport
	http.DefaultTransport = &ForbiddenTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
}

type loopbackOnly struct{ next http.RoundTripper }

func (r loopbackOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	// Resolve localhost ourselves; never consult DNS or inherited resolver state.
	if req.URL.Hostname() == "localhost" {
		req = req.Clone(req.Context())
		u := *req.URL
		u.Host = net.JoinHostPort("127.0.0.1", req.URL.Port())
		req.URL = &u
	}
	ip := net.ParseIP(req.URL.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("credential test blocked external HTTP request to %s", req.URL.Host)
	}
	return r.next.RoundTrip(req)
}

// ForbiddenTransport counts attempted wire access without DNS, sockets or TLS.
// It can also guard explicit constructor transports that bypass DefaultTransport.
type ForbiddenTransport struct{ Calls atomic.Int64 }

func (r *ForbiddenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.Calls.Add(1)
	return nil, fmt.Errorf("credential test forbidden wire: %s", req.URL.Host)
}
