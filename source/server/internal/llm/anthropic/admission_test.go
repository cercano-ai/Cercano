package anthropic

import (
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/hostsvc/credentials"
	"cercano/source/server/internal/modelpolicy"
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type admissionWire func(*http.Request) (*http.Response, error)

func (f admissionWire) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func messageResponse() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fixture","type":"message","role":"assistant","model":"fixture","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))}
}

// Characterization of the legacy reusable-key gap; no network or native store.
func TestLegacyMessagesRetainedKeyLogoutGap(t *testing.T) {
	owner, other := attemptOwners(t)
	snap, err := owner.CredentialSnapshot(context.Background(), "profile")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := fakeMessages(Config{APIKey: snap.Payload}, admissionWire(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != snap.Payload {
			t.Fatal("fixture auth did not match synthetic snapshot exclusively")
		}
		return messageResponse(), nil
	}))
	if err := other.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if err := owner.AdmitAttempt(context.Background(), "profile", snap.Revision); !errors.Is(err, credentials.ErrStale) {
		t.Fatalf("logout did not commit: %v", err)
	}
	if err := callMessages(c, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

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

// Inject before SDK construction; exercise the real wrapper stack.
func fakeMessages(cfg Config, wire admissionWire) *Client {
	cfg.BaseTransport = wire
	return NewClient(cfg)
}
func callMessages(c *Client, stream bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if !stream {
		_, err := c.Chat(ctx, ChatRequest{Model: "fixture"})
		return err
	}
	r, err := c.StreamChat(ctx, ChatRequest{Model: "fixture"})
	if err != nil {
		return err
	}
	defer r.Close()
	_, _, err = r.Next()
	if err == io.EOF {
		return nil
	}
	return err
}
func TestMessagesSnapshotLogoutRace(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "Chat", true: "Stream"}[stream], func(t *testing.T) {
			owner, other := attemptOwners(t)
			reached := make(chan struct{})
			release, releaseGate := fixtureGate(t)
			calls := 0
			c := fakeMessages(Config{Credentials: attemptBarrier{RequestCredentials: owner, snapshot: func() { close(reached); waitGate(release) }}, CredentialProfile: "profile"}, func(*http.Request) (*http.Response, error) { calls++; return messageResponse(), nil })
			done := make(chan error, 1)
			exited := make(chan struct{})
			t.Cleanup(func() { releaseGate(); awaitGate(t, exited) })
			go func() { defer close(exited); done <- callMessages(c, stream) }()
			awaitGate(t, reached)
			err := other.Delete("profile")
			releaseGate()
			got := awaitResult(t, done)
			if err != nil || got == nil || calls != 0 {
				t.Fatalf("logout=%v request=%v calls=%d", err, got, calls)
			}
		})
	}
}
func TestMessagesAdmittedBeforeLogout(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "Chat", true: "Stream"}[stream], func(t *testing.T) {
			owner, other := attemptOwners(t)
			admitted := make(chan struct{})
			release, releaseGate := fixtureGate(t)
			arrived := make(chan struct{})
			finish, finishGate := fixtureGate(t)
			calls := 0
			c := fakeMessages(Config{APIKey: "wrong", Credentials: attemptBarrier{RequestCredentials: owner, admitted: func() { close(admitted); waitGate(release) }}, CredentialProfile: "profile"}, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("X-Api-Key") != "fixture" || r.Header.Get("Authorization") != "" || r.GetBody != nil {
					t.Error("incorrect wire auth/replay")
				}
				close(arrived)
				waitGate(finish)
				if stream {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))}, nil
				}
				return messageResponse(), nil
			})
			done := make(chan error, 1)
			exited := make(chan struct{})
			t.Cleanup(func() { releaseGate(); finishGate(); awaitGate(t, exited) })
			go func() { defer close(exited); done <- callMessages(c, stream) }()
			awaitGate(t, admitted)
			err := other.Delete("profile")
			releaseGate()
			awaitGate(t, arrived)
			_, snapshotErr := owner.CredentialSnapshot(context.Background(), "profile")
			finishGate()
			got := awaitResult(t, done)
			if err != nil || got != nil || !errors.Is(snapshotErr, credentials.ErrStale) {
				t.Fatalf("logout=%v request=%v snapshot=%v", err, got, snapshotErr)
			}
			if err := callMessages(c, stream); err == nil || calls != 1 {
				t.Fatalf("fresh request=%v calls=%d", err, calls)
			}
		})
	}
}
func TestMessagesRetryAndRedirect(t *testing.T) {
	for _, status := range []int{400, 429, 500, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			owner, other := attemptOwners(t)
			counted := &messagesCountedCredentials{RequestCredentials: owner}
			calls := 0
			c := fakeMessages(Config{Credentials: counted, CredentialProfile: "profile"}, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 1 {
					t.Fatal("unexpected wire retry")
				}
				if err := other.Delete("profile"); err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}, "Location": []string{"https://fixture.invalid/redirect"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"temperature deprecated"}}`))}, nil
			})
			temp := 0.2
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := c.Chat(ctx, ChatRequest{Model: "fixture", Temperature: &temp})
			if (status != 307 && err == nil) || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if status == 400 && counted.snapshots != 2 {
				t.Fatalf("fallback snapshots=%d", counted.snapshots)
			}
			if status == 400 && !c.tempDeprecated.Load("fixture") {
				t.Fatal("fallback not exercised")
			}
		})
	}
}

func TestMessagesSDKRetriesDisabled(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, status := range []int{429, 500} {
			owner, _ := attemptOwners(t)
			calls := 0
			c := fakeMessages(Config{Credentials: owner, CredentialProfile: "profile"}, func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"api_error","message":"fixture"}}`))}, nil
			})
			if err := callMessages(c, stream); err == nil || calls != 1 {
				t.Fatalf("stream=%v status=%d error=%v calls=%d", stream, status, err, calls)
			}
		}
	}
}
func TestMessagesPolicyBeforeReplayRemoval(t *testing.T) {
	owner, _ := attemptOwners(t)
	calls, checks := 0, 0
	c := fakeMessages(Config{Credentials: owner, CredentialProfile: "profile"}, func(r *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil || r.GetBody != nil || !strings.Contains(string(body), `"model":"approved"`) {
			t.Error("body/replay constraints lost")
		}
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://fixture.invalid/redirect"}}, Body: http.NoBody, Request: r}, nil
	})
	parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx := modelpolicy.WithAuthority(parent, modelpolicy.AuthorizeFunc(func(_ context.Context, a modelpolicy.Attempt) error {
		checks++
		if a.Provider != "anthropic" || a.Placement != "external" || a.Endpoint != "https://api.anthropic.com" {
			t.Errorf("policy identity: %+v", a)
		}
		if a.Model != "approved" {
			return modelpolicy.Deny(a, "not approved")
		}
		return nil
	}))
	for _, model := range []string{"forbidden", "approved"} {
		_, err := c.Chat(ctx, ChatRequest{Model: model})
		if err == nil {
			t.Fatal("expected policy/redirect denial")
		}
		if model == "forbidden" && calls != 0 {
			t.Fatal("denied model reached wire")
		}
	}
	if calls != 1 || checks != 2 {
		t.Fatalf("wire=%d checks=%d", calls, checks)
	}
}
func TestMessagesUnsupportedDirectConfig(t *testing.T) {
	for _, route := range []string{"subscription", "meridian", "unknown"} {
		owner, _ := attemptOwners(t)
		c := fakeMessages(Config{Credentials: owner, CredentialProfile: "profile", Route: route}, func(*http.Request) (*http.Response, error) {
			t.Fatal("unsupported route reached wire")
			return nil, nil
		})
		if err := callMessages(c, false); err == nil {
			t.Fatal("unsupported route accepted")
		}
	}
}

func TestMessagesTransportConstraints(t *testing.T) {
	transport := messagesAttemptTransport().next.(*http.Transport)
	if !transport.DisableKeepAlives || transport.ForceAttemptHTTP2 || transport.TLSNextProto == nil || len(transport.TLSNextProto) != 0 {
		t.Fatal("fresh HTTP/1 constraints missing")
	}
	calls := 0
	wire := messagesNonReplay{next: admissionWire(func(*http.Request) (*http.Response, error) { calls++; return messageResponse(), nil })}
	for _, kind := range []string{"empty", "unknown-length", "get", "idempotent", "x-idempotent"} {
		r, _ := http.NewRequest("POST", "https://fixture.invalid", strings.NewReader("body"))
		switch kind {
		case "empty":
			r.ContentLength = 0
		case "unknown-length":
			r.ContentLength = -1
		case "get":
			r.Method = "GET"
		case "idempotent":
			r.Header["Idempotency-Key"] = nil
		case "x-idempotent":
			r.Header["X-Idempotency-Key"] = nil
		}
		if _, err := wire.RoundTrip(r); err == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
	if calls != 0 {
		t.Fatal("unsafe request reached wire")
	}
}

type messagesCountedCredentials struct {
	secrets.RequestCredentials
	snapshots int
}

func (c *messagesCountedCredentials) CredentialSnapshot(ctx context.Context, p string) (secrets.CredentialSnapshot, error) {
	c.snapshots++
	return c.RequestCredentials.CredentialSnapshot(ctx, p)
}

// All waits are bounded even if a constructor or wrapper regresses.
func waitGate(ch <-chan struct{}) {
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
	}
}
func awaitGate(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture barrier timed out")
	}
}
func awaitResult(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("fixture request timed out")
		return nil
	}
}

func TestMessagesConstructorIsolation(t *testing.T) {
	for _, kind := range []string{"omitted", "misapplied", "injected"} {
		t.Run(kind, func(t *testing.T) {
			owner, _ := attemptOwners(t) // deny default BEFORE constructing SDK
			trap := &credentialtest.ForbiddenTransport{}
			cfg := Config{Credentials: owner, CredentialProfile: "profile"}
			calls := 0
			fake := admissionWire(func(*http.Request) (*http.Response, error) { calls++; return messageResponse(), nil })
			if kind == "injected" {
				cfg.BaseTransport = fake
			}
			if kind == "misapplied" {
				unused := cfg
				unused.BaseTransport = fake // intentionally wrong config, never mutate SDK options
				_ = unused
			}
			c := newClientWithFallback(cfg, trap)
			err := callMessages(c, false)
			if kind == "injected" {
				if err != nil || calls != 1 || trap.Calls.Load() != 0 {
					t.Fatalf("fake=%d trap=%d error=%v", calls, trap.Calls.Load(), err)
				}
			} else if err == nil || calls != 0 || trap.Calls.Load() != 1 {
				t.Fatalf("missing fake not trapped: fake=%d trap=%d error=%v", calls, trap.Calls.Load(), err)
			}
		})
	}
}

func fixtureGate(t *testing.T) (chan struct{}, func()) {
	ch := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(ch) }) }
	t.Cleanup(release)
	return ch, release
}
