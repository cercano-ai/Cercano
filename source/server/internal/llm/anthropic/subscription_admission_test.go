package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
)

type subscriptionAttemptSource struct {
	owner   secrets.RequestCredentials
	profile string
	after   func()
}

func (s subscriptionAttemptSource) CredentialProfile() string { return s.profile }
func (s subscriptionAttemptSource) Token(context.Context) (string, error) {
	panic("legacy Token called")
}
func (s subscriptionAttemptSource) SubscriptionSnapshot(ctx context.Context) (secrets.CredentialSnapshot, error) {
	snap, err := s.owner.CredentialSnapshot(ctx, s.profile)
	if err == nil && s.after != nil {
		s.after()
	}
	return snap, err
}

// Deterministic barriers at snapshot and successful admission; no physical-send
// ordering claim. All issuer/material and wire dependencies are memory-only.
func TestAnthropicSubscriptionAdmission(t *testing.T) {
	for _, mode := range []string{"fresh", "logout-before", "logout-after", "profile-mismatch", "source-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			owner, other := attemptOwners(t) // EnableHermetic, memory backend only.
			t.Setenv("ANTHROPIC_BASE_URL", "https://unexpected.invalid")
			forbidden := &credentialtest.ForbiddenTransport{}
			http.DefaultTransport = forbidden
			t.Cleanup(func() {
				if forbidden.Calls.Load() != 0 {
					t.Error("default transport used")
				}
			})
			payload, err := (anthropicauth.TokenSet{Access: "snapshot-access", Refresh: "refresh", ExpiresAt: time.Now().Add(time.Hour)}).Encode()
			if err != nil {
				t.Fatal(err)
			}
			if err := other.Set("profile", payload); err != nil {
				t.Fatal(err)
			}
			logout := func() {
				if err := other.Delete("profile"); err != nil {
					t.Fatal(err)
				}
			}
			source := subscriptionAttemptSource{owner: owner, profile: "profile"}
			if mode == "logout-before" {
				source.after = logout
			}
			if mode == "profile-mismatch" {
				source.profile = "other"
			}
			var authority secrets.RequestCredentials = owner
			if mode == "logout-after" {
				authority = attemptBarrier{RequestCredentials: owner, admitted: logout}
			}
			calls := 0
			cfg := Config{Route: RouteSubscription, CredentialProfile: "profile", Credentials: authority, TokenSource: source}
			if mode == "source-mismatch" {
				cfg.TokenSource = nil
			}
			c := fakeMessages(cfg, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "api.anthropic.com" {
					t.Errorf("subscription endpoint host = %q; want api.anthropic.com", r.URL.Host)
				}
				if r.Header.Get("X-Api-Key") != "" || r.Header.Get("Authorization") != "Bearer snapshot-access" || r.Header.Get("anthropic-beta") != oauthBetaHeader {
					t.Error("subscription header identity")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				var envelope struct {
					System []struct {
						Text string `json:"text"`
					} `json:"system"`
				}
				if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.System) == 0 || envelope.System[0].Text != claudeCodeIdentity {
					t.Error("missing leading Claude Code identity")
				}
				return messageResponse(), nil
			})
			err = callMessages(c, false)
			allowed := mode == "fresh" || mode == "logout-after"
			if allowed && (err != nil || calls != 1) {
				t.Fatalf("admitted attempt: calls=%d err=%v", calls, err)
			}
			if !allowed && (err == nil || calls != 0) {
				t.Fatalf("refused attempt: calls=%d err=%v", calls, err)
			}
		})
	}
}

// Chat temperature fallback obtains a new snapshot. Streaming has no fallback.
func TestAnthropicSubscriptionTemperatureRetry(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "chat", true: "stream"}[stream], func(t *testing.T) {
			owner, other := attemptOwners(t)
			deny := &credentialtest.ForbiddenTransport{}
			http.DefaultTransport = deny
			t.Cleanup(func() {
				if deny.Calls.Load() != 0 {
					t.Error("default wire attempted")
				}
			})
			payload, _ := (anthropicauth.TokenSet{Access: "access", ExpiresAt: time.Now().Add(time.Hour)}).Encode()
			if err := other.Set("profile", payload); err != nil {
				t.Fatal(err)
			}
			counted := &messagesCountedCredentials{RequestCredentials: owner}
			source := subscriptionAttemptSource{owner: counted, profile: "profile"}
			calls := 0
			c := fakeMessages(Config{Route: RouteSubscription, CredentialProfile: "profile", Credentials: counted, TokenSource: source}, func(*http.Request) (*http.Response, error) {
				calls++
				if err := other.Delete("profile"); err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"temperature deprecated"}}`))}, nil
			})
			temp := 0.2
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req := ChatRequest{Model: "fixture", Temperature: &temp}
			var err error
			if stream {
				r, e := c.StreamChat(ctx, req)
				err = e
				if e == nil {
					defer r.Close()
					_, _, err = r.Next()
				}
			} else {
				_, err = c.Chat(ctx, req)
			}
			wantSnapshots := 2
			if stream {
				wantSnapshots = 1
			}
			if err == nil || calls != 1 || counted.snapshots != wantSnapshots {
				t.Fatalf("err=%v wire=%d snapshots=%d", err, calls, counted.snapshots)
			}
		})
	}
}

// Explicit config must also lose to the subscription pin; direct clients retain
// both explicit config and SDK environment endpoint selection.
func TestAnthropicEndpointSelection(t *testing.T) {
	for _, tc := range []struct{ name, route, base, want string }{
		{"subscription-config", RouteSubscription, "https://config.invalid", "api.anthropic.com"},
		{"direct-config", "", "https://config.invalid", "config.invalid"},
		{"direct-env", "", "", "unexpected.invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, other := attemptOwners(t)
			t.Setenv("ANTHROPIC_BASE_URL", "https://unexpected.invalid")
			payload, _ := (anthropicauth.TokenSet{Access: "access", ExpiresAt: time.Now().Add(time.Hour)}).Encode()
			if err := other.Set("profile", payload); err != nil {
				t.Fatal(err)
			}
			calls := 0
			cfg := Config{Route: tc.route, BaseURL: tc.base, APIKey: "fake"}
			if tc.route == RouteSubscription {
				cfg.Credentials = owner
				cfg.CredentialProfile = "profile"
				cfg.TokenSource = subscriptionAttemptSource{owner: owner, profile: "profile"}
			}
			c := fakeMessages(cfg, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != tc.want {
					t.Errorf("host=%q want=%q", r.URL.Host, tc.want)
				}
				return messageResponse(), nil
			})
			if err := callMessages(c, false); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}
