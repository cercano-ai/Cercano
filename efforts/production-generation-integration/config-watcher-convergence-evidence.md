# Bounded cross-host watcher convergence — 2026-10-28

Phase 3 watcher/apply task remains **in_progress**. This is not full cross-host policy certification or activation approval.

## Source and fixtures

`config_cross_host_test.go` uses two Server instances with independent config services sharing one temporary canonical file, memory credential stores and existing fake provider/runtime fixtures. It invokes the ordinary reload callback deterministically; no full startup, subprocess, live data, native credentials or provider transport is used. Cases cover SaveAndSet and UpdateConfig publication, exact revision adoption without a file rewrite, echo suppression, subsequent saves from the adopted baseline, simultaneous stale whole drafts (one conflict, no silent refresh), and malformed external YAML (last-known revision retained, diagnostic visible, no application event or overwrite).

`config_cross_host_failure_test.go` exercises a failing provider rebuild on the receiving host, including durable baseline adoption without an application event and a visible saved/not-applied diagnostic. The direct UpdateConfig failure case checks the RPC outcome and suppressed application events.

Minimal existing-path correction: UpdateConfig no longer treats an arbitrary provider rebuild failure as success. Queued effects/notifications occur after fallible provider application. Providers expose a typed CloudUnavailableError for the existing successful publication of a new routing graph with an absent cloud sentinel; this preserves ordinary open-only/missing-cloud behavior and the existing cloud-only fail-closed regression instead of confusing an unavailable cloud with failure to install policy. No writer, authority, service, watcher, configuration representation or activation path was added.

## Verification in this continuation

Commands from `source/server`, each with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

* PASS: `go test ./internal/server -run 'TestCrossHostConfig|TestConfigApplyCounterexample|TestReloadConfigFromDisk|TestStartConfigWatcher|TestUpdateConfig' -count=1`
* PASS: `go test -race ./internal/server -run 'TestCrossHostConfig|TestConfigApplyCounterexample|TestReloadConfigFromDisk|TestStartConfigWatcher|TestUpdateConfigCloudOnlyMissingAccountDoesNotKeepLocalPolicy' -count=10`

The first command includes ordinary UpdateConfig regressions; the second repeats bounded convergence/failure cases under race plus the existing watcher and cloud-only missing-account cases. Existing fsnotify coverage is single-host; new two-host tests drive the same reload entry directly rather than claiming two-process notification qualification. Full suites/builds were not rerun in this slice.

## Exact remaining admission boundary

`applyConfigFromDisk` compares the loaded revision with cfgSvc.Get's snapshot. `updateConfigLocked` adopts/saves that snapshot before provider application. On a genuine failed rebuild, durable state advances but the provider graph may remain old. Existing DispatchCandidates can still expose that old graph, and a subsequent identical-revision reload is skipped. The new watcher failure test explicitly records both facts. Suppressing success events does NOT fence new model work, make GetConfig a live-policy acknowledgment, or implement retry/reconciliation. Reconfigure also has no error return. Closing these gaps needs a distinct applied-revision/admission/retry contract across consumers, not silent rebasing or another settings broker; it is left for the next bounded slice.

Runtime endpoints or other effects before the failure are not rolled back; saved/not-applied may describe partial application. Environment resolution, restart guidance, frozen/prepared configuration and canonical activation are unchanged. No phase completion, merge, push, native-backend safety or complete invalidation guarantee is claimed.
