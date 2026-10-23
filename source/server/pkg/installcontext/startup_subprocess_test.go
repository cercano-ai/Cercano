package installcontext_test

// Real-subprocess tests for the startup gate: a member copy of this test
// binary is executed as the ACTUAL enrolled selected member (via
// installcontexttest.Build) with the isolated HOME whose state chain holds
// the completed protected selection, so the child's VerifyStartup exercises
// the real os.Executable() path, the real derived state root, and the real
// enrollment verifier end to end — without ever launching an agent or
// touching a real HOME or state root.
//
// Helper mode (TestMain, env-triggered) is reached only by these tests and
// performs exactly what a real main() does with the locators: parse, strip,
// and VerifyStartup against its own running image. It never starts anything
// else.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"cercano/source/server/pkg/installcontext"
	"cercano/source/server/pkg/installcontexttest"
)

const (
	startupArgsEnv = "CERCANO_INSTALLCONTEXT_TEST_STARTUP_ARGS"
	startupRoleEnv = "CERCANO_INSTALLCONTEXT_TEST_STARTUP_ROLE"
)

// startupSuccess is the JSON helper mode prints after successful startup
// verification, so the parent can assert the identity and the untouched
// remaining argv.
type startupSuccess struct {
	InstallID string   `json:"install_id"`
	Remaining []string `json:"remaining"`
}

// TestMain runs the package tests or, when the fixture env var is set,
// installs the fixture argv and performs the startup gate exactly as the
// mains do (parse + strip + VerifyStartup of the running image). Only this
// package's own tests set the variable; nothing else in the process runs.
func TestMain(m *testing.M) {
	if raw := os.Getenv(startupArgsEnv); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			fmt.Fprintln(os.Stderr, "installcontext-test: fixture args:", err)
			os.Exit(97)
		}
		os.Args = append([]string{"installcontext-test"}, args...)
		locators, found, remaining, err := installcontext.Parse(os.Args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "installcontext-test: locator usage:", err)
			os.Exit(2)
		}
		if !found {
			// Startup unchanged: print the remaining argv verbatim so the
			// parent can prove no unrelated token was consumed, reordered,
			// or filtered.
			out, err := json.Marshal(remaining)
			if err != nil {
				os.Exit(98)
			}
			fmt.Println(string(out))
			return
		}
		client := os.Getenv(startupRoleEnv) == "client"
		record, err := installcontext.VerifyStartup(context.Background(), locators, client)
		if err != nil {
			fmt.Fprintln(os.Stderr, "installcontext-test: refusing startup:", err)
			os.Exit(1)
		}
		out, err := json.Marshal(startupSuccess{InstallID: record.InstallID, Remaining: remaining})
		if err != nil {
			os.Exit(98)
		}
		fmt.Println(string(out))
		return
	}
	os.Exit(m.Run())
}

// runStartup executes one of the fixture's selected members (a copy of this
// test binary) with the fixture's isolated HOME environment and the given
// startup argv, in the given launcher role.
func runStartup(t *testing.T, f *installcontexttest.Install, exe, role string, args ...string) (string, string, int) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	env := append(f.ChildEnv(),
		startupArgsEnv+"="+string(raw),
		startupRoleEnv+"="+role,
	)
	return installcontexttest.Run(t, exe, env)
}

// mustExecutable returns the running test binary path.
func mustExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// TestStartupSubprocessVerifiesActualEnrolledExecutable proves the gate
// accepts the real enrolled selected member in its own role: the executed
// process IS the member, VerifyStartup must accept its own os.Executable(),
// and the unrelated trailing argv must survive verbatim.
func TestStartupSubprocessVerifiesActualEnrolledExecutable(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	for _, tc := range []struct{ name, exe, role string }{
		{"agent role", f.AgentExe, "agent"},
		{"client role", f.ClientExe, "client"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(f.LocatorArgs(), "chat", "--mcp")
			stdout, stderr, code := runStartup(t, f, tc.exe, tc.role, args...)
			if code != 0 {
				t.Fatalf("exit %d stderr %q", code, stderr)
			}
			var got startupSuccess
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
				t.Fatalf("stdout %q: %v", stdout, err)
			}
			if got.InstallID != f.InstallID {
				t.Fatalf("verified install id %q want %q", got.InstallID, f.InstallID)
			}
			if want := []string{"chat", "--mcp"}; len(got.Remaining) != len(want) || strings.Join(got.Remaining, " ") != strings.Join(want, " ") {
				t.Fatalf("remaining argv %q want %q (unrelated args must survive untouched)", got.Remaining, want)
			}
		})
	}
}

// TestStartupSubprocessRefusesWrongInstallID proves a well-formed locator
// pair pointing at another (nonexistent) installation cannot pass: the
// locators are only a pointer, the enrollment/state must match.
func TestStartupSubprocessRefusesWrongInstallID(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	args := []string{
		installcontext.FlagInstallRoot, f.Root,
		installcontext.FlagInstallID, strings.Repeat("b", 32),
		"version",
	}
	stdout, stderr, code := runStartup(t, f, f.AgentExe, "agent", args...)
	if code == 0 {
		t.Fatalf("wrong install id accepted: stdout %q", stdout)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("refused startup must not print a success payload: %q", stdout)
	}
	if !strings.Contains(stderr, "refusing startup") {
		t.Fatalf("stderr %q must explain the refusal", stderr)
	}
}

// TestStartupSubprocessRefusesWrongRole proves the fixed launcher role
// cannot be swapped: the enrolled agent image run in the client role must
// be refused, and vice versa.
func TestStartupSubprocessRefusesWrongRole(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	for _, tc := range []struct{ name, exe, role string }{
		{"agent image in client role", f.AgentExe, "client"},
		{"client image in agent role", f.ClientExe, "agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(f.LocatorArgs(), "version")
			stdout, stderr, code := runStartup(t, f, tc.exe, tc.role, args...)
			if code == 0 {
				t.Fatalf("wrong role accepted: stdout %q", stdout)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("refused startup must not print a success payload: %q", stdout)
			}
			if !strings.Contains(stderr, "refusing startup") {
				t.Fatalf("stderr %q must explain the refusal", stderr)
			}
		})
	}
}

// TestStartupSubprocessRefusesUnsafePermissions proves writable selected images
// cannot pass child admission. Content authenticity belongs to acquisition,
// not a persisted updater-manifest boot dependency.
func TestStartupSubprocessRefusesUnsafePermissions(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	if err := os.Chmod(f.AgentExe, 0777); err != nil {
		t.Fatal(err)
	}
	args := append(f.LocatorArgs(), "version")
	stdout, _, code := runStartup(t, f, f.AgentExe, "agent", args...)
	if code == 0 {
		t.Fatalf("unsafe-permission member accepted: stdout %q", stdout)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("refused startup must not print a success payload: %q", stdout)
	}
}

// TestStartupSubprocessWithoutLocatorsLeavesArgvUntouched proves that with
// no locators nothing is claimed: unrelated argv is returned verbatim,
// including payload tokens that happen to appear after the -- terminator
// (locators there stay positional and are never parsed or stripped).
func TestStartupSubprocessWithoutLocatorsLeavesArgvUntouched(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	cases := [][]string{
		{"agent", "--mcp"},
		{"version", "--"},
		{"--", installcontext.FlagInstallRoot, f.Root, installcontext.FlagInstallID, f.InstallID},
		{"--", installcontext.FlagInstallRoot},
		{"chat", "--", installcontext.FlagInstallRoot, "value-as-operand"},
	}
	for _, args := range cases {
		stdout, _, code := runStartup(t, f, f.AgentExe, "agent", args...)
		if code != 0 {
			t.Fatalf("no-locator startup must proceed: exit %d args %q", code, args)
		}
		var got []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
			t.Fatalf("stdout %q: %v", stdout, err)
		}
		if len(got) != len(args) || strings.Join(got, " ") != strings.Join(args, " ") {
			t.Fatalf("remaining argv %q must equal input %q exactly (order and tokens preserved)", got, args)
		}
	}
}
