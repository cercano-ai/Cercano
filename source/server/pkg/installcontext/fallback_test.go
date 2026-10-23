package installcontext_test

// Fallback wiring test: installcontext.VerifyChild must fall back from the
// completed-only verifier to the selected-candidate verifier ONLY on the
// completed lookup's normal not-found, and the completed path must keep
// refusing candidates. The fixture mirrors verifiedChildFixture but stops at
// the approved selected checkpoint with the current operation restarting.

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

func candidateChildFixture(t *testing.T) childFixture {
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
	enrolled, err := enrollment.Create(ctx, root)
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
	for _, cp := range []state.JournalCheckpoint{state.JournalSwitchIntent, state.JournalSelected} {
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
	rec.State = operation.StateRestarting
	if _, err = store.SaveOperationRecord(ctx, r, rec); err != nil {
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
		locators:      installcontext.Locators{InstallRoot: root, InstallID: enrolled.InstallID},
		record:        enrolled,
		stateRoot:     stateRoot,
		agentExe:      filepath.Join(versionDir, "bin", a),
		clientExe:     filepath.Join(versionDir, "bin", c),
		selectionPath: filepath.Join(store.Directory(), selection.FileName),
	}
}

func TestVerifyChildSelectionIndependentOfOperationGeneration(t *testing.T) {
	ctx := context.Background()
	f := candidateChildFixture(t)

	// Shared startup verification wires the candidate fallback: a selected,
	// restarting operation authorizes its exact image through the same public
	// entry point used for completed installs.
	if got, err := installcontext.VerifyChild(ctx, f.locators, f.stateRoot, f.agentExe, false); err != nil || got != f.record {
		t.Fatalf("candidate fallback not wired: record %+v err %v", got, err)
	}
	if got, err := installcontext.VerifyChild(ctx, f.locators, f.stateRoot, f.clientExe, true); err != nil || got != f.record {
		t.Fatalf("client role candidate fallback: record %+v err %v", got, err)
	}
	// The verifier is not weakened by the fallback.
	if _, err := installcontext.VerifyChild(ctx, f.locators, f.stateRoot, f.agentExe, true); err == nil {
		t.Fatal("agent executable accepted in client role via fallback")
	}
	// Selection is authoritative; an old operation generation cannot gate boot.
	selected := activation.Selection{SchemaVersion: 1, InstallID: f.locators.InstallID, Generation: 2, SelectedVersion: "1.0.0", StagedVersionDir: "v1", VerifiedArtifactSHA256: strings.Repeat("a", 64)}
	raw, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f.selectionPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := installcontext.VerifyChild(ctx, f.locators, f.stateRoot, f.agentExe, false); err != nil {
		t.Fatal("valid selection gated by operation generation", err)
	}
}
