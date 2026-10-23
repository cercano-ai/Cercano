package installcontext_test

// Boot validation tests use historical completed fixtures, but boot authority
// is exclusively protected enrollment, selection and the selected layout.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
	"cercano/source/server/pkg/installcontext"
)

// childFixture is an explicitly enrolled per-user installation with a
// protected completed activation selecting one verified version containing
// both launcher-role member images.
type childFixture struct {
	locators  installcontext.Locators
	record    enrollment.Record
	stateRoot string
	agentExe  string
	clientExe string
	// selectionPath is set by candidateChildFixture only.
	selectionPath string
}

func verifiedChildFixture(t *testing.T) childFixture {
	t.Helper()
	ctx := context.Background()

	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "installed")
	if _, err = privdir.Ensure(root); err != nil {
		t.Fatal(err)
	}
	a, c := "cercano", "cercano-cli"
	if runtime.GOOS == "windows" {
		a, c = "cercano.exe", "cercano-cli.exe"
	}
	for _, n := range []string{a, c} {
		if err = os.WriteFile(filepath.Join(root, n), []byte("inert launcher fixture"), 0700); err != nil {
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

	stateRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
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
	t.Cleanup(func() { store.Close() })
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
	data := []byte("inert verified child image")
	members := []state.ManifestMember{}
	for _, name := range []string{a, c} {
		if err = os.WriteFile(filepath.Join(versionDir, "bin", name), data, 0700); err != nil {
			t.Fatal(err)
		}
		members = append(members, state.ManifestMember{Path: "bin/" + name, Length: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	manifest := state.ActivationManifest{SchemaVersion: state.ActivationManifestSchemaVersion, InstallID: enrolled.InstallID, OpID: op.ID, TargetVersion: "1.0.0", StagedVersionDir: "v1", VerifiedArtifactSHA256: strings.Repeat("a", 64), Members: members}
	if _, err = store.SaveActivationManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	journal := state.ActivationJournal{SchemaVersion: state.ActivationJournalSchemaVersion, InstallID: enrolled.InstallID, OpID: op.ID, TargetVersion: manifest.TargetVersion, StagedVersionDir: manifest.StagedVersionDir, VerifiedArtifactSHA256: manifest.VerifiedArtifactSHA256, Checkpoint: state.JournalPrepared}
	rev, err := store.SaveActivationJournal(ctx, 0, journal)
	if err != nil {
		t.Fatal(err)
	}
	for _, cp := range []state.JournalCheckpoint{state.JournalSwitchIntent, state.JournalSelected, state.JournalHealthVerified, state.JournalComplete} {
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
	if _, err = store.SaveOperationRecord(ctx, r, rec); err != nil {
		t.Fatal(err)
	}
	// A newer pending operation must not invalidate the completed selection.
	if _, err = state.NewAdapter(store).Start(ctx, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	selected := activation.Selection{SchemaVersion: 1, InstallID: enrolled.InstallID, Generation: 1, SelectedVersion: "1.0.0", StagedVersionDir: "v1", VerifiedArtifactSHA256: manifest.VerifiedArtifactSHA256}
	raw, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(store.Directory(), selection.FileName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return childFixture{
		locators:  installcontext.Locators{InstallRoot: root, InstallID: enrolled.InstallID},
		record:    enrolled,
		stateRoot: stateRoot,
		agentExe:  filepath.Join(versionDir, "bin", a),
		clientExe: filepath.Join(versionDir, "bin", c),
	}
}

func TestVerifyChildVerifiesRolesAgainstProtectedSelection(t *testing.T) {
	ctx := context.Background()
	f := verifiedChildFixture(t)

	// Launcher-built locators round-trip into a successful verification for
	// each fixed launcher role.
	args, err := installcontext.LocatorArgs(f.locators.InstallRoot, f.locators.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	locators, found, remaining, err := installcontext.Parse(args)
	if err != nil || !found || len(remaining) != 0 {
		t.Fatalf("locator round trip: found=%t remaining=%v err=%v", found, remaining, err)
	}
	if got, err := installcontext.VerifyChild(ctx, locators, f.stateRoot, f.agentExe, false); err != nil || got != f.record {
		t.Fatalf("agent role verification: record %+v err %v", got, err)
	}
	if got, err := installcontext.VerifyChild(ctx, locators, f.stateRoot, f.clientExe, true); err != nil || got != f.record {
		t.Fatalf("client role verification: record %+v err %v", got, err)
	}

	// Wrong role for the executable must be refused (verifier not weakened).
	if _, err = installcontext.VerifyChild(ctx, locators, f.stateRoot, f.agentExe, true); err == nil {
		t.Fatal("agent executable accepted in client role")
	}
	if _, err = installcontext.VerifyChild(ctx, locators, f.stateRoot, f.clientExe, false); err == nil {
		t.Fatal("client executable accepted in agent role")
	}
	// An unverified image must be refused.
	if _, err = installcontext.VerifyChild(ctx, locators, f.stateRoot, filepath.Join(f.locators.InstallRoot, "cercano"), false); err == nil {
		t.Fatal("unverified launcher image accepted")
	}
	// A missing state root must be refused without provisioning anything.
	missing := filepath.Join(f.stateRoot, "missing")
	if _, err = installcontext.VerifyChild(ctx, locators, missing, f.agentExe, false); err == nil {
		t.Fatal("missing state root accepted")
	}
	if _, e := os.Stat(missing); !os.IsNotExist(e) {
		t.Fatal("missing state root initialized", e)
	}
	// Absent locators must be refused rather than guessed.
	if _, err = installcontext.VerifyChild(ctx, installcontext.Locators{}, f.stateRoot, f.agentExe, false); err == nil {
		t.Fatal("absent locators accepted")
	}
	if _, err = installcontext.VerifyChild(ctx, installcontext.Locators{InstallRoot: f.locators.InstallRoot}, f.stateRoot, f.agentExe, false); err == nil {
		t.Fatal("partial locators accepted")
	}
}

func TestVerifyStartupRefusesUnverifiedRunningImage(t *testing.T) {
	f := verifiedChildFixture(t)
	// The running test binary is NOT the enrolled fixture image, so the
	// startup gate must refuse even for a valid, completed installation.
	if _, err := installcontext.VerifyStartup(context.Background(), f.locators, false); err == nil {
		t.Fatal("startup gate accepted a running image that is not the verified child")
	}
	if _, err := installcontext.VerifyStartup(context.Background(), f.locators, true); err == nil {
		t.Fatal("startup gate accepted a running image in client role")
	}
	if _, err := installcontext.VerifyStartup(context.Background(), installcontext.Locators{}, false); err == nil {
		t.Fatal("startup gate accepted absent locators")
	}
}

func TestBootRefusesUnsafeSelectedPathsWithoutDatabase(t *testing.T) {
	for _, kind := range []string{"missing", "symlink-image", "symlink-directory", "writable-directory", "traversal", "wrong-install"} {
		t.Run(kind, func(t *testing.T) {
			f := verifiedChildFixture(t)
			dir, err := installcontext.BootDirectory(f.stateRoot, f.record.InstallID)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(filepath.Join(dir, "state.db")); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing":
				err = os.Remove(f.agentExe)
			case "symlink-image":
				err = os.Remove(f.agentExe)
				if err == nil {
					err = os.Symlink(f.clientExe, f.agentExe)
				}
			case "symlink-directory":
				bin := filepath.Dir(f.agentExe)
				err = os.Rename(bin, bin+"-moved")
				if err == nil {
					err = os.Symlink(bin+"-moved", bin)
				}
			case "writable-directory":
				err = os.Chmod(filepath.Dir(f.agentExe), 0777)
			default:
				path := filepath.Join(dir, selection.FileName)
				var selected activation.Selection
				selected, err = activation.ReadSelection(path)
				if err == nil {
					if kind == "traversal" {
						selected.StagedVersionDir = "../v1"
					} else {
						selected.InstallID = strings.Repeat("a", 32)
					}
					var raw []byte
					raw, err = json.Marshal(selected)
					if err == nil {
						err = os.WriteFile(path, raw, 0600)
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, _, lock, err := installcontext.PrepareLaunch(context.Background(), f.locators, f.stateRoot, false); err == nil {
				lock.Close()
				t.Fatal("unsafe stable target accepted")
			}
			if _, err := installcontext.VerifyChild(context.Background(), f.locators, f.stateRoot, f.agentExe, false); err == nil {
				t.Fatal("unsafe child accepted")
			}
		})
	}
}
