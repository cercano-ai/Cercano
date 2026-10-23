package installcontexttest_test

// Self-tests for the shared enrolled-installation fixture: the members it
// builds are real executable copies that pass installcontext.VerifyChild for
// their roles, are refused for the wrong role / wrong install ID / unsafe
// permissions, and the child environment derives the exact state root the
// fixture's store was opened with. These mirror
// pkg/installcontext/verify_test.go but with real executable content.

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"cercano/source/server/internal/updatecoord/state"
	"cercano/source/server/pkg/installcontext"
	"cercano/source/server/pkg/installcontexttest"
)

func TestBuildEnrolledInstallPassesChildVerification(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	ctx := context.Background()

	// The real selected member copies verify for their fixed launcher roles.
	if got, err := installcontext.VerifyChild(ctx, f.Locators(), f.StateRoot, f.AgentExe, false); err != nil || got.InstallID != f.InstallID {
		t.Fatalf("agent member verification: record %+v err %v", got, err)
	}
	if got, err := installcontext.VerifyChild(ctx, f.Locators(), f.StateRoot, f.ClientExe, true); err != nil || got.InstallID != f.InstallID {
		t.Fatalf("client member verification: record %+v err %v", got, err)
	}

	// Wrong role for the executable is refused (the verifier is not weakened
	// by the fixture).
	if _, err := installcontext.VerifyChild(ctx, f.Locators(), f.StateRoot, f.AgentExe, true); err == nil {
		t.Fatal("agent member accepted in client role")
	}
	if _, err := installcontext.VerifyChild(ctx, f.Locators(), f.StateRoot, f.ClientExe, false); err == nil {
		t.Fatal("client member accepted in agent role")
	}

	// A well-formed but wrong install ID must be refused: the locators are
	// only a pointer, the enrollment and state must independently match.
	wrongID := installcontext.Locators{InstallRoot: f.Root, InstallID: strings.Repeat("b", 32)}
	if _, err := installcontext.VerifyChild(ctx, wrongID, f.StateRoot, f.AgentExe, false); err == nil {
		t.Fatal("wrong install id accepted")
	}
}

func TestBuildChildEnvDerivesFixtureStateRoot(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	env := map[string]string{}
	for _, kv := range f.ChildEnv() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	// ChildEnv sets the probes for every platform, so a child of this test
	// binary derives exactly the fixture's state root via
	// installcontext.DefaultStateRoot.
	derived, err := state.StateRoot(runtime.GOOS, env["HOME"], env["LOCALAPPDATA"], env["XDG_STATE_HOME"])
	if err != nil {
		t.Fatal(err)
	}
	if derived != f.StateRoot {
		t.Fatalf("child env derives state root %q, fixture used %q", derived, f.StateRoot)
	}
}

func TestBuildRefusesUnsafePermissionsInProcess(t *testing.T) {
	f := installcontexttest.Build(t, mustExecutable(t), mustExecutable(t))
	if err := os.Chmod(f.AgentExe, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := installcontext.VerifyChild(context.Background(), f.Locators(), f.StateRoot, f.AgentExe, false); err == nil {
		t.Fatal("unsafe-permission member accepted by layout verification")
	}
}

func mustExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}
