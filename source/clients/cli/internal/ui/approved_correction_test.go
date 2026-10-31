package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityDoesNotRestrictOrdinaryUI(t *testing.T) {
	isolateOrdinaryUI(t)
	for _, line := range []string{"/help", "/config", "/settings", "/theme", "/runtime"} {
		m := New(devResumeClient(t, filepath.Join(t.TempDir(), "metadata.json")), false).WithDevHost(DevHostSelection{Conversation: "old"}, "test")
		next, _ := m.runSlash(line)
		if next.(Model).errMsg != "" {
			t.Errorf("%s: %s", line, next.(Model).errMsg)
		}
	}
}

func TestPinnedShutdownPathsRefuseBeforeRPC(t *testing.T) {
	isolateOrdinaryUI(t)
	m := New(devResumeClient(t, filepath.Join(t.TempDir(), "metadata.json")), false).WithDevHost(DevHostSelection{Conversation: "old"}, "test")
	for _, action := range []string{"restart", "runtime", "model"} {
		_, cmd := m.runSlash("/restart-agent test")
		if action == "runtime" {
			cmd = dispatchOpenRuntimeSwitch(m.shutdownOrigin(), "llama_server")
		}
		if action == "model" {
			cmd = dispatchOpenModelPick(m.shutdownOrigin(), "llama_server", "/fake/model")
		}
		result := cmd().(shutdownDecisionMsg)
		if !strings.Contains(result.text, "generation-aware restart") {
			t.Errorf("%s: %s", action, result.text)
		}
	}
}

func addUnrelatedRecord(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "hosts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hosts", "stale.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
}
