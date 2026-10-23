package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/state"
)

// Test the explicit legacy force-control path, not ordinary TUI observation.
// Ordinary observer startup must remain free of force registration and polling.
func TestLegacyForceLocalPollUsesProtectedRequestWithoutNetworkCommand(t *testing.T) {
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(parent, "installed")
	if _, e = privdir.Ensure(root); e != nil {
		t.Fatal(e)
	}
	agent, client := "cercano", "cercano-cli"
	if runtime.GOOS == "windows" {
		agent += ".exe"
		client += ".exe"
	}
	for _, name := range []string{agent, client} {
		if e = os.WriteFile(filepath.Join(root, name), []byte("inert"), 0700); e != nil {
			t.Fatal(e)
		}
	}
	record, e := enrollment.Create(context.Background(), root)
	if e != nil && !errors.Is(e, enrollment.ErrCommitted) {
		t.Fatal(e)
	}
	stateRoot, store, opID := forceLocalFixture(t, record.InstallID)
	s := &Server{events: newEventHub()}
	var stops atomic.Int32
	s.SetProcessForceStopRequester(func(string) { stops.Add(1) })
	ctx := context.Background()
	owner, e := s.newForceLocalOwner()
	if e != nil || owner == nil {
		t.Fatal("create legacy force owner", owner, e)
	}
	if stopped, e := owner.poll(ctx, s, stateRoot, record.InstallID); e != nil || stopped {
		t.Fatal("registration poll", stopped, e)
	}
	session, found, e := store.ReadForceAgent(ctx)
	if e != nil || !found {
		t.Fatal("agent presence missing", e)
	}
	if stops.Load() != 0 {
		t.Fatal("registration interrupted agent")
	}
	now := time.Now()
	intent := state.ForceIntent{OpID: opID, Agent: session, IssuedMS: now.UnixMilli(), ExpiresMS: now.Add(20 * time.Second).UnixMilli()}
	if e = store.SubmitForceIntent(ctx, intent, true, time.Now); e != nil {
		t.Fatal(e)
	}
	if stopped, e := owner.poll(ctx, s, stateRoot, record.InstallID); e != nil || !stopped {
		t.Fatal("authorized force request not consumed", stopped, e)
	}
	if stops.Load() != 1 {
		t.Fatal("wrong stop count", stops.Load())
	}
	got, found, consumed, e := store.ReadForceIntent(context.Background(), opID)
	if e != nil || !found || !consumed || got != intent {
		t.Fatal("stop was not preceded by durable consumption", got, found, consumed, e)
	}
	// Only a callback counter is invoked. No real process is signaled or exited.
}
