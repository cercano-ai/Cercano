// Package server — out-of-band config-file change detection.
//
// Most config changes come through UpdateConfig (the /config slash command,
// the cercano_config MCP tool, etc.). But a user or another tool may edit
// config.yaml by hand, bypassing the RPC. This watcher catches those edits,
// diffs against the agent's current in-memory config, and replays only the
// changed fields through UpdateConfig — so file edits and RPC edits share one
// apply path (validation, provider rebuild, persistence, broadcast).
//
// It watches the containing DIRECTORY, not the file inode, for the same
// reason the permissions watcher does: editors and `os.WriteFile` commonly
// land via temp-file + rename, which makes a single-file watch go deaf after
// the first swap. A directory watch keeps catching create/rename/write on the
// target name.
//
// Echo loop note: UpdateConfig re-saves the file after applying changes,
// which fires another fsnotify event. On the next pass the diff against
// currentConfig is empty, so UpdateConfig short-circuits at the
// "no changes requested" branch before re-saving — no infinite loop.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
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
	// machine.yaml holds the runtime sections shared by every profile; keep
	// this agent's in-memory copy current when another profile or the user
	// edits it. Failing to watch it is non-fatal.
	machinePath := filepath.Clean(paths.MachineConfigPath())
	watchMachine := false
	if mdir := filepath.Dir(machinePath); mdir == filepath.Clean(dir) {
		watchMachine = true
	} else if err := os.MkdirAll(mdir, 0o700); err == nil && w.Add(mdir) == nil {
		watchMachine = true
	}
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				name := filepath.Clean(ev.Name)
				if watchMachine && name == machinePath {
					s.reloadMachineFromDisk()
				}
				if name != target {
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

// reloadMachineFromDisk refreshes the machine-wide runtime sections (and the
// baseline Save diffs against) from machine.yaml. It restarts nothing; it only
// keeps the in-memory copy current so the agent reports current values and its
// next Save does not treat them as its own edits. A runtime edit made since
// the last save keeps its value: the event may come from this agent's own
// earlier save, and the pending Persist still has to write the edit.
func (s *Server) reloadMachineFromDisk() {
	mc, err := config.LoadMachine()
	if err != nil {
		fmt.Printf("ConfigWatcher: failed to reload machine config: %v\n", err)
		return
	}
	if err := s.cfgSvc.Mutate(func(c *config.Config) { c.RefreshMachineSections(mc) }); err != nil {
		fmt.Printf("ConfigWatcher: machine config rejected: %v\n", err)
	}
}

// reloadConfigFromDisk reads the config path, diffs against the current
// in-memory config, and replays the diff through UpdateConfig. Public-method-
// private-helper split keeps the watcher loop tiny and makes the apply path
// testable.
func (s *Server) reloadConfigFromDisk(ctx context.Context) {
	cfgPath := s.cfgSvc.Path()
	newCfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Printf("ConfigWatcher: failed to reload %s: %v\n", cfgPath, err)
		return
	}

	// Snapshot the current in-memory config, then diff. UpdateConfig applies
	// changes through cfgSvc which handles its own locking.
	snap := s.cfgSvc.Get()

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
	if newCfg.CloudProvider != snap.CloudProvider {
		req.CloudProvider = newCfg.CloudProvider
	}
	if newCfg.CloudModel != snap.CloudModel {
		req.CloudModel = newCfg.CloudModel
	}
	if newCfg.CloudAPIKey != snap.CloudAPIKey {
		req.CloudApiKey = newCfg.CloudAPIKey
	}
	if newCfg.CloudBaseURL != snap.CloudBaseURL {
		req.CloudBaseUrl = newCfg.CloudBaseURL
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

	if _, err := s.UpdateConfig(ctx, req); err != nil {
		fmt.Printf("ConfigWatcher: UpdateConfig rejected file change: %v\n", err)
	}
}
