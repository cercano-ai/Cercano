// Package installcontexttest builds throwaway ENROLLED installations for
// tests of the completed-selection startup gates: installcontext.
// VerifyStartup and the mains (cercano, cercano-cli) that call it.
//
// Build produces, under an isolated temporary HOME:
//
//   - a privileged (privdir-protected) install root with the two inert
//     launcher stubs and an enrollment record (enrollment.Create),
//   - a completed protected activation whose selected members are REAL
//     executable files — byte-for-byte copies of the caller-provided
//     sources (typically the running test binary, so a subprocess of a
//     member copy is the actual enrolled selected image and exercises the
//     real os.Executable() path),
//   - the protected per-user updater state chain and SQLite store for that
//     install ID, with a newer pending operation that must not invalidate
//     the completed selection, mirroring
//     pkg/installcontext/verify_test.go's fixture.
//
// The state chain is created with privdir.Ensure BEFORE the SQLite store is
// opened, matching the protected-chain requirement on Windows. No network,
// no models, no real HOME or live installation is touched, nothing is
// migrated, and no agent is ever launched. It is completed-installation
// verification only; it must never be used to make an unverified candidate
// startup pass.
package installcontexttest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
	"cercano/source/server/pkg/installcontext"
)

// Install is a fully enrolled per-user installation whose completed
// protected activation selects real executable members.
type Install struct {
	// Home is the isolated per-user HOME fixture root.
	Home string
	// Root is the privileged install root (locator --internal-install-root).
	Root string
	// StateRoot is the per-user updater state root a child derives from
	// Home via installcontext.DefaultStateRoot with ChildEnv's probes.
	StateRoot string
	// VersionDir is the privileged versions/<version> directory holding the
	// selected members.
	VersionDir string
	// InstallID is the enrolled installation ID (locator
	// --internal-install-id).
	InstallID string
	// AgentExe and ClientExe are the selected member executables for the
	// fixed launcher roles.
	AgentExe  string
	ClientExe string
	// AgentName and ClientName are the platform member names.
	AgentName  string
	ClientName string
}

// Locators returns the installation locators a trusted launcher would pass.
func (i *Install) Locators() installcontext.Locators {
	return installcontext.Locators{InstallRoot: i.Root, InstallID: i.InstallID}
}

// LocatorArgs returns the exact reserved argv tokens for the installation.
func (i *Install) LocatorArgs() []string {
	return []string{installcontext.FlagInstallRoot, i.Root, installcontext.FlagInstallID, i.InstallID}
}

// ChildEnv returns os.Environ() rewritten so a child process derives Home as
// its per-user root: HOME/USERPROFILE plus LOCALAPPDATA/XDG_STATE_HOME at
// the same subdirectories Build used for the state-root derivation, so
// installcontext.DefaultStateRoot() inside the child resolves to
// Install.StateRoot. All other variables pass through untouched.
func (i *Install) ChildEnv() []string {
	env := []string{}
	for _, kv := range os.Environ() {
		key := kv
		if j := strings.IndexByte(kv, '='); j >= 0 {
			key = kv[:j]
		}
		switch strings.ToUpper(key) {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA",
			"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME":
		default:
			env = append(env, kv)
		}
	}
	return append(env,
		"HOME="+i.Home,
		"USERPROFILE="+i.Home,
		"APPDATA="+filepath.Join(i.Home, "appdata"),
		"LOCALAPPDATA="+filepath.Join(i.Home, "localappdata"),
		"XDG_STATE_HOME="+filepath.Join(i.Home, "xdg-state"),
		"XDG_CONFIG_HOME="+filepath.Join(i.Home, "xdg-config"),
		"XDG_DATA_HOME="+filepath.Join(i.Home, "xdg-data"),
		"XDG_CACHE_HOME="+filepath.Join(i.Home, "xdg-cache"),
	)
}

// Build creates the enrolled installation fixture. agentSource and
// clientSource must be real executable files (usually os.Executable()); the
// selected members are byte-for-byte copies of them.
func Build(t testing.TB, agentSource, clientSource string) *Install {
	return build(t, agentSource, clientSource, false)
}

// BuildCandidate selects an exact restarting candidate without claiming health
// or completion. Used only by disposable startup/launcher fixtures.
func BuildCandidate(t testing.TB, agentSource, clientSource string) *Install {
	return build(t, agentSource, clientSource, true)
}

func build(t testing.TB, agentSource, clientSource string, candidate bool) *Install {
	t.Helper()
	ctx := context.Background()

	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "installed")
	if _, err = privdir.Ensure(root); err != nil {
		t.Fatal(err)
	}
	a, c := "cercano", "cercano-cli"
	if runtime.GOOS == "windows" {
		a, c = "cercano.exe", "cercano-cli.exe"
	}
	for _, n := range []string{a, c} {
		if err = os.WriteFile(filepath.Join(root, n), []byte("inert launcher fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var enrolled enrollment.Record
	enrolled, err = enrollment.Create(ctx, root)
	if err != nil && !(runtime.GOOS == "windows" && errors.Is(err, enrollment.ErrCommitted)) {
		t.Fatal(err)
	}
	if enrolled, err = enrollment.Read(ctx, root); err != nil {
		t.Fatal(err)
	}

	// Derive the state root exactly the way the child will (home plus the
	// platform probes ChildEnv sets), then build the protected chain with
	// privdir.Ensure BEFORE opening the SQLite store — required on Windows.
	// The probe subdirectories are NOT pre-created: StateRoot is pure, and
	// on Linux (xdg-state) and Windows (localappdata) the state root IS one
	// of them. Pre-creating it permissively would make privdir.Ensure
	// refuse the existing directory (unix 0755; Windows inherited DACL),
	// the CI 39162207104 failure. Ensure provisions the state root itself
	// with the approved private policy.
	localappdata := filepath.Join(home, "localappdata")
	xdgState := filepath.Join(home, "xdg-state")
	stateRoot, err := state.StateRoot(runtime.GOOS, home, localappdata, xdgState)
	if err != nil {
		t.Fatal(err)
	}
	// privdir.Ensure does not create intermediate parents: the state root's
	// parent chain (e.g. $HOME/Library on darwin) must exist, then the root
	// itself and every link under it are ensured with privdir.
	if err = os.MkdirAll(filepath.Dir(stateRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err = privdir.Ensure(stateRoot); err != nil {
		t.Fatal(err)
	}
	org := "cercano"
	if runtime.GOOS == "windows" {
		org = "Cercano"
	}
	dir := stateRoot
	for _, part := range []string{org, "updater", enrolled.InstallID} {
		dir = filepath.Join(dir, part)
		if _, err = privdir.Ensure(dir); err != nil {
			t.Fatal(err)
		}
	}
	store, err := state.Open(stateRoot, enrolled.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := state.NewAdapter(store).Start(ctx, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	versions := filepath.Join(root, "versions")
	if _, err = privdir.Ensure(versions); err != nil {
		t.Fatal(err)
	}
	versionDir := filepath.Join(versions, "v1")
	if _, err = privdir.Ensure(versionDir); err != nil {
		t.Fatal(err)
	}
	if _, err = privdir.Ensure(filepath.Join(versionDir, "bin")); err != nil {
		t.Fatal(err)
	}
	members := []state.ManifestMember{}
	for _, src := range []struct{ source, name string }{{agentSource, a}, {clientSource, c}} {
		data, err := os.ReadFile(src.source)
		if err != nil {
			t.Fatalf("reading member source %s: %v", src.source, err)
		}
		dst := filepath.Join(versionDir, "bin", src.name)
		if err = os.WriteFile(dst, data, 0o700); err != nil {
			t.Fatal(err)
		}
		if err = os.Chmod(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		members = append(members, state.ManifestMember{
			Path:   "bin/" + src.name,
			Length: int64(len(data)),
			SHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	}
	manifest := state.ActivationManifest{
		SchemaVersion:          state.ActivationManifestSchemaVersion,
		InstallID:              enrolled.InstallID,
		OpID:                   op.ID,
		TargetVersion:          "1.0.0",
		StagedVersionDir:       "v1",
		VerifiedArtifactSHA256: strings.Repeat("a", 64),
		Members:                members,
	}
	if _, err = store.SaveActivationManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	journal := state.ActivationJournal{
		SchemaVersion:          state.ActivationJournalSchemaVersion,
		InstallID:              enrolled.InstallID,
		OpID:                   op.ID,
		TargetVersion:          manifest.TargetVersion,
		StagedVersionDir:       manifest.StagedVersionDir,
		VerifiedArtifactSHA256: manifest.VerifiedArtifactSHA256,
		Checkpoint:             state.JournalPrepared,
	}
	rev, err := store.SaveActivationJournal(ctx, 0, journal)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := []state.JournalCheckpoint{state.JournalSwitchIntent, state.JournalSelected}
	if !candidate {
		checkpoints = append(checkpoints, state.JournalHealthVerified, state.JournalComplete)
	}
	for _, cp := range checkpoints {
		journal.Checkpoint = cp
		rev, err = store.SaveActivationJournal(ctx, rev, journal)
		if err != nil {
			t.Fatal(err)
		}
	}
	rec, r, err := store.LoadOperationRecord(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec.State = operation.StateComplete
	rec.HealthVerified = true
	if candidate {
		rec.State = operation.StateRestarting
		rec.HealthVerified = false
	}
	if _, err = store.SaveOperationRecord(ctx, r, rec); err != nil {
		t.Fatal(err)
	}
	// A newer pending operation must not invalidate the completed selection.
	if !candidate {
		if _, err = state.NewAdapter(store).Start(ctx, "2.0.0"); err != nil {
			t.Fatal(err)
		}
	}
	selected := activation.Selection{
		SchemaVersion:          1,
		InstallID:              enrolled.InstallID,
		Generation:             1,
		SelectedVersion:        "1.0.0",
		StagedVersionDir:       "v1",
		VerifiedArtifactSHA256: manifest.VerifiedArtifactSHA256,
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(store.Directory(), selection.FileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	// Close the store so the child's read-only verification opens a
	// quiescent database; the fixture state persists on disk.
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	return &Install{
		Home:       home,
		Root:       root,
		StateRoot:  stateRoot,
		VersionDir: versionDir,
		InstallID:  enrolled.InstallID,
		AgentExe:   filepath.Join(versionDir, "bin", a),
		ClientExe:  filepath.Join(versionDir, "bin", c),
		AgentName:  a,
		ClientName: c,
	}
}

// TamperSameSize flips one byte of the file at path, preserving its length
// and mode: a same-size content tamper that a length check alone would
// accept. On darwin this also invalidates the copy's ad-hoc code signature,
// so the OS may kill the process at exec before Go code runs — use Run and
// accept a signal death (-1) as a refusal.
func TamperSameSize(t testing.TB, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("tamper target %s is empty", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 0x5a
	if err = os.WriteFile(path, data, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != info.Size() {
		t.Fatalf("tamper changed size: %d -> %d", info.Size(), after.Size())
	}
}

// Tree returns the sorted set of all paths under root (relative to root),
// for asserting a subprocess created nothing new under an isolated fixture.
func Tree(t testing.TB, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// NoNewPaths fails the test when after contains any path absent from
// before, i.e. the subprocess created new entries under the fixture.
func NoNewPaths(t testing.TB, before, after []string) {
	t.Helper()
	have := make(map[string]bool, len(before))
	for _, p := range before {
		have[p] = true
	}
	for _, p := range after {
		if !have[p] {
			t.Errorf("subprocess created new path under fixture: %s", p)
		}
	}
}

// Run executes exe with env and the given argv, bounded by a deadline and
// reaped via Wait, returning stdout, stderr, and the exit code. The code is
// -1 when the process was killed by a signal (for example darwin
// code-signing killing a same-size-tampered binary before main runs); a
// refusal must never be a clean 0.
func Run(t testing.TB, exe string, env []string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("subprocess %s %v was not reaped in time (deadline)", exe, args)
	}
	code := 0
	var exitErr *exec.ExitError
	if runErr != nil && errors.As(runErr, &exitErr) {
		code = exitErr.ExitCode()
	} else if runErr != nil {
		t.Fatalf("running subprocess %s %v: %v", exe, args, runErr)
	}
	return stdout.String(), stderr.String(), code
}
