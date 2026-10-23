package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
)

// SyncTUIUpdateFromState is shared by trusted install completion and replacement
// agent initialization. The caller supplies an already opened, installation-bound
// store. It does not open/create/migrate state, signal processes or resolve an
// executable. Unknown/in-progress/recovered state emits nothing. Ordinary agent
// startup must not depend on this optional notification succeeding.
//
// Readback brackets the read-only database query. This is an observation, not a
// filesystem transaction or execution authority. A concurrent change refuses;
// a later change is handled by a subsequent sync and local restart verification.
func (s *Server) SyncTUIUpdateFromState(ctx context.Context, store *state.Store) (bool, error) {
	return s.syncTUIUpdateFromState(ctx, store, nil)
}

func (s *Server) syncTUIUpdateFromState(ctx context.Context, store *state.Store, validateBinding func() error) (bool, error) {
	if ctx == nil || store == nil {
		return false, errors.New("installed TUI source is unavailable")
	}
	dir := store.Directory()
	if err := privdir.VerifyExisting(dir); err != nil {
		return false, err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer parent.Close()
	identity, err := parent.Stat()
	if err != nil {
		return false, err
	}
	path := filepath.Join(dir, selection.FileName)
	if err = ctx.Err(); err != nil {
		return false, err
	}
	observed, err := activation.ReadSelection(path)
	if errors.Is(err, activation.ErrSelectionAbsent) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if observed.InstallID != store.InstallID() {
		return false, errors.New("installed TUI selection belongs to another installation")
	}
	id, found, err := store.CompletedSelectionOperation(ctx, state.SelectedVersion{Version: observed.SelectedVersion, Directory: observed.StagedVersionDir, ArtifactSHA256: observed.VerifiedArtifactSHA256, Generation: observed.Generation})
	if err != nil || !found {
		return false, err
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	after, err := activation.ReadSelection(path)
	if err != nil {
		return false, err
	}
	named, err := os.Lstat(dir)
	if err != nil || !named.IsDir() || !os.SameFile(identity, named) || observed != after {
		return false, errors.New("installed TUI selection changed during observation")
	}
	if validateBinding != nil {
		if err := validateBinding(); err != nil {
			return false, err
		}
	}
	// One running agent's event hub may not be rebound to another installation.
	h := s.eventHub()
	h.mu.Lock()
	if h.tuiInstallID != "" && h.tuiInstallID != store.InstallID() {
		h.mu.Unlock()
		return false, errors.New("installed TUI source installation mismatch")
	}
	h.tuiInstallID = store.InstallID()
	h.mu.Unlock()
	if err = s.NotifyTUIUpdateInstalled(observed.SelectedVersion, id); err != nil {
		return false, err
	}
	return true, nil
}
