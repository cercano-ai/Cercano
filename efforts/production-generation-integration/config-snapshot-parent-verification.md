# Explicit snapshot save integration — parent verification, 2026-10-28

Approved decision recorded in config-snapshot-approval.md (bb53aaba2764). Implementation checkpoints: 8fa0fc6989bf snapshot boundary/setup/routing/UpdateConfig; 5319562ca3b2 runtime and backup staging; 4938cce328e6 load validation; f29e5be3cc38 existing reset coordinated publication; 030bcd8c29e7 profile/login/watcher/activation ordering; f82cd00064b4 independent-process tests; bdd8724a3603 fixture baseline migration; fd55a88d2aed reset activation serialization and cleared-model watcher fix.

No stateful writer, new authority, automatic repair, native credentials, live config change, activation, merge or push. Existing full Save serialization remains legacy struct serialization (not unknown-field preservation or environment-overlay separation). Config and credential changes are not an atomic transaction; saved-but-not-applied results and credential-first partial failures are explicit. Runtime installation is independently effectful and not rolled back by a rejected config save.

## Parent-executed verification

All commands used CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off in the execution worktree.

- Initial focused snapshot/routing/setup race checks passed.
- Expanded selector found three baseline-less fixture failures (watcher seed, routing crosslayer, setup reset). These were not production-protection failures; reproduction and fixture migration recorded in snapshot_fixture_evidence.md. That failed run is retained as failed evidence, not erased by later success.
- After final fixes, full `go test -race ./pkg/config ./internal/hostsvc/config ./internal/hostsvc/credentials ./internal/setupreset -count=2` passed all four packages.
- `go test -race ./internal/server ./cmd/cercano -run '^Test(UpdateConfig|UpdateRouting|StartConfigWatcher|ReloadConfig|ConfigApply|Snapshot|SetupReset|SetupConfigBoundary|RoutingSettingsClientPersistenceSnapshotExecution|.*Reset.*Snapshot|.*Watcher.*Override)' -count=2` passed both packages.
- `go test -race ./internal/server -run '^Test(ProfileSnapshot|WatcherSnapshot|UpdateConfigSnapshot|ConfigWatcher|BackupSaveBoundary|RuntimeSaveBoundary|Reset.*|Watcher.*)' -count=2` passed.
- Server `go build ./...` and CLI `go build -o /dev/null .` passed; binaries were not executed.
- Direct production save-call search under source found three main.go SaveSnapshot calls and hostsvc/config SaveSnapshot; server settings/profile/routing/runtime/backup publication uses SaveAndSet. Search in internal/server found no legacy persistConfig/Persist/config.Save calls. This lexical search alone does not establish exhaustive behavior for aliases or arbitrary filesystem writers. Existing reset separately uses coordinator ResetSetup.
- CLI SaveActiveTheme writes separate ui.yaml, not application config.yaml; theme authority coordination remains outside this bounded main-config change.

Read-only final review task task_7c6ec12e3b14ce330ffa1a5c0c023557 found no concrete blocker in inspected reset locking and snapshot adoption, but explicitly did not certify exhaustive caller coverage/callback lock cycles. Earlier review's reset race and empty-model override defect were reproduced and fixed at fd55a88d2aed; worker reports 20 race repetitions for narrow regressions, not independently repeated 20 times by parent.

Independent-process tests are detailed at efforts/snapshot-process-test-evidence.md; task reported exactly one winner/one conflict for absent and existing authorities and lock timeout while parent held the OS lock, race count=3.

Broader isolated suites are delegated as task_2088f61cab89a0aa0e4e680d40c9a254; results pending when this note was written. Cross-host invalidation, remaining shared-mutation work, full production parity and activation remain incomplete. No whole-Phase-3 completion claim.
