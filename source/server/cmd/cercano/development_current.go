package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	pb "cercano/source/server/pkg/proto"
)

// This is discovery, not session authority. Only the new-session launcher writes
// it, after authenticated readiness and acquisition. Reconnect and worker paths
// keep their immutable selection and never call this function.
func publishCurrentTarget(state string, s launchSelection) error {
	s.Conversation, s.Epoch = "", ""
	f, err := os.CreateTemp(state, ".current-target-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = json.NewEncoder(f).Encode(s); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, filepath.Join(state, "current-target.json")); err != nil {
		return err
	}
	dir, err := os.Open(state)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Missing/invalid discovery cannot authorize retirement. The existing reservation
// remains the sole judge of open sessions and outstanding work; disconnect is
// deliberately not a drain signal. There is no supersession ledger.
func retireIfNotCurrent(ctx context.Context, state string, identity *pb.LifecycleIdentity, startupTarget string, retire func(context.Context) error) {
	var current launchSelection
	if launchRead(filepath.Join(state, "current-target.json"), &current) != nil {
		return
	}
	target := current.Identity
	if target == nil || target.Host == "" || target.Host == identity.Host || (target.Build != "" && target.Build == identity.Build) || target.Host == startupTarget || target.Store != identity.Store || target.Contract != identity.Contract || target.Schema != identity.Schema {
		return
	}
	_ = retire(ctx) // Busy/incomplete/failed cleanup stays owned by existing retirement accounting.
}

func watchCurrentTarget(ctx context.Context, state string, identity *pb.LifecycleIdentity, retire func(context.Context) error) func() {
	// Capture the predecessor before publishing this host's readiness. An idle
	// candidate must not retire merely because its launch has not committed yet.
	// This is invocation-local startup state, not durable supersession history.
	var initial launchSelection
	_ = launchRead(filepath.Join(state, "current-target.json"), &initial)
	startupTarget := initial.Identity.GetHost()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				retireIfNotCurrent(ctx, state, identity, startupTarget, retire)
			}
		}
	}()
	return func() { cancel(); <-done }
}
