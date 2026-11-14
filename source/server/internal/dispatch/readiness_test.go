package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cercano/source/server/pkg/config"
)

func llamaCfg(enabled bool, defaultModel string) config.Config {
	return config.Config{
		OpenRuntime: "llama_server",
		LlamaServer: config.LlamaServerConfig{Enabled: enabled, DefaultModel: defaultModel},
	}
}

func TestOpenModelReady_PresentGGUF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(path, []byte("GGUF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !OpenModelReady(llamaCfg(true, path)) {
		t.Error("model file present on disk should be ready")
	}
}

func TestOpenModelReady_MissingGGUF(t *testing.T) {
	dir := t.TempDir()
	// Points at a path that doesn't exist — the "still downloading" state.
	if OpenModelReady(llamaCfg(true, filepath.Join(dir, "not-downloaded-yet.gguf"))) {
		t.Error("a not-yet-present model must register absent so routing crosses to cloud")
	}
}

func TestOpenModelReady_EmptyOrDisabled(t *testing.T) {
	if OpenModelReady(llamaCfg(true, "")) {
		t.Error("empty default model is not ready")
	}
	if OpenModelReady(llamaCfg(false, "/anything")) {
		t.Error("disabled llama-server is not ready")
	}
}

func TestOpenModelReady_NonLlamaRuntimeAlwaysReady(t *testing.T) {
	// Ollama and other runtimes manage their own model presence.
	c := config.Config{OpenRuntime: "ollama"}
	if !OpenModelReady(c) {
		t.Error("non-llama-server runtime should be treated as ready")
	}
}

func TestLooksLikePath(t *testing.T) {
	for _, path := range []string{"/models/model.gguf", "~/models/model.gguf", "./model.gguf", "../model.gguf", filepath.Join(t.TempDir(), "model.gguf")} {
		if !looksLikePath(path) {
			t.Errorf("looksLikePath(%q) = false", path)
		}
	}
	if runtime.GOOS == "windows" {
		for _, path := range []string{`C:\models\model.gguf`, "C:/models/model.gguf", `\\server\share\model.gguf`} {
			if !looksLikePath(path) {
				t.Errorf("looksLikePath(%q) = false", path)
			}
		}
	}
}

func TestOpenModelReady_MissingWindowsGGUF(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows absolute path semantics")
	}
	path := filepath.Join(t.TempDir(), "not-downloaded-yet.gguf")
	for _, path := range []string{path, filepath.ToSlash(path)} {
		t.Run(path, func(t *testing.T) {
			if !filepath.IsAbs(path) {
				t.Fatalf("fixture is not native absolute: %q", path)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("fixture must be absent: %v", err)
			}
			if !looksLikePath(path) {
				t.Fatalf("missing Windows path classified as model ID: %q", path)
			}
			if OpenModelReady(llamaCfg(true, path)) {
				t.Error("absent Windows path must not be ready")
			}
		})
	}
}

func TestOpenModelReady_MissingPOSIXGGUF(t *testing.T) {
	path := "/cercano-readiness-" + filepath.Base(t.TempDir()) + "/not-downloaded-yet.gguf"
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("fixture must be absent: %v", err)
	}
	if OpenModelReady(llamaCfg(true, path)) {
		t.Error("absent POSIX path must not be ready")
	}
}

func TestOpenModelReady_ModelIDs(t *testing.T) {
	for _, model := range []string{"llama_server:catalog:model-a", "llama3:latest", "org/model", "org/model.gguf", "model-a.gguf"} {
		t.Run(model, func(t *testing.T) {
			if looksLikePath(model) {
				t.Errorf("model ID classified as path: %q", model)
			}
			if !OpenModelReady(llamaCfg(true, model)) {
				t.Error("opaque default model ID should remain ready")
			}
			if !OpenModelReadyFor(llamaCfg(true, ""), model) {
				t.Error("effective model ID should remain ready independently of default")
			}
			if OpenModelReadyFor(llamaCfg(true, model), "") {
				t.Error("empty effective model must not fall back to default")
			}
		})
	}
}
