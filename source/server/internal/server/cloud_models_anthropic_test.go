package server

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

type anthropicCatalogTransport func(*http.Request) (*http.Response, error)

func (f anthropicCatalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestListCloudProfileModels_AnthropicDefaultsAndAuth(t *testing.T) {
	credentialtest.Enable(t)
	for _, route := range []string{"", "subscription"} {
		for _, base := range []string{"", "https://custom.example"} {
			t.Run(route+"/"+base, func(t *testing.T) {
				s, _ := newTestServer()
				s.cfgSvc.Set(config.Config{ActiveCloudProfile: "claude", CloudProfiles: []config.CloudProfile{{Name: "claude", Flavor: "messages", Provider: "anthropic", Route: route, BaseURL: base}}})
				if route == "subscription" {
					if err := anthropicauth.Save(s.cfgSvc.Secrets(), "claude", anthropicauth.TokenSet{Access: "subscription-access", Refresh: "refresh", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
						t.Fatal(err)
					}
				} else if err := s.cfgSvc.Secrets().Set("claude", "api-key"); err != nil {
					t.Fatal(err)
				}
				old := http.DefaultTransport
				t.Cleanup(func() { http.DefaultTransport = old })
				hits := 0
				http.DefaultTransport = anthropicCatalogTransport(func(r *http.Request) (*http.Response, error) {
					hits++
					wantBase := base
					if wantBase == "" {
						wantBase = "https://api.anthropic.com"
					}
					if r.URL.String() != wantBase+"/v1/models" {
						t.Errorf("URL = %s", r.URL)
					}
					if r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("missing version")
					}
					if route == "subscription" {
						if r.Header.Get("Authorization") != "Bearer subscription-access" || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" || r.Header.Get("x-api-key") != "" {
							t.Errorf("wrong subscription authentication")
						}
					} else if r.Header.Get("x-api-key") != "api-key" || r.Header.Get("Authorization") != "" {
						t.Error("wrong API-key authentication")
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"claude-fixture","display_name":"Claude fixture"}]}`))}, nil
				})
				resp, err := s.ListCloudProfileModels(context.Background(), &proto.ListCloudProfileModelsRequest{})
				if err != nil || resp.Error != "" {
					t.Fatalf("discovery failed: %v %v", resp, err)
				}
				if hits != 1 || len(resp.Models) != 1 || resp.Models[0].DisplayName != "Claude fixture" {
					t.Fatalf("unexpected catalog: %v (requests %d)", resp, hits)
				}
			})
		}
	}
}

func TestListCloudProfileModels_AnthropicSignedOut(t *testing.T) {
	s, _ := newTestServer()
	s.cfgSvc.Set(config.Config{ActiveCloudProfile: "signed-out", CloudProfiles: []config.CloudProfile{{Name: "signed-out", Flavor: "messages", Route: "subscription"}}})
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = anthropicCatalogTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("signed-out discovery made HTTP request")
		return nil, nil
	})
	resp, err := s.ListCloudProfileModels(context.Background(), &proto.ListCloudProfileModelsRequest{})
	if err != nil || resp.Error == "" || strings.Contains(resp.Error, "base_url") {
		t.Fatalf("expected authentication error, got %v %v", resp, err)
	}
}

func TestListCloudProfileModels_AnthropicRefusesRedirect(t *testing.T) {
	s, _ := newTestServer()
	s.cfgSvc.Set(config.Config{ActiveCloudProfile: "claude", CloudProfiles: []config.CloudProfile{{Name: "claude", Flavor: "messages"}}})
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	hits := 0
	http.DefaultTransport = anthropicCatalogTransport(func(r *http.Request) (*http.Response, error) {
		hits++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.example/models"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	resp, err := s.ListCloudProfileModels(context.Background(), &proto.ListCloudProfileModelsRequest{})
	if err != nil || !strings.Contains(resp.Error, "refused redirect") || hits != 1 {
		t.Fatalf("redirect not blocked: %v %v requests=%d", resp, err, hits)
	}
}
