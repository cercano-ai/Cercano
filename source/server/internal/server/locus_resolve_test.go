package server

import (
	"context"
	"testing"

	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/pkg/config"
)

type stubProv struct{ name string }

func (s stubProv) Name() string { return s.name }
func (s stubProv) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsTools: true}
}
func (s stubProv) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	panic("unused")
}
func (s stubProv) StreamChat(ctx context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	panic("unused")
}

func TestResolveMainProvider(t *testing.T) {
	cloud := stubProv{"anthropic"}
	local := stubProv{"ollama"}

	mk := func(mode string, c, l inference.Provider) *Server {
		s, _ := newTestServer()
		// A wired provider also needs an explicit model on a non-curated runtime.
		conf := config.Config{LocusMode: mode, OpenRuntime: "ollama"}
		conf.Models.SetOverride("ollama", conf.TaskAssignment(config.TaskChat).Quality.CapabilityTier(), "fixture-model")
		s.cfgSvc.Set(conf)
		s.providerSvc.SetCloudLLMProvider(c)
		s.providerSvc.SetOpenLLMProvider(l)
		return s
	}

	// Local Primary, both available → local, not fallback.
	if p, isCloud, fb, err := mk("open_primary", cloud, local).resolveMainProvider(); err != nil || isCloud || fb || p.Name() != "ollama" {
		t.Errorf("local_primary both: %v isCloud=%v fb=%v err=%v", p, isCloud, fb, err)
	}
	// Local Primary, local missing → fall back to cloud.
	if p, isCloud, fb, err := mk("open_primary", cloud, nil).resolveMainProvider(); err != nil || !isCloud || !fb || p.Name() != "anthropic" {
		t.Errorf("local_primary fallback: %v isCloud=%v fb=%v err=%v", p, isCloud, fb, err)
	}
	// Cloud Primary, cloud missing → fall back to local (mirror of the above).
	if p, isCloud, fb, err := mk("cloud_primary", nil, local).resolveMainProvider(); err != nil || isCloud || !fb || p.Name() != "ollama" {
		t.Errorf("cloud_primary fallback: %v isCloud=%v fb=%v err=%v", p, isCloud, fb, err)
	}
	// Cloud Only, cloud missing → hard error (no cross).
	if _, _, _, err := mk("cloud_only", nil, local).resolveMainProvider(); err == nil {
		t.Error("cloud_only with no cloud should error")
	}
	// Local Only, local missing → hard error (no cross even though cloud present).
	if _, _, _, err := mk("open_only", cloud, nil).resolveMainProvider(); err == nil {
		t.Error("local_only with no local should error")
	}
}

// A provider alone must not make an unconfigured non-curated runtime ready.
func TestResolveMainProviderOllamaWithoutModel(t *testing.T) {
	for _, mode := range []string{"open_primary", "cloud_primary", "open_only"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := newTestServer()
			s.cfgSvc.Set(config.Config{LocusMode: mode, OpenRuntime: "ollama"})
			s.providerSvc.SetCloudLLMProvider(stubProv{"anthropic"})
			s.providerSvc.SetOpenLLMProvider(stubProv{"ollama"})
			p, isCloud, fallback, err := s.resolveMainProvider()
			if mode == "open_only" {
				if err == nil {
					t.Fatal("open_only without a configured model should error")
				}
				return
			}
			if err != nil || p == nil || !isCloud || p.Name() != "anthropic" || fallback != (mode == "open_primary") {
				t.Fatalf("provider=%v isCloud=%v fallback=%v err=%v", p, isCloud, fallback, err)
			}
		})
	}
}
