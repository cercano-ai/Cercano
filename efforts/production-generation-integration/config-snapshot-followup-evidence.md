# Snapshot follow-up — partial, 2026-10-28

Continuation after 8fa0fc6989bf. This is NOT Phase 3 completion or all-writer safety evidence.

## Changes and actual probes in this run

* Backup profile publication uses SaveAndSet before rebuild (inherited uncommitted correction, now tested). Its inherited deterministic stale-baseline fixture passed here; the earlier red backup result is history, not a newly repeated red.
* Runtime detection now uses SaveAndSet in both detection-result branches instead of Set then Persist (or Set alone). A helper exposes this boundary without invoking installers. With the old Set/Persist ordering, `TestRuntimeSaveBoundaryRejectsBeforeLiveEffects` failed here: `rejected detection applied live`. After correction it passed under race. Installation still precedes detection/publication and is not rolled back; no installation was executed. Detection-error partial results now require successful publication before live installation.
* Added coordinator-local read injection and an existing-file permission-denied fixture. It checks errors.Is(os.ErrPermission), no publication, and unchanged file. This is injected permission denial, NOT native mode-bit verification.

All tests used `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`; new fixtures use temporary files and memory secrets. No native credentials, production subprocess startup, downloads or activation.

Commands run successfully from source/server:

* `go test ./pkg/config ./internal/hostsvc/config -count=1`
* `go test ./pkg/config ./internal/hostsvc/config ./internal/server ./cmd/cercano -run '^$'` (compile only)
* `go test -race ./pkg/config ./internal/hostsvc/config ./internal/server ./cmd/cercano -run 'TestSaveBoundary|TestSaveAndSetBoundary|TestBackupSaveBoundary|TestRuntimeSaveBoundary|TestUpdateConfig|TestUpdateRouting|TestSetupConfigBoundary|TestCoordinatedTwoStore' -count=1`

The earlier non-race focused backup/runtime command intentionally failed before the runtime fix. Sequential saves, stale copies, concurrent creators and publication failures are covered by the existing focused boundary tests, not new independent-process qualification.

## Inspected paths still unsupported / unfinished

* setupreset/reset.go still publishes through its private temporary-file rename, bypassing the coordinator, after credential deletion/live apply. This is a remaining implementation gap, not a generic scope objection. Expected-baseline guarded publication before destructive effects remains required.
* server.go:957 still discards Persist errors. SetActiveCloudProfile mutates selection, cancels turns, logs and rebuilds before persistence; SetCloudProfileKey writes credentials and changes selection before persistence; UpsertCloudProfile mutates/rebuilds before persistence; RemoveCloudProfile removes live metadata/credentials and rebuilds before persistence. None was migrated in this slice.
* chatgpt_login.go and claude_login.go still call persistConfig after login commit/provider effects. Login commit and logout/credential-service callbacks require complete candidate/effect-order migration and tests; no login/logout qualification is claimed.
* config_watcher.go:106-216 loads a fresh config but constructs an UpdateConfig request against cfgSvc.Get without adopting the loaded snapshot. UpdateConfig saves using the old live baseline: meaningful external edits can conflict rather than reload. No-diff external edits do not advance the live baseline. Its header's old echo-loop explanation is insufficient under explicit snapshots. No watcher fix or deterministic watcher execution was performed; blindly rebasing the live draft is NOT a fix.
* Actual publication inventory includes coordinator atomic rename and setupreset private atomic rename; pkg/setupstate/state.go writes separate setup-state JSON, not config YAML. Direct SaveSnapshot callers remain startup relocation, two setup saves and hostsvc saveAndSetLocked. This source search included filesystem write/rename primitives and caller effects, not only config.Save. It is not an exhaustive repository-wide proof for indirect callbacks, non-Go writers or every filesystem primitive.

Remaining requirements: guarded reset and fixtures; profile/login/logout migrations and fixtures; watcher apply/no-save-loop/baseline tests; exhaustive filesystem/callback inventory; full affected suites and broader process/platform qualification. No config/credential atomicity, installation rollback, full API effect coverage or Phase 3 completion is claimed.
