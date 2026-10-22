package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"cercano/source/server/internal/conversation"
)

// This is an offline smoke test of a fresh store, not startup health or a
// release compatibility contract. Never use DefaultPath or startup wrappers.
// Defaults() probes platform memory, so no pure default-config API is used.
type preflightReport struct {
	Component      string            `json:"component"`
	Protocol       int               `json:"protocol"`
	Checks         map[string]string `json:"checks"`
	Health         string            `json:"health"`
	Compatibility  string            `json:"persistent_state_compatibility"`
	ExecutionReady bool              `json:"execution_ready"`
}

type preflightDeps struct {
	mkdir  func(string, string) (string, error)
	open   func(string) (conversation.Store, error)
	remove func(string) error
}

func runCandidatePreflight(rest []string, out, errOut io.Writer) int {
	return runCandidatePreflightWith(rest, out, errOut, preflightDeps{os.MkdirTemp, conversation.Open, os.RemoveAll})
}

// Accept no caller-selected paths or overrides. The process owner must bound
// runtime; this function bounds report size, not filesystem/writer latency.
func runCandidatePreflightWith(rest []string, out, errOut io.Writer, d preflightDeps) int {
	if len(rest) != 0 {
		io.WriteString(errOut, "cercano: usage: __internal-update --preflight\n")
		return privateExitUsage
	}
	checks := map[string]string{}
	for _, name := range []string{"temp_root", "store_open", "fixture_write", "store_close", "store_reopen", "fixture_read", "reopened_close", "cleanup"} {
		checks[name] = "not_run"
	}
	checks["default_config_validation"] = "skipped_no_pure_defaults_api"
	report := preflightReport{"cercano", 1, checks, "not_checked", "unknown", false}
	failed := false
	check := func(name string, err error) bool {
		checks[name] = "passed"
		if err != nil {
			checks[name] = "failed"
			failed = true
			return false
		}
		return true
	}
	root, err := d.mkdir("", "cercano-preflight-")
	if check("temp_root", err) {
		// Only remove the exact root returned by our successful MkdirTemp.
		func() {
			defer func() { check("cleanup", d.remove(root)) }()
			path := filepath.Join(root, "conversations.db")
			store, err := d.open(path)
			if !check("store_open", err) {
				return
			}
			written := check("fixture_write", store.EnsureConversation(context.Background(), "offline-preflight", "inert-project", "inert-model"))
			closed := check("store_close", store.Close())
			if !written || !closed {
				return
			}
			store, err = d.open(path)
			if !check("store_reopen", err) {
				return
			}
			info, err := store.Get(context.Background(), "offline-preflight")
			if check("fixture_read", err) && (info.ID != "offline-preflight" || info.ProjectDir != "inert-project" || info.Model != "inert-model" || info.TurnCount != 0) {
				checks["fixture_read"] = "failed"
				failed = true
			}
			check("reopened_close", store.Close())
		}()
	}
	// Never include paths or arbitrary underlying error text in bounded output.
	raw, err := json.Marshal(report)
	if err != nil {
		return privateExitExecution
	}
	raw = append(raw, '\n')
	n, err := out.Write(raw)
	if failed || err != nil || n != len(raw) {
		return privateExitExecution
	}
	return 0
}
