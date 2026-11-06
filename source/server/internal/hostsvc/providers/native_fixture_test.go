package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime/catalogdefaults"
	"cercano/source/server/internal/openmodels"
	"cercano/source/server/pkg/config"
)

// The retained native PTY harness set everyday via legacy open_model, but
// left Chat on premium. Test the real loader and catalog at the guest's 8GiB.
func TestNativeFixtureSelection(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	corrected, err := os.ReadFile("testdata/native-ollama.yaml")
	if err != nil {
		t.Fatal(err)
	}
	const legacy = "ollama_url: http://127.0.0.1:11434\nopen_runtime: ollama\nopen_model: qwen2.5:0.5b\nlocus_mode: open_only\nport: \"50083\"\ncloud_profiles: []\n"
	const catalog = "llama_server:catalog:qwen3-14b-q4_k_m"
	for _, tt := range []struct {
		name, yaml, want string
		invoke           bool
	}{
		{"historical_missing_chat_quality", legacy, catalog, false},
		{"corrected_standard_chat", string(corrected), "qwen2.5:0.5b", true},
		{"explicit_premium_override", legacy + "models:\n  open:\n    overrides:\n      ollama:\n        most_capable: qwen2.5:0.5b\n", "qwen2.5:0.5b", true},
		{"native_catalog_default", "open_runtime: llama_server\nlocus_mode: open_only\n", catalog, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			cs := cfgsvc.New("", c, nil)
			models := openmodels.New(cs, catalogdefaults.ForRuntime, func() uint64 { return 8 << 30 })
			s := &service{cfgSvc: cs, openModels: models, openLLMProvider: &labelProvider{label: "ollama"}}
			t.Logf("runtime=%s everyday=%s chat_quality=%s main=%s", c.OpenRuntime, models.ChatModel(), c.TaskAssignment(config.TaskChat).Quality, s.MainModel(false))
			if got := s.MainModel(false); got != tt.want {
				t.Fatalf("main=%q, want %q", got, tt.want)
			}
			// StreamProcessRequest uses PrimaryModel to seed a worker conversation.
			if got := s.PrimaryModel(); got != tt.want {
				t.Fatalf("conversation seed model=%q, want %q", got, tt.want)
			}
			if !tt.invoke {
				return
			}
			// Provider boundary: same task-bound model consumed by Core.RunTurn, using
			// a local in-memory provider rather than launching Ollama or inference.
			p, cloud, fallback, err := s.Main()
			if err != nil {
				t.Fatal(err)
			}
			if cloud || fallback {
				t.Fatal("unexpected cloud/fallback")
			}
			model, ok := inference.TaskModelFor(p)
			if !ok || model != tt.want {
				t.Fatalf("bound model=%q ok=%v", model, ok)
			}
			result, err := p.Chat(context.Background(), inference.Call{Model: model})
			if err != nil || result.Model != "ollama:"+tt.want {
				t.Fatalf("provider result=%q err=%v", result.Model, err)
			}
		})
	}
}
