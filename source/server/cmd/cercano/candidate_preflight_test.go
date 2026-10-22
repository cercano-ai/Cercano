package main

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
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
		if out.Len()+errOut.Len() > 512 {
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
	var report map[string]any
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	if raw != candidatePreflightReport || len(raw) > 512 || len(report) != 6 || report["health"] != "not_checked" || report["persistent_state_compatibility"] != "unknown" || report["execution_ready"] != false {
		t.Fatalf("unsafe report: %s", raw)
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
		if code != privateExitUsage || out != "" || len(errOut) > 512 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	}
}
