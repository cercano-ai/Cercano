package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/state"
)

func notificationEnrollment(t *testing.T) (string, enrollment.Record) {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "installed")
	if _, err = privdir.Ensure(root); err != nil {
		t.Fatal(err)
	}
	agent, cli := "cercano", "cercano-cli"
	if runtime.GOOS == "windows" {
		agent += ".exe"
		cli += ".exe"
	}
	for _, name := range []string{agent, cli} {
		if err = os.WriteFile(filepath.Join(root, name), []byte("inert"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	record, err := enrollment.Create(context.Background(), root)
	if err != nil && !errors.Is(err, enrollment.ErrCommitted) {
		t.Fatal(err)
	}
	return filepath.Join(root, agent), record
}

func observeForBootTest(t *testing.T, s *Server, launcher, root string, record enrollment.Record) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	s.watchBoundEnrolledTUIUpdates(ctx, launcher, root, &record, time.Millisecond, func(error) {})
}

func TestTUIUpdateWatchBootDoesNotRegisterOrRepair(t *testing.T) {
	for _, kind := range []string{"absent", "current", "corrupt", "old-schema"} {
		t.Run(kind, func(t *testing.T) {
			launcher, record := notificationEnrollment(t)
			root := t.TempDir()
			var path string
			var before []byte
			if kind != "absent" {
				var store *state.Store
				root, store, _, _ = installedNoticeFixture(t, record.InstallID)
				path = filepath.Join(store.Directory(), "state.db")
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				var err error
				before, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "corrupt" {
					before = []byte("not a database")
				}
				if kind == "old-schema" {
					binary.BigEndian.PutUint32(before[60:64], 1)
				}
				if err = os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, _ := newServerWithStore(t)
			s.SetProcessForceStopRequester(func(string) { t.Error("notification observer requested stop") })
			observeForBootTest(t, s, launcher, root, record)
			if s.localForceControl != nil {
				t.Error("notification observer initialized force owner")
			}
			if kind == "absent" {
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 0 {
					t.Fatal("observer initialized state", entries, err)
				}
			} else {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("observer modified/repaired database", err)
				}
				if kind == "current" {
					store, err := state.OpenExistingReadOnly(context.Background(), root, record.InstallID)
					if err != nil {
						t.Fatal(err)
					}
					defer store.Close()
					if _, found, err := store.ReadForceAgent(context.Background()); err != nil || found {
						t.Fatal("observer registered force agent", found, err)
					}
				}
			}
		})
	}
}

func TestTUIUpdateWatchBootDoesNotConsumeForceIntent(t *testing.T) {
	launcher, record := notificationEnrollment(t)
	root, store, op := forceLocalFixture(t, record.InstallID)
	s, _ := newServerWithStore(t)
	s.SetProcessForceStopRequester(func(string) { t.Error("observer consumed force intent") })
	// Explicit legacy API remains available, but observation must not drive it.
	owner, err := s.newForceLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = owner.poll(ctx, s, root, record.InstallID); err != nil {
		t.Fatal(err)
	}
	intent := state.ForceIntent{OpID: op, Agent: owner.agent, IssuedMS: time.Now().UnixMilli(), ExpiresMS: time.Now().Add(20 * time.Second).UnixMilli()}
	if err = store.SubmitForceIntent(ctx, intent, true, time.Now); err != nil {
		t.Fatal(err)
	}
	observeForBootTest(t, s, launcher, root, record)
	if _, found, consumed, err := store.ReadForceIntent(ctx, op); err != nil || !found || consumed {
		t.Fatal("observer changed force intent", found, consumed, err)
	}
}
