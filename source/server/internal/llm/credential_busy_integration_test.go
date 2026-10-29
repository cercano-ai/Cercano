package llm_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/hostsvc/credentials"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/llm/anthropic"
	"cercano/source/server/internal/llm/openai"
	"cercano/source/server/internal/llm/responses"
	"cercano/source/server/internal/secrets"
)

// All records and issuer work are synthetic; only exclusion files touch disk.
type busyRecords struct{ raw string }

func (b *busyRecords) Read(string) (string, error)        { return b.raw, nil }
func (b *busyRecords) Replace(_ string, raw string) error { b.raw = raw; return nil }
func (b *busyRecords) Keys() ([]string, error)            { return []string{"profile"}, nil }
func busyOwner(t *testing.T) (*credentials.Service, *credentials.CoordinatedStore) {
	t.Helper()
	credentialtest.EnableHermetic(t) // denies default HTTP, including loopback, before construction
	b := &busyRecords{raw: `{"version":1,"revision":"` + strings.Repeat("ab", 32) + `","state":"active","payload":"synthetic-secret"}`}
	store, err := credentials.NewCoordinatedStore(b, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return credentials.New(store), store
}

type busyWire func(*http.Request) (*http.Response, error)

func (f busyWire) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type publicClient interface {
	Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error)
	StreamChat(context.Context, llm.ChatRequest) (llm.StreamReader, error)
}

// Every SDK is constructed with an explicit non-dialing wire and synthetic URL.
func busyClient(provider string, source secrets.RequestCredentials, wire http.RoundTripper) publicClient {
	switch provider {
	case "responses":
		return responses.NewClient(responses.Config{BaseURL: "https://responses.invalid", Credentials: source, CredentialProfile: "profile", BaseTransport: wire})
	case "anthropic":
		return anthropic.NewClient(anthropic.Config{BaseURL: "https://anthropic.invalid", Credentials: source, CredentialProfile: "profile", BaseTransport: wire})
	default:
		return openai.NewClient(openai.Config{BaseURL: "https://openai.invalid", Credentials: source, CredentialProfile: "profile", BaseTransport: wire})
	}
}

// Acquire real coordinator exclusion exactly at snapshot or admission, without
// scheduling races, sleeps, issuer HTTP, or backend failure simulation.
type busyAttempt struct {
	secrets.RequestCredentials
	store           *credentials.CoordinatedStore
	revision, phase string
}

func (b busyAttempt) contend(ctx context.Context, f func() error) error {
	_, err := b.store.RefreshSnapshot(ctx, "profile", b.revision, func(context.Context, string) (string, error) { return "", f() })
	return err
}
func (b busyAttempt) CredentialSnapshot(ctx context.Context, p string) (s secrets.CredentialSnapshot, err error) {
	if b.phase != "snapshot" {
		return b.RequestCredentials.CredentialSnapshot(ctx, p)
	}
	err = b.contend(ctx, func() error { s, err = b.RequestCredentials.CredentialSnapshot(ctx, p); return err })
	return
}
func (b busyAttempt) AdmitAttempt(ctx context.Context, p, r string) error {
	if b.phase != "admission" {
		return b.RequestCredentials.AdmitAttempt(ctx, p, r)
	}
	return b.contend(ctx, func() error { return b.RequestCredentials.AdmitAttempt(ctx, p, r) })
}
func assertBusy(t *testing.T, err error) {
	t.Helper()
	if llm.ClassOf(err) != llm.ErrBusy || !errors.Is(err, credentials.ErrBusy) {
		t.Errorf("want busy sentinel, got %s: %v", llm.ClassOf(err), err)
	}
	// Keep the existing generic Busy resilience policy; no special failover rule.
	if !llm.Retryable(llm.ClassOf(err)) || !llm.Failoverable(llm.ClassOf(err), err) {
		t.Error("generic Busy resilience policy changed")
	}
	if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
		t.Error("secret in diagnostic")
	}
}
func TestCredentialBusyPublicProviders(t *testing.T) {
	for _, provider := range []string{"responses", "anthropic", "openai"} {
		for _, phase := range []string{"snapshot", "admission"} {
			for _, stream := range []bool{false, true} {
				name := provider + "/" + phase + "/Chat"
				if stream {
					name = provider + "/" + phase + "/Stream"
				}
				t.Run(name, func(t *testing.T) {
					owner, store := busyOwner(t)
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					snap, err := owner.CredentialSnapshot(ctx, "profile")
					if err != nil {
						t.Fatal(err)
					}
					source := busyAttempt{RequestCredentials: owner, store: store, revision: snap.Revision, phase: phase}
					calls := 0
					wire := busyWire(func(*http.Request) (*http.Response, error) {
						calls++
						return nil, errors.New("unexpected synthetic wire")
					})
					c := busyClient(provider, source, wire)
					if stream {
						var r llm.StreamReader
						r, err = c.StreamChat(ctx, llm.ChatRequest{Model: "fixture"})
						if r != nil {
							defer r.Close()
							if err == nil {
								_, _, err = r.Next()
							}
						}
					} else {
						_, err = c.Chat(ctx, llm.ChatRequest{Model: "fixture"})
					}
					assertBusy(t, err)
					if calls != 0 {
						t.Fatalf("wire calls=%d", calls)
					}
					after, e := owner.CredentialSnapshot(ctx, "profile")
					if e != nil || after != snap {
						t.Fatal("contention mutated record")
					}
				})
			}
		}
	}
}

func TestCredentialBusyLoginAndRefresh(t *testing.T) {
	owner, store := busyOwner(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	login, err := owner.BeginLogin(ctx, "profile", "openai-responses")
	if err != nil {
		t.Fatal(err)
	}
	defer login.Close()
	snap, err := owner.CredentialSnapshot(ctx, "profile")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RefreshSnapshot(ctx, "profile", snap.Revision, func(context.Context, string) (string, error) {
		_, e := owner.BeginLogin(ctx, "profile", "openai-responses")
		assertBusy(t, e)
		assertBusy(t, login.Commit("synthetic-secret-new"))
		_, e = owner.CredentialSnapshot(ctx, "profile")
		assertBusy(t, e)
		assertBusy(t, owner.AdmitAttempt(ctx, "profile", snap.Revision))
		calls := 0
		_, e = owner.RefreshSnapshot(ctx, "profile", snap.Revision, func(context.Context, string) (string, error) { calls++; return "", nil })
		assertBusy(t, e)
		if calls != 0 {
			t.Error("busy refresh invoked issuer")
		}
		return "replacement-synthetic", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Busy must not cancel the login, but a later revision must not be rebased.
	if e := login.Commit("synthetic-secret-new"); !errors.Is(e, credentials.ErrStale) || llm.ClassOf(e) != llm.ErrCredential {
		t.Fatalf("stale commit: %v", e)
	}
	if err := store.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if e := login.Commit("synthetic-secret-new"); !errors.Is(e, credentials.ErrStale) {
		t.Fatalf("logout bypass: %v", e)
	}
}

// Reuse the very same public client after replacement and logout. Synthetic 401
// responses intentionally avoid vendor response decoders; the wire assertion is
// about a fresh, admitted credential on each independent attempt, not issuance.
func TestCredentialCachedClientReplacement(t *testing.T) {
	for _, provider := range []string{"responses", "anthropic", "openai"} {
		for _, stream := range []bool{false, true} {
			name := provider + "/Chat"
			if stream {
				name = provider + "/Stream"
			}
			t.Run(name, func(t *testing.T) {
				owner, store := busyOwner(t)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				calls := 0
				expected := "synthetic-secret"
				wire := busyWire(func(r *http.Request) (*http.Response, error) {
					calls++
					got := r.Header.Get("Authorization")
					want := "Bearer " + expected
					if provider == "anthropic" {
						got, want = r.Header.Get("X-Api-Key"), expected
					}
					if got != want {
						t.Error("cached client did not use fresh snapshot")
					}
					if r.GetBody != nil {
						t.Error("replayable admitted request")
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"authentication_error","message":"synthetic rejection"}}`)), Request: r}, nil
				})
				c := busyClient(provider, owner, wire)
				attempt := func() error {
					if !stream {
						_, err := c.Chat(ctx, llm.ChatRequest{Model: "fixture"})
						return err
					}
					r, err := c.StreamChat(ctx, llm.ChatRequest{Model: "fixture"})
					if r != nil {
						defer r.Close()
						if err == nil {
							_, _, err = r.Next()
						}
					}
					return err
				}
				if err := attempt(); llm.ClassOf(err) != llm.ErrAuth {
					t.Fatalf("first attempt: %v", err)
				}
				if calls != 1 {
					t.Fatalf("first wire calls=%d", calls)
				}
				if err := store.Set("profile", "replacement-synthetic"); err != nil {
					t.Fatal(err)
				}
				expected = "replacement-synthetic"
				if err := attempt(); llm.ClassOf(err) != llm.ErrAuth {
					t.Fatalf("replacement attempt: %v", err)
				}
				if calls != 2 {
					t.Fatalf("replacement wire calls=%d", calls)
				}
				if err := store.Delete("profile"); err != nil {
					t.Fatal(err)
				}
				if err := attempt(); err == nil {
					t.Fatal("logout accepted")
				}
				if calls != 2 {
					t.Fatalf("logout dispatched: wire calls=%d", calls)
				}
			})
		}
	}
}

// A constructor-injected wire must still sit behind replay/redirect guards and
// disabled SDK retries; otherwise a test seam could mask production semantics.
func TestCredentialTransportGuards(t *testing.T) {
	for _, provider := range []string{"responses", "anthropic", "openai"} {
		for _, stream := range []bool{false, true} {
			for _, status := range []int{http.StatusTemporaryRedirect, http.StatusServiceUnavailable} {
				name := provider + "/Chat/" + http.StatusText(status)
				if stream {
					name = provider + "/Stream/" + http.StatusText(status)
				}
				t.Run(name, func(t *testing.T) {
					owner, _ := busyOwner(t)
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					calls := 0
					wire := busyWire(func(r *http.Request) (*http.Response, error) {
						calls++
						if r.GetBody != nil {
							t.Error("admitted body replayable")
						}
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://redirect.invalid/never"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"overloaded_error","message":"synthetic rejection"}}`)), Request: r}, nil
					})
					c := busyClient(provider, owner, wire)
					var err error
					if stream {
						var r llm.StreamReader
						r, err = c.StreamChat(ctx, llm.ChatRequest{Model: "fixture"})
						if r != nil {
							defer r.Close()
							if err == nil {
								_, _, err = r.Next()
							}
						}
					} else {
						_, err = c.Chat(ctx, llm.ChatRequest{Model: "fixture"})
					}
					if status == http.StatusServiceUnavailable && err == nil {
						t.Fatal("upstream failure accepted")
					}
					if status == http.StatusServiceUnavailable && llm.ClassOf(err) != llm.ErrBusy {
						t.Fatalf("upstream Busy changed: %v", err)
					}
					if calls != 1 {
						t.Fatalf("redirect/retry wire calls=%d", calls)
					}
				})
			}
		}
	}
}
