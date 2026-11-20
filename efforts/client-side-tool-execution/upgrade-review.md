# Offline upgrade review (2026-11-19)

Independent read-only review of initial mechanism (task task_213aa59af348299a826508808ea8b87d), before production transition work:

- Blocker: backup file Sync does not make the newly created directory entry durable. Flush parent directory with suitable platform handling before source migration SQL. Fail before mutation on flush failure.
- Missing evidence: test keeps no source connection alive, so WAL normally checkpoints before backup. Add test keeping another connection open, disabling auto-checkpoint, committing distinctive history/task/ownership data, asserting nonempty WAL, then checking standalone backup.
- Legacy adoption had no automatic backup in initial slice; production follow-up must cover it.
- Initial production migration registry empty: mechanism only, not completed feature. Follow-up is adding real storage schema without backfilling or enabling client routing.
- Reviewer's test invocation used an unavailable absolute Go executable; no review tests ran. Main agent used `go` successfully but encountered the expected transient compile failure during active implementation (executionBindingsSchema not yet added); rerun after edits settle.

## Resolution and verification (2026-11-19)

- Reproduced the durability blocker before fixing it with an injected sync callback in `TestOfflineBackupDurabilityBoundary`: `go test ./internal/conversation -run '^TestOfflineBackupDurabilityBoundary$' -count=1` failed with `synced directory flags [false]; want [false true] before source writes`. This instruments the actual backup helper, not a simulated backup.
- Fixed `versioned_upgrade.go`: after the snapshot file is synced and closed, open and sync its parent directory, then close it, propagating all errors before migration statements. The callback seam is per-call (no mutable global). This follows `internal/server/resume.go`'s open-directory/File.Sync/error-propagation convention. Windows explicitly refuses the durability boundary rather than pretending ordinary file-handle flushing establishes directory durability. Other platforms/filesystems fail closed if directory opening or syncing is unsupported or fails. A failed backup remains reserved for operator inspection; retry does not overwrite it.
- Added `TestOfflineUpgradeSyncFailureBeforeMutation` for both file and directory sync failures. It checks schema and both version markers inside the still-open source transaction, before rollback can hide mutation.
- Added `TestOfflineUpgradeCommittedWALBackup`: a retained dedicated connection disables autocheckpoint, commits distinctive history/task/ownership-epoch data, and asserts the WAL is larger than its header immediately before production v2-to-v3 upgrade. Only the backup main file is copied into a separate temporary directory; standalone reads verify the committed data, old markers, and absence of the new binding table. The migrated source passes v3 preflight.
- The already-implemented production v2-to-v3 registry and migration semantics were preserved. Legacy adoption already calls the same backup helper before schema writes (`offline_upgrade.go`); it now receives the directory barrier too. Existing production-upgrade and legacy-adoption tests pass.

Verification on Darwin, from `source/server`:

```sh
go test ./internal/conversation -run 'Test(OfflineBackupDurabilityBoundary|OfflineUpgradeCommittedWALBackup|OfflineUpgradeSyncFailureBeforeMutation|DeclaredVersionUpgrade|ProductionBindingUpgrade|OfflineUpgradePreservesAndRepeats)$' -count=1 -v
go test ./internal/conversation -count=1
```

Both passed after the fix. All database fixtures were temporary; no real user databases were used. Linux and Windows were not runtime-tested. Windows offline upgrades requiring a new backup are intentionally unsupported/fail closed pending a validated platform-specific publication barrier. These tests demonstrate barrier ordering and error handling, not physical power-loss survival or guarantees beyond the OS/filesystem sync contract.
