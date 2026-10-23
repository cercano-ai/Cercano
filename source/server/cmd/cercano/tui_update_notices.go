package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"cercano/source/server/internal/server"
	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/state"
	"cercano/source/server/pkg/installcontext"
)

// startInstalledTUINotices runs the installed-TUI update observer for the
// agent lifetime. It only reads existing installed-completion metadata; errors
// are diagnostic-only. It never initializes, migrates or repairs updater state,
// registers force control, or polls force-stop/save intents.
//
// With a present verified context the observer resolves the stable launcher
// from the verified record (never this process's own executable) and requires
// the enrollment beside it to match the startup verified record EXACTLY before
// binding — a stale startup record (the installation changed on disk after
// this agent verified) must not silently rebind the watch to the new on-disk
// identity. Once bound, the watch revalidates the expected record on every
// probe and never rebinds to a changed enrollment.
//
// Without a verified context (the agent was not launched as a verified
// installed member — no locators) behavior is unchanged: optional only,
// unenrolled/development/package-managed executables do no updater
// database work. This is not installation enrollment or package ownership
// proof.
func startInstalledTUINotices(verified installcontext.Verified, srv *server.Server) func() {
	if !verified.Present() {
		// Absent verified context: unchanged pre-verification discovery
		// path (no expected record).
		return startBoundInstalledTUINotices("", nil, srv)
	}
	launcher, e := verified.StableLauncher()
	if e != nil {
		return func() {}
	}
	record := verified.Record()
	return startBoundInstalledTUINotices(launcher, &record, srv)
}

// startBoundInstalledTUINotices runs the installed-TUI update observer with
// explicit identity binding. With a non-nil expected record, launcher must be
// the verified stable launcher path; the observer verifies that the discovered
// enrollment matches the expected record exactly BEFORE opening any state
// or emitting notices, and the bound watch loop
// revalidates the same fixed record on every probe — it never re-discovers a
// different installation and emits for it. With a nil expected record the
// unchanged no-context discovery path applies. This prevents cross-install
// identity confusion.
//
// Observation never mutates enrollment or updater control rows.
func startBoundInstalledTUINotices(launcher string, expected *enrollment.Record, srv *server.Server) func() {
	none := func() {}
	ctx := context.Background()

	if expected == nil {
		// No expected enrollment: unchanged pre-verification behavior. A
		// missing adjacent identity is the normal case and needs no
		// warning or loop.
		exe, e := os.Executable()
		if e != nil {
			return none
		}
		if _, e = os.Lstat(filepath.Join(filepath.Dir(exe), enrollment.FileName)); os.IsNotExist(e) {
			return none
		} else if e != nil {
			return none
		}
		home, e := os.UserHomeDir()
		if e != nil {
			return none
		}
		root, e := state.StateRoot(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_STATE_HOME"))
		if e != nil {
			return none
		}
		watchCtx, cancel := context.WithCancel(ctx)
		go srv.WatchEnrolledTUIUpdates(watchCtx, exe, root, func(error) {
			// Keep paths/database content out of diagnostics. A failure is not an agent
			// readiness failure and must not block conversations or reconnection.
			log.Print("[updates] installed TUI notification unavailable; agent operation is unaffected")
		})
		return cancel
	}

	// Expected enrollment: the record beside the verified stable launcher
	// must equal the startup verified record exactly. Any mismatch (or
	// discovery error) refuses the observer outright — before any state access.
	current, e := enrollment.DiscoverStableLauncher(ctx, launcher)
	if e != nil || current != *expected {
		return none
	}

	home, e := os.UserHomeDir()
	if e != nil {
		return none
	}
	root, e := state.StateRoot(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_STATE_HOME"))
	if e != nil {
		return none
	}

	// Bound watch: the loop revalidates the same fixed expected record on
	// every probe before touching state.
	watchCtx, cancel := context.WithCancel(ctx)
	go srv.WatchBoundEnrolledTUIUpdates(watchCtx, launcher, root, expected, func(error) {
		// Keep paths/database content out of diagnostics. A failure is not an agent
		// readiness failure and must not block conversations or reconnection.
		log.Print("[updates] installed TUI notification unavailable; agent operation is unaffected")
	})
	return cancel
}
