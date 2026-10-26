package server

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// chatgptServer returns a test double standing in for the codex backend
// (https://chatgpt.com/backend-api/codex) plus a ChatGPT subscription profile.
// The profile's base_url stays empty, exactly as StartChatGPTLogin writes it —
// the route, not the profile, pins the backend.
func chatgptServer(t *testing.T, s *Server, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	fixture := httptest.NewServer(handler)
	t.Cleanup(fixture.Close)
	s.chatgptCatalogBase = fixture.URL
	s.cfgSvc.Set(config.Config{
		ActiveCloudProfile: "chatgpt",
		CloudProfiles: []config.CloudProfile{{
			Name:   "chatgpt",
			Flavor: "responses",
			Route:  "chatgpt",
		}},
	})
	return fixture
}

func signInChatGPT(t *testing.T, s *Server) {
	t.Helper()
	err := chatgptauth.Save(s.cfgSvc.Secrets(), "chatgpt", chatgptauth.TokenSet{
		Access:    "chatgpt-access",
		Refresh:   "chatgpt-refresh",
		ExpiresAt: time.Now().Add(time.Hour),
		AccountID: "acct-123",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListCloudProfileModels_ChatGPTListsCodexCatalog(t *testing.T) {
	credentialtest.Enable(t)
	s, _ := newTestServer()
	var sawPath, sawVersion, sawAuth, sawAccount string
	chatgptServer(t, s, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawVersion = r.URL.Query().Get("client_version")
		sawAuth = r.Header.Get("Authorization")
		sawAccount = r.Header.Get("ChatGPT-Account-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[
			{"slug":"gpt-5.5-mini","display_name":"GPT-5.5 mini","visibility":"list","priority":2},
			{"slug":"codex-internal","display_name":"internal","visibility":"hide","priority":0},
			{"slug":"gpt-5.5","display_name":"GPT-5.5","visibility":"list","priority":1}
		]}`))
	})
	signInChatGPT(t, s)

	resp, err := s.ListCloudProfileModels(context.Background(), &proto.ListCloudProfileModelsRequest{})
	if err != nil {
		t.Fatalf("ListCloudProfileModels: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if sawPath != "/models" {
		t.Errorf("catalog path: got %q want /models", sawPath)
	}
	if sawVersion == "" {
		t.Error("catalog request carried no client_version")
	}
	if sawAuth != "Bearer chatgpt-access" {
		t.Errorf("catalog auth: got %q want subscription bearer", sawAuth)
	}
	if sawAccount != "acct-123" {
		t.Errorf("ChatGPT-Account-Id: got %q want acct-123", sawAccount)
	}
	var got []string
	for _, m := range resp.Models {
		got = append(got, m.Id+"="+m.DisplayName)
	}
	want := "gpt-5.5=GPT-5.5,gpt-5.5-mini=GPT-5.5 mini"
	if strings.Join(got, ",") != want {
		t.Errorf("models: got %v want %s (picker-visible only, priority order)", got, want)
	}
}

func TestListCloudProfileModels_ChatGPTSignedOutDoesNotFetch(t *testing.T) {
	s, _ := newTestServer()
	var hits atomic.Int32
	chatgptServer(t, s, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	resp, err := s.ListCloudProfileModels(context.Background(), &proto.ListCloudProfileModelsRequest{})
	if err != nil {
		t.Fatalf("ListCloudProfileModels: %v", err)
	}
	if resp.Error == "" || strings.Contains(resp.Error, "base_url") {
		t.Errorf("signed-out error: got %q, want a credential error (not base_url)", resp.Error)
	}
	if hits.Load() != 0 {
		t.Errorf("catalog fetched %d times without a token", hits.Load())
	}
}

func TestListCloudProfileModels_ChatGPTUpstreamFailureIsReported(t *testing.T) {
	credentialtest.Enable(t)
	s, _ := newTestServer()
	chatgptServer(t, s, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	signInChatGPT(t, s)

	resp, err := s.ListCloudProfileModels(context.Background(), &proto.ListCloudProfileModelsRequest{})
	if err != nil {
		t.Fatalf("ListCloudProfileModels: %v", err)
	}
	if !strings.Contains(resp.Error, "403") || len(resp.Models) != 0 {
		t.Errorf("upstream 403: got error %q models %d, want 403 error and no models", resp.Error, len(resp.Models))
	}
}
