package server

import (
	"cercano/source/server/internal/credentialtest"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"cercano/source/server/internal/modelevidence"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func TestMistralEndpointChangeAndProfileRecreation(t *testing.T) {
	credentialtest.Enable(t)
	api := func(contextSize int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"data":[{"id":"same","max_context_length":%d,"capabilities":{"completion_chat":true,"function_calling":true}}]}`, contextSize)
		}))
	}
	first := api(4096)
	defer first.Close()
	second := api(8192)
	defer second.Close()
	s, _ := newTestServer()
	p := config.CloudProfile{Name: "work", Provider: "mistral", Flavor: "chat_completions", BaseURL: first.URL}
	s.cfgSvc.Set(config.Config{CloudProfiles: []config.CloudProfile{p}})
	if err := s.cfgSvc.Secrets().Set("work", "fixture"); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if got := s.modelEvidence().Resolve(ctx, modelevidence.IdentityFor(p, "same")); got.ContextWindow != 4096 {
		t.Fatalf("initial=%+v", got)
	}
	changed, err := s.UpsertCloudProfile(ctx, &proto.UpsertCloudProfileRequest{Name: "work", BaseUrl: second.URL})
	if err != nil || !changed.Ok {
		t.Fatalf("change=%v err=%v", changed, err)
	}
	if got := s.modelEvidence().Resolve(ctx, modelevidence.IdentityFor(p, "same")); got.ContextWindow != 0 {
		t.Fatalf("old endpoint evidence survived: %+v", got)
	}
	p.BaseURL = second.URL
	if got := s.modelEvidence().Resolve(ctx, modelevidence.IdentityFor(p, "same")); got.ContextWindow != 8192 {
		t.Fatalf("new endpoint=%+v", got)
	}
	removed, err := s.RemoveCloudProfile(ctx, &proto.RemoveCloudProfileRequest{Name: "work"})
	if err != nil || !removed.Ok {
		t.Fatalf("remove=%v %v", removed, err)
	}
	provider := "mistral"
	recreated, err := s.UpsertCloudProfile(ctx, &proto.UpsertCloudProfileRequest{Name: "work", Flavor: "chat_completions", BaseUrl: first.URL, Provider: &provider, CreateOnly: true})
	if err != nil || !recreated.Ok {
		t.Fatalf("recreate=%v %v", recreated, err)
	}
	listed, err := s.ListCloudProfileModels(ctx, &proto.ListCloudProfileModelsRequest{ProfileName: "work"})
	if err != nil || listed.Error == "" || len(listed.Models) != 0 {
		t.Fatalf("recreated account inherited old key/access: %v %v", listed, err)
	}
}

func TestMistralDirectRouteRecognition(t *testing.T) {
	for _, route := range []string{"", "direct"} {
		if !isMistralProfile(config.CloudProfile{Flavor: "chat_completions", Route: route, BaseURL: "https://api.mistral.ai/v1"}) {
			t.Fatalf("direct route %q not recognized", route)
		}
	}
	if isMistralProfile(config.CloudProfile{Flavor: "chat_completions", Route: "subscription", BaseURL: "https://api.mistral.ai/v1"}) {
		t.Fatal("subscription misidentified as static Mistral")
	}
}
