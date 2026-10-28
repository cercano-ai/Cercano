package providers

import (
	"cercano/source/server/internal/hostsvc/credentials"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm/anthropic"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

type hostAnthropicWire func(*http.Request) (*http.Response, error)

func (f hostAnthropicWire) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCoordinatedAnthropicHostGraph(t *testing.T) {
	credentialtest.EnableHermetic(t)
	deny := &credentialtest.ForbiddenTransport{}
	http.DefaultTransport = deny
	issuer := &credentialtest.ForbiddenTransport{}
	flow := anthropicauth.Flow{TokenURL: "https://issuer.invalid/token", Client: &http.Client{Transport: issuer}}
	authority := &hostCredentialFake{Store: secrets.NewMemory()}
	prof := config.CloudProfile{Name: "subscription-profile", Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription, Model: "fixture", BaseURL: "https://must-not-use.invalid"}
	c := config.Defaults()
	c.LocusMode = "cloud_only"
	c.ActiveCloudProfile = prof.Name
	c.CloudProfiles = []config.CloudProfile{prof}
	s := &service{cfgSvc: cfgsvc.New("", c, authority), router: &recordingRouter{}, anthropicFlow: &flow}
	var client *anthropic.Client
	calls := 0
	s.SetCloudBuilder(func(got config.CloudProfile, key string, opts ...cloudfactory.Options) (inference.Provider, error) {
		opts[0].AnthropicBaseTransport = hostAnthropicWire(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "api.anthropic.com" || r.Header.Get("Authorization") != "Bearer fake-access" || r.Header.Get("X-Api-Key") != "" {
				t.Error("endpoint or auth identity mismatch")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fixture","type":"message","role":"assistant","model":"fixture","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))}, nil
		})
		p, err := cloudfactory.BuildCloudProvider(got, key, opts...)
		if err == nil {
			client = p.(*anthropic.Client)
		}
		return p, err
	})
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("missing client")
	}
	if _, err := client.Chat(context.Background(), anthropic.ChatRequest{Model: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || issuer.Calls.Load() != 0 || deny.Calls.Load() != 0 {
		t.Fatalf("wire=%d issuer=%d default=%d", calls, issuer.Calls.Load(), deny.Calls.Load())
	}
}

// Memory-only record backend; the coordinator uses only temporary exclusion locks.
type hostAnthropicRecords struct{ raw string }

func (b *hostAnthropicRecords) Read(string) (string, error)        { return b.raw, nil }
func (b *hostAnthropicRecords) Replace(_ string, raw string) error { b.raw = raw; return nil }
func (b *hostAnthropicRecords) Keys() ([]string, error)            { return []string{"profile"}, nil }

func TestCoordinatedAnthropicHostRefresh(t *testing.T) {
	for _, mode := range []string{"refresh", "logout", "issuer-uncertain"} {
		t.Run(mode, func(t *testing.T) {
			// Construct every fake dependency while credentials are still disabled.
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			backend := &hostAnthropicRecords{raw: `{"version":1,"revision":"` + strings.Repeat("ab", 32) + `","state":"active","payload":"fixture"}`}
			authority, err := credentials.NewCoordinatedStore(backend, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			deny := &credentialtest.ForbiddenTransport{}
			issuerCalls, wireCalls := 0, 0
			flow := anthropicauth.Flow{TokenURL: "https://issuer.invalid/token", Client: &http.Client{Transport: hostAnthropicWire(func(r *http.Request) (*http.Response, error) {
				issuerCalls++
				if r.URL.Host != "issuer.invalid" || r.URL.Path != "/token" {
					t.Fatal("wrong issuer")
				}
				if !strings.Contains(backend.raw, `"state":"pending"`) {
					t.Fatal("issuer without pending record")
				}
				if mode == "logout" {
					if err := authority.Delete("profile"); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "issuer-uncertain" {
					return nil, errors.New("fake lost issuer acknowledgement")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"rotated-access","refresh_token":"rotated-refresh","expires_in":3600}`))}, nil
			})}}
			wire := hostAnthropicWire(func(r *http.Request) (*http.Response, error) {
				wireCalls++
				if r.URL.Host != "api.anthropic.com" || r.Header.Get("Authorization") != "Bearer rotated-access" || r.Header.Get("X-Api-Key") != "" || r.GetBody != nil {
					t.Error("endpoint/auth/replay mismatch")
				}
				snap, err := authority.CredentialSnapshot(r.Context(), "profile")
				if err != nil {
					t.Fatal(err)
				}
				tokens, err := anthropicauth.DecodeTokenSet(snap.Payload)
				if err != nil || tokens.Access != "rotated-access" || tokens.Refresh != "rotated-refresh" {
					t.Fatal("wire before committed rotation")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fixture","type":"message","role":"assistant","model":"fixture","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))}, nil
			})
			prof := config.CloudProfile{Name: "profile", Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription, Model: "fixture", BaseURL: "https://config.invalid"}
			c := config.Defaults()
			c.LocusMode, c.ActiveCloudProfile, c.CloudProfiles = "cloud_only", prof.Name, []config.CloudProfile{prof}
			s := &service{cfgSvc: cfgsvc.New("", c, authority), router: &recordingRouter{}, anthropicFlow: &flow}
			var client *anthropic.Client
			s.SetCloudBuilder(func(p config.CloudProfile, key string, opts ...cloudfactory.Options) (inference.Provider, error) {
				opts[0].AnthropicBaseTransport = wire
				result, err := cloudfactory.BuildCloudProvider(p, key, opts...)
				if err == nil {
					client = result.(*anthropic.Client)
				}
				return result, err
			})
			credentialtest.EnableHermetic(t)
			http.DefaultTransport = deny
			t.Setenv("ANTHROPIC_BASE_URL", "https://unexpected.invalid")
			if err := anthropicauth.Save(authority, "profile", anthropicauth.TokenSet{Access: "expired-access", Refresh: "single-use", ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
				t.Fatal(err)
			}
			before, err := authority.CredentialSnapshot(context.Background(), "profile")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Rebuild(); err != nil {
				t.Fatal(err)
			}
			if client == nil {
				t.Fatal("missing client")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			for i := 0; i < 2; i++ {
				_, err := client.Chat(ctx, anthropic.ChatRequest{Model: "fixture"})
				if (mode == "refresh") != (err == nil) {
					t.Fatalf("mode=%s request=%d err=%v", mode, i, err)
				}
			}
			wantWire := 0
			if mode == "refresh" {
				wantWire = 2
				after, err := authority.CredentialSnapshot(ctx, "profile")
				if err != nil || after.Revision == before.Revision {
					t.Fatal("rotation revision missing")
				}
			}
			if issuerCalls != 1 || wireCalls != wantWire || deny.Calls.Load() != 0 {
				t.Fatalf("issuer=%d wire=%d default=%d", issuerCalls, wireCalls, deny.Calls.Load())
			}
		})
	}
}
