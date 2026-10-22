package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/conversation"
)

func TestCandidatePreflightDispatch(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"__internal-update", "--preflight"}, 0},
		{[]string{"__internal-update", "--preflight", "--preflight"}, privateExitUsage},
		{[]string{"__internal-update", "--preflight", "--"}, privateExitUsage},
		{[]string{"__internal-update", "--preflight", "agent"}, privateExitUsage},
		{[]string{"__internal-update", "--preflight", "--internal-install-root", "/not-read"}, privateExitUsage},
		{[]string{"__internal-update", "--preflight", "--install-id", "abc", "--operation-id", "1"}, privateExitUsage},
		{[]string{"__internal-update", "--preflight=true"}, privateExitUsage},
		{[]string{"__internal-update", "--preflight", strings.Repeat("x", 65536)}, privateExitUsage},
	} {
		var out, errOut strings.Builder
		code, claimed := dispatchPrivateUpdate(tc.args, &out, &errOut, sentinelRunner(t))
		if !claimed || code != tc.code {
			t.Fatalf("claimed=%v code=%d want=%d", claimed, code, tc.code)
		}
		if out.Len()+errOut.Len() > 1024 {
			t.Fatal("unbounded report")
		}
		if code == 0 {
			assertPreflightReport(t, out.String())
			if errOut.Len() != 0 {
				t.Fatal(errOut.String())
			}
		} else if out.Len() != 0 {
			t.Fatal("malformed invocation emitted report")
		}
	}
}

func assertPreflightReport(t *testing.T, raw string) {
	t.Helper()
	var report preflightReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	if len(raw) > 1024 || report.Health != "not_checked" || report.Compatibility != "unknown" || report.ExecutionReady || report.Component != "cercano" || report.Protocol != 1 {
		t.Fatalf("unsafe report: %s", raw)
	}
	if len(report.Checks) != 9 {
		t.Fatal(report.Checks)
	}
	for name, status := range report.Checks {
		want := "passed"
		if name == "default_config_validation" {
			want = "skipped_no_pure_defaults_api"
		}
		if status != want {
			t.Fatalf("%s: %s", name, status)
		}
	}
}

type preflightBrokenWriter struct{ short bool }

func (w preflightBrokenWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("fixture write failed")
}
func TestCandidatePreflightOutputFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		code, claimed := dispatchPrivateUpdate([]string{"__internal-update", "--preflight"}, preflightBrokenWriter{short}, io.Discard, sentinelRunner(t))
		if !claimed || code != privateExitExecution {
			t.Fatalf("claimed=%v code=%d", claimed, code)
		}
	}
}

// Reuses the bounded, reaped real-main fixture with isolated HOME/XDG/AppData
// roots and filesystem noncreation assertions. Main's existing wiring test
// proves this dispatch precedes installation verification and startup.
func TestMainSubprocessCandidatePreflight(t *testing.T) {
	out, errOut, code := runMainFixture(t, "__internal-update", "--preflight")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	assertPreflightReport(t, out)
	for _, rest := range [][]string{{"--"}, {"agent"}, {"--preflight"}, {"--internal-install-id", "abc"}} {
		args := append([]string{"__internal-update", "--preflight"}, rest...)
		out, errOut, code = runMainFixture(t, args...)
		if code != privateExitUsage || out != "" || len(errOut) > 1024 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	}
}

type failingPreflightStore struct {
	conversation.Store
	stage string
}

func (s failingPreflightStore) EnsureConversation(context.Context, string, string, string) error {
	if s.stage == "fixture_write" {
		return errors.New("injected")
	}
	return nil
}
func (s failingPreflightStore) Get(context.Context, string) (conversation.Info, error) {
	if s.stage == "fixture_read" {
		return conversation.Info{}, errors.New("injected")
	}
	if s.stage == "fixture_mismatch" {
		return conversation.Info{}, nil
	}
	return conversation.Info{ID: "offline-preflight", ProjectDir: "inert-project", Model: "inert-model"}, nil
}
func (s failingPreflightStore) Close() error {
	if s.stage == "close" {
		return errors.New("injected")
	}
	return nil
}
func TestCandidatePreflightFailures(t *testing.T) {
	for _, stage := range []string{"temp_root", "store_open", "fixture_write", "store_close", "store_reopen", "fixture_read", "fixture_mismatch", "reopened_close", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			parent := t.TempDir()
			sentinel := filepath.Join(parent, "unowned")
			if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			root := ""
			opens := 0
			removed := false
			d := preflightDeps{
				mkdir: func(_, prefix string) (string, error) {
					if stage == "temp_root" {
						return "", errors.New("injected")
					}
					var err error
					root, err = os.MkdirTemp(parent, prefix)
					return root, err
				},
				open: func(path string) (conversation.Store, error) {
					opens++
					if path != filepath.Join(root, "conversations.db") {
						t.Fatal(path)
					}
					if stage == "store_open" || (stage == "store_reopen" && opens == 2) {
						return nil, errors.New("injected")
					}
					fail := stage
					if (stage == "store_close" && opens == 1) || (stage == "reopened_close" && opens == 2) {
						fail = "close"
					}
					return failingPreflightStore{stage: fail}, nil
				},
				remove: func(path string) error {
					if path != root || root == "" {
						t.Fatal("unowned cleanup", path)
					}
					removed = true
					if stage == "cleanup" {
						return errors.New("injected")
					}
					return os.RemoveAll(path)
				},
			}
			var out strings.Builder
			if code := runCandidatePreflightWith(nil, &out, io.Discard, d); code != privateExitExecution {
				t.Fatal(code)
			}
			var report preflightReport
			if err := json.Unmarshal([]byte(out.String()), &report); err != nil {
				t.Fatal(err)
			}
			name := stage
			if name == "fixture_mismatch" {
				name = "fixture_read"
			}
			if report.Checks[name] != "failed" || out.Len() > 1024 || report.ExecutionReady {
				t.Fatal(out.String())
			}
			if removed != (stage != "temp_root") {
				t.Fatal("cleanup not attempted")
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Fatal("unowned file touched", err)
			}
		})
	}
}
func TestCandidatePreflightMalformedDoesNoWork(t *testing.T) {
	if code := runCandidatePreflightWith([]string{"extra"}, io.Discard, io.Discard, preflightDeps{}); code != privateExitUsage {
		t.Fatal(code)
	}
}
func TestBoundedFixtureOutput(t *testing.T) {
	var b boundedFixtureOutput
	for i := 0; i < 20; i++ {
		n, err := b.Write([]byte(strings.Repeat("x", 4096)))
		if n != 4096 || err != nil {
			t.Fatal(n, err)
		}
	}
	if b.Len() != 4096 {
		t.Fatal(b.Len())
	}
}
