package server

import (
	"cercano/source/server/internal/credentialtest"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func TestMistralSessionSelectionRevalidatesAccount(t *testing.T) {
	credentialtest.Enable(t)
	var revoked atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer work-key" {
			t.Error("wrong account credential")
		}
		if revoked.Load() {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"exact","capabilities":{"completion_chat":true,"function_calling":true}},{"id":"retired","deprecation":"2000-01-01","capabilities":{"completion_chat":true,"function_calling":true}}]}`)
	}))
	defer api.Close()
	s, _ := newServerWithStore(t)
	cfg := config.Defaults()
	cfg.CloudProfiles = []config.CloudProfile{{Name: "work", Provider: "mistral", Flavor: "chat_completions", BaseURL: api.URL + "/v1"}}
	s.SetConfigPersistence("", cfg)
	keys := secrets.NewMemory()
	if err := keys.Set("work", "work-key"); err != nil {
		t.Fatal(err)
	}
	s.SetSecrets(keys)
	req := &proto.SessionModelRequest{ConversationId: "one", Action: "set", Profile: "work", Model: "exact"}
	if _, err := s.SessionModel(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	req.Model = "retired"
	if _, err := s.SessionModel(t.Context(), req); err == nil {
		t.Fatal("retired model selected")
	}
	req.Model = "foreign-private"
	if _, err := s.SessionModel(t.Context(), req); err == nil {
		t.Fatal("foreign model selected")
	}
	for _, tc := range []struct {
		model string
		ok    bool
	}{{"exact", true}, {"foreign-private", false}, {"retired", false}} {
		saved, err := s.UpsertCloudProfile(t.Context(), &proto.UpsertCloudProfileRequest{Name: "work", ModelChoices: &proto.ProfileModelChoices{TierOverrides: map[string]string{"standard": tc.model}}})
		if err != nil || saved.Ok != tc.ok {
			t.Fatalf("save %s: %v %v", tc.model, saved, err)
		}
	}
	saved, err := s.UpsertCloudProfile(t.Context(), &proto.UpsertCloudProfileRequest{Name: "work", ModelChoices: &proto.ProfileModelChoices{ImageModel: "exact"}})
	if err != nil || saved.Ok {
		t.Fatalf("unconfirmed vision accepted: %v %v", saved, err)
	}
	revoked.Store(true)
	req.Model = "exact"
	if _, err := s.SessionModel(t.Context(), req); err == nil {
		t.Fatal("cached availability bypassed revalidation")
	}
}
