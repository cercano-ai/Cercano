package ui

import (
	"path/filepath"
	"testing"
)

func TestDebugLogsLiveInProfileLogsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "work")
	for _, k := range []string{"CERCANO_TUI_PERF_LOG", "CERCANO_SEARCH_TRACE_LOG", "CERCANO_COLORPROBE_LOG"} {
		t.Setenv(k, "")
	}
	logs := filepath.Join(home, ".config", "cercano", "profiles", "work", "logs")
	rows := []struct{ name, got, want string }{
		{"tui-perf.log", tuiPerfLogPath(), filepath.Join(logs, "tui-perf.log")},
		{"search-perf.jsonl", searchTracePath(), filepath.Join(logs, "search-perf.jsonl")},
		{"colorprobe.log", resolveColorProbeLogPath(), filepath.Join(logs, "colorprobe.log")},
	}
	for _, r := range rows {
		if r.got != r.want {
			t.Errorf("%s: got %q, want %q", r.name, r.got, r.want)
		}
	}
}

func TestDebugLogEnvOverridesStillWin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("CERCANO_PROFILE", "work")
	rows := []struct {
		env string
		fn  func() string
	}{
		{"CERCANO_TUI_PERF_LOG", tuiPerfLogPath},
		{"CERCANO_SEARCH_TRACE_LOG", searchTracePath},
		{"CERCANO_COLORPROBE_LOG", resolveColorProbeLogPath},
	}
	for _, r := range rows {
		want := filepath.Join(dir, r.env+".log")
		t.Setenv(r.env, want)
		if got := r.fn(); got != want {
			t.Errorf("%s set: got %q, want %q", r.env, got, want)
		}
	}
}
