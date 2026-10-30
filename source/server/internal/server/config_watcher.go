// Package server — out-of-band config-file change detection.
//
// Most config changes come through UpdateConfig (the /config slash command,
// the cercano_config MCP tool, etc.). But a user or another tool may edit
// config.yaml by hand, bypassing the RPC. This watcher catches those edits,
// diffs against the agent's current in-memory config, and replays only the
// changed fields through UpdateConfig — so file edits and RPC edits share one
// apply path (validation, provider rebuild, broadcast), without writing back.
//
// It watches the containing DIRECTORY, not the file inode, for the same
// reason the permissions watcher does: editors and `os.WriteFile` commonly
// land via temp-file + rename, which makes a single-file watch go deaf after
// the first swap. A directory watch keeps catching create/rename/write on the
// target name.
//
// Reload adopts the authoritative baseline without saving, so it cannot echo writes.
package server

import (
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"

	"github.com/fsnotify/fsnotify"
)

// StartConfigWatcher begins watching configPath for out-of-band edits and
// replays hot-reloadable field changes through UpdateConfig. Returns an error
// only if the watch can't be established; the caller may treat that as
// non-fatal (the RPC path still works). The goroutine stops when ctx is
// cancelled.
//
// Non-hot-reloadable diffs (port, embedding_model, llama_server.*,
// compaction.*) are logged as warnings telling the user a restart is needed.
func (s *Server) StartConfigWatcher(ctx context.Context, configPath string) error {
	if s.cfgSvc.Frozen() {
		return cfgsvc.ErrFrozen
	}
	if configPath == "" {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dir := filepath.Dir(configPath)
	if err := w.Add(dir); err != nil {
		w.Close()
		return err
	}
	target := filepath.Clean(configPath)
	s.configWatchers.Add(1)
	go func() {
		defer s.configWatchers.Done()
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) != target {
					continue
				}
				s.reloadConfigFromDisk(ctx)
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				// Transient watch error — keep going; the RPC path still works.
			}
		}
	}()
	return nil
}

// reloadConfigFromDisk reads the config path, diffs against the current
// in-memory config, and replays the diff through UpdateConfig. Public-method-
// private-helper split keeps the watcher loop tiny and makes the apply path
// testable.
func (s *Server) reloadConfigFromDisk(ctx context.Context) {
	s.runWatchCallback(func() { s.applyConfigFromDisk(ctx) })
}

// runWatchCallback is the entry boundary, not a cancellation pre-check: an
// admitted callback retains its authority and is joined before disposal.
func (s *Server) runWatchCallback(callback func()) {
	if !s.watchAdmission.Enter() {
		return
	}
	defer s.watchAdmission.Leave()
	callback()
}

func (s *Server) applyConfigFromDisk(ctx context.Context) {
	s.configApplyMu.Lock()
	defer s.configApplyMu.Unlock()
	if s.cfgSvc.Frozen() {
		return
	}
	cfgPath := s.cfgSvc.Path()
	newCfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Printf("ConfigWatcher: failed to reload %s: %v\n", cfgPath, err)
		return
	}

	// Snapshot the current in-memory config, then diff. UpdateConfig applies
	// changes through cfgSvc which handles its own locking.
	snap := s.cfgSvc.Get()
	if snap.Snapshot().Revision == newCfg.Snapshot().Revision {
		return
	}

	req := &proto.UpdateConfigRequest{}
	if newCfg.OllamaURL != snap.OllamaURL {
		req.OllamaUrl = newCfg.OllamaURL
	}
	// Detect a change to the active runtime's everyday override. (A runtime
	// switch is handled by the OpenRuntime diff below; it re-resolves the
	// effective model without touching overrides.)
	newEveryday, _ := newCfg.Models.OverrideFor(newCfg.OpenRuntime, config.TierEveryday)
	oldEveryday, _ := snap.Models.OverrideFor(snap.OpenRuntime, config.TierEveryday)
	if newEveryday != oldEveryday {
		req.OpenModel = newEveryday
	}
	if newCfg.OpenRuntime != snap.OpenRuntime {
		req.OpenRuntime = newCfg.OpenRuntime
	}
	if newCfg.LocusMode != snap.LocusMode {
		req.LocusMode = newCfg.LocusMode
	}
	if newCfg.Compaction.ElideToolResults != snap.Compaction.ElideToolResults {
		if newCfg.Compaction.ElideToolResults {
			req.ElideToolResults = "true"
		} else {
			req.ElideToolResults = "false"
		}
	}
	if newCfg.Compaction.LossyToolElision != snap.Compaction.LossyToolElision {
		if newCfg.Compaction.LossyToolElision {
			req.LossyToolElision = "true"
		} else {
			req.LossyToolElision = "false"
		}
	}
	if newCfg.Compaction.Retention.RawRetentionDays != snap.Compaction.Retention.RawRetentionDays {
		req.RawRetentionDays = strconv.Itoa(newCfg.Compaction.Retention.RawRetentionDays)
	}
	if newCfg.Compaction.Retention.CompactedRetentionDays != snap.Compaction.Retention.CompactedRetentionDays {
		req.CompactedRetentionDays = strconv.Itoa(newCfg.Compaction.Retention.CompactedRetentionDays)
	}
	if newCfg.Compaction.Retention.KeepForever != snap.Compaction.Retention.KeepForever {
		if newCfg.Compaction.Retention.KeepForever {
			req.KeepForever = "true"
		} else {
			req.KeepForever = "false"
		}
	}
	if newCfg.ToolLoop.MaxIterations != snap.ToolLoop.MaxIterations {
		req.ToolLoopMaxIterations = strconv.Itoa(newCfg.ToolLoop.MaxIterations)
	}
	if newCfg.Compaction.Enabled != snap.Compaction.Enabled {
		if newCfg.Compaction.Enabled {
			req.CompactionEnabled = "true"
		} else {
			req.CompactionEnabled = "false"
		}
	}
	if newCfg.Compaction.ToolElisionOnly != snap.Compaction.ToolElisionOnly {
		if newCfg.Compaction.ToolElisionOnly {
			req.ToolElisionOnly = "true"
		} else {
			req.ToolElisionOnly = "false"
		}
	}

	// Warn on fields that UpdateConfig doesn't hot-reload. The agent keeps
	// running on the old in-memory values until a restart.
	if newCfg.Port != snap.Port {
		fmt.Printf("ConfigWatcher: port change (%q → %q) requires a restart\n", snap.Port, newCfg.Port)
	}
	newEmbed, _ := newCfg.Models.OverrideFor(newCfg.OpenRuntime, config.TierEmbedding)
	oldEmbed, _ := snap.Models.OverrideFor(snap.OpenRuntime, config.TierEmbedding)
	if newEmbed != oldEmbed {
		fmt.Printf("ConfigWatcher: embedding model change requires a restart\n")
	}

	if reply, err := s.updateConfigLocked(ctx, req, &newCfg); err != nil {
		fmt.Printf("ConfigWatcher: UpdateConfig rejected file change: %v\n", err)
	} else if !reply.Success {
		fmt.Printf("ConfigWatcher: configuration not fully applied: %s\n", reply.Message)
	}
}

// WaitForWatchers drains startup watchers after their contexts have been canceled.
// Call only after all Start*Watcher calls have returned.
func (s *Server) WaitForWatchers() {
	s.configWatchers.Wait()
	s.watchAdmission.Wait()
	if p, ok := s.providerSvc.(interface{ WaitForRebuild() }); ok {
		p.WaitForRebuild()
	}
	if w, ok := s.permBroker.(interface{ WaitForWatchers() }); ok {
		w.WaitForWatchers()
	}
}
