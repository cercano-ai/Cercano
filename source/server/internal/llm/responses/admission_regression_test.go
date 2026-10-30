package responses

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/hostsvc/credentials"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/modelpolicy"
	"cercano/source/server/internal/secrets"
)

// Memory records only; the temporary exclusion root is not a native store.
type attemptRecords struct {
	mu  sync.Mutex
	raw string
}

func (b *attemptRecords) Read(string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.raw, nil
}
func (b *attemptRecords) Replace(_ string, raw string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.raw = raw
	return nil
}
func (b *attemptRecords) Keys() ([]string, error) { return []string{"profile"}, nil }
func attemptOwners(t *testing.T) (secrets.RequestCredentials, *credentials.CoordinatedStore) {
	t.Helper()
	credentialtest.EnableHermetic(t)
	b := &attemptRecords{raw: `{"version":1,"revision":"` + strings.Repeat("ab", 32) + `","state":"active","payload":"fixture"}`}
	root := t.TempDir()
	a, err := credentials.NewCoordinatedStore(b, root)
	if err != nil {
		t.Fatal(err)
	}
	other, err := credentials.NewCoordinatedStore(b, root)
	if err != nil {
		t.Fatal(err)
	}
	return secrets.Guard(credentials.New(a), secrets.Options{ReadOnly: true}).(secrets.RequestCredentials), other
}

type attemptBarrier struct {
	secrets.RequestCredentials
	snapshot func()
	admitted func()
}

func (b attemptBarrier) CredentialSnapshot(ctx context.Context, p string) (secrets.CredentialSnapshot, error) {
	s, err := b.RequestCredentials.CredentialSnapshot(ctx, p)
	if err == nil && b.snapshot != nil {
		b.snapshot()
	}
	return s, err
}
func (b attemptBarrier) AdmitAttempt(ctx context.Context, p, r string) error {
	err := b.RequestCredentials.AdmitAttempt(ctx, p, r)
	if err == nil && b.admitted != nil {
		b.admitted()
	}
	return err
}

func TestCoordinatedSnapshotMutationRefusesHTTP(t *testing.T) {
	for _, mutation := range []string{"rotation", "logout"} {
		t.Run(mutation, func(t *testing.T) {
			owner, other := attemptOwners(t)
			reached, release := make(chan struct{}), make(chan struct{})
			c := NewClient(Config{Credentials: attemptBarrier{RequestCredentials: owner, snapshot: func() { close(reached); <-release }}, CredentialProfile: "profile"})
			calls := 0
			c.http = attemptDoer(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})
			done := make(chan error, 1)
			go func() {
				r, err := c.do(context.Background(), request{})
				if r != nil {
					r.Body.Close()
				}
				done <- err
			}()
			<-reached
			var err error
			if mutation == "rotation" {
				err = other.Set("profile", "rotated")
			} else {
				err = other.Delete("profile")
			}
			close(release)
			got := <-done
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(got, credentials.ErrStale) || calls != 0 {
				t.Fatalf("error=%v HTTP calls=%d", got, calls)
			}
		})
	}
}

func TestCoordinatedAdmittedAttemptOutlivesLogout(t *testing.T) {
	owner, other := attemptOwners(t)
	admitted, send := make(chan struct{}), make(chan struct{})
	arrived, complete := make(chan struct{}), make(chan struct{})
	c := NewClient(Config{Credentials: attemptBarrier{RequestCredentials: owner, admitted: func() { close(admitted); <-send }}, CredentialProfile: "profile"})
	c.http = attemptDoer(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("lost admitted payload")
		}
		close(arrived)
		<-complete
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	done := make(chan error, 1)
	go func() {
		r, err := c.do(context.Background(), request{})
		if r != nil {
			r.Body.Close()
		}
		done <- err
	}()
	<-admitted
	// This independent owner must acquire the credential exclusion before HTTP starts.
	err := other.Delete("profile")
	close(send)
	<-arrived
	// It must also be available while the HTTP operation is pending.
	_, snapshotErr := owner.CredentialSnapshot(context.Background(), "profile")
	close(complete)
	got := <-done
	if err != nil || got != nil || !errors.Is(snapshotErr, credentials.ErrStale) {
		t.Fatalf("logout=%v request=%v snapshot=%v", err, got, snapshotErr)
	}
}

type countedAttempts struct {
	secrets.RequestCredentials
	snapshots, attempts int
}

func (c *countedAttempts) CredentialSnapshot(ctx context.Context, p string) (secrets.CredentialSnapshot, error) {
	c.snapshots++
	return c.RequestCredentials.CredentialSnapshot(ctx, p)
}
func (c *countedAttempts) AdmitAttempt(ctx context.Context, p, r string) error {
	c.attempts++
	return c.RequestCredentials.AdmitAttempt(ctx, p, r)
}

func TestCoordinatedTemperatureFallbackReadmits(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "Chat"
		if stream {
			name = "StreamChat"
		}
		t.Run(name, func(t *testing.T) {
			f := &admissionFake{}
			counted := &countedAttempts{RequestCredentials: f}
			c := NewClient(Config{Credentials: counted, CredentialProfile: "profile", Model: "fixture"})
			calls := 0
			c.http = attemptDoer(func(r *http.Request) (*http.Response, error) {
				calls++
				if f.admissions != 1 {
					t.Error("first request not admitted")
				}
				f.loggedOut = true
				return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Unsupported parameter: temperature"}}`)), Header: make(http.Header), Request: r}, nil
			})
			temperature := 0.5
			req := llm.ChatRequest{Model: "fixture", Temperature: &temperature}
			var err error
			if stream {
				s, e := c.StreamChat(context.Background(), req)
				err = e
				if s != nil {
					s.Close()
				}
			} else {
				_, err = c.Chat(context.Background(), req)
			}
			if err == nil || !strings.Contains(err.Error(), "logged out") || calls != 1 || f.admissions != 1 {
				t.Fatalf("error=%v calls=%d admissions=%d", err, calls, f.admissions)
			}
			if counted.snapshots != 2 || counted.attempts != 2 {
				t.Fatalf("snapshots=%d admission checks=%d", counted.snapshots, counted.attempts)
			}
			if !c.tempUnsupportedLoad("fixture") {
				t.Fatal("temperature fallback not reached")
			}
		})
	}
}

func TestCoordinatedPolicyAndWireComposition(t *testing.T) {
	f := &admissionFake{}
	c := NewClient(Config{Credentials: f, CredentialProfile: "profile"})
	base := c.http.(*http.Client)
	calls, checks := 0, 0
	// Preserve the production redirect callback; replace only the transport with
	// the same policy -> nonReplay -> wire order and an entirely fake wire.
	base.Transport = nonReplayTransport{next: attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.GetBody != nil || r.Method != "POST" || r.ContentLength == 0 {
			t.Error("wire replay constraints lost")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(string(body), `"model":"approved"`) {
			t.Error("policy changed body")
		}
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://fixture.invalid/redirect"}}, Body: http.NoBody, Request: r}, nil
	})}
	c.http = modelpolicy.Client(base, "openai", "external", modelpolicy.Responses)
	ctx := modelpolicy.WithAuthority(context.Background(), modelpolicy.AuthorizeFunc(func(_ context.Context, a modelpolicy.Attempt) error {
		checks++
		if a.Provider != "openai" || a.Placement != "external" || a.Endpoint != defaultBaseURL {
			t.Error("policy identity lost")
		}
		if a.Model != "approved" {
			return modelpolicy.Deny(a, "not approved")
		}
		return nil
	}))
	for _, model := range []string{"forbidden", "approved"} {
		r, err := c.do(ctx, request{Model: model})
		if r != nil {
			r.Body.Close()
		}
		var denied *modelpolicy.Denial
		if !errors.As(err, &denied) {
			t.Fatalf("%s: expected policy/redirect denial, got %v", model, err)
		}
		if model == "forbidden" && calls != 0 {
			t.Fatal("denied model reached wire")
		}
	}
	if calls != 1 || checks != 2 {
		t.Fatalf("wire=%d policy=%d", calls, checks)
	}
}

// Reuse one constructed client: each new attempt resolves the current active
// snapshot. This is not a retry of an already-admitted attempt or stale login.
func TestCoordinatedCachedClientFreshActiveRevision(t *testing.T) {
	owner, other := attemptOwners(t)
	c := NewClient(Config{Credentials: owner, CredentialProfile: "profile"})
	var sent []string
	c.http = attemptDoer(func(r *http.Request) (*http.Response, error) {
		sent = append(sent, r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	send := func() {
		t.Helper()
		r, err := c.do(context.Background(), request{})
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
	}
	send()
	if err := other.Set("profile", "replacement-fixture"); err != nil {
		t.Fatal(err)
	}
	send()
	if len(sent) != 2 || sent[0] != "Bearer fixture" || sent[1] != "Bearer replacement-fixture" {
		t.Fatalf("cached credentials: %v", sent)
	}
	if err := other.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.do(context.Background(), request{}); !errors.Is(err, credentials.ErrStale) {
		t.Fatalf("logged-out cached client: %v", err)
	}
	if len(sent) != 2 {
		t.Fatal("logged-out request sent")
	}
}
