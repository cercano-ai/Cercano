package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCercanoProfileFilesLiveInProfileDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "work")
	profileDir := filepath.Join(home, ".config", "cercano", "profiles", "work")

	rows := []struct{ name, got, want string }{
		{"catalog-cache.json", profileFile("catalog-cache.json"), filepath.Join(profileDir, "catalog-cache.json")},
		{"permissions.yaml", profileFile("permissions.yaml"), filepath.Join(profileDir, "permissions.yaml")},
		{"telemetry.db", profileFile("telemetry.db"), filepath.Join(profileDir, "telemetry.db")},
		{"crash.log", profileFile("crash.log"), filepath.Join(profileDir, "crash.log")},
		{"dispatch.log", dispatchLogPath(), filepath.Join(profileDir, "logs", "dispatch.log")},
		// update_check.json is machine-wide: one release check per machine.
		{"update_check.json dir", updateCacheDir(), filepath.Join(home, ".config", "cercano")},
	}
	for _, r := range rows {
		if r.got != r.want {
			t.Errorf("%s: got %q, want %q", r.name, r.got, r.want)
		}
	}
}

func TestStartupCheckRejectsMissingHomeAndBadProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("CERCANO_PROFILE", "")
	if err := checkStartupEnvironment(); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Errorf("no HOME: err = %v, want home directory error", err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "Bad Name")
	if err := checkStartupEnvironment(); err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Errorf("bad profile: err = %v, want invalid profile name", err)
	}
	t.Setenv("CERCANO_PROFILE", "work")
	if err := checkStartupEnvironment(); err != nil {
		t.Errorf("valid environment: err = %v", err)
	}
}

func TestStartupCheckSkipsOnlyInformationalCommands(t *testing.T) {
	for _, args := range [][]string{{"cercano", "version"}, {"cercano", "--version"}, {"cercano", "-version"}, {"cercano", "protocols", "sync"}, {"cercano", "skills", "sync"}} {
		if needsStartupCheck(args) {
			t.Errorf("needsStartupCheck(%v) = true, want false", args)
		}
	}
	for _, args := range [][]string{{"cercano"}, {"cercano", "--mcp"}, {"cercano", "agent"}, {"cercano", "model-host"}, {"cercano", "worker"}, {"cercano", "run", "hi"}, {"cercano", "setup"}, {"cercano", "reset"}, {"cercano", "enterprise", "status"}, {"cercano", "restart-after-upgrade"}, {"cercano", "stats"}, {"cercano", "logs"}, {"cercano", "export"}} {
		if !needsStartupCheck(args) {
			t.Errorf("needsStartupCheck(%v) = false, want true", args)
		}
	}
}
