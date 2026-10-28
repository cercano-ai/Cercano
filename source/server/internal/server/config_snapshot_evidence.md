# Bounded server snapshot integration (2026-10-28)

Scope: server and hostsvc/config only. No pkg/config or setupreset changes in this checkpoint; no new writer authority or stale-save retry.

## Caller coverage

- Removed error-discarding persistConfig. SetActiveCloudProfile, UpsertCloudProfile and RemoveCloudProfile publish isolated drafts before turn cancellation, catalog invalidation, provider rebuild or credential deletion. SetCloudProfileKey preserves credential-first ordering and reports credential-saved/config-not-applied if selection publication fails.
- ChatGPT and Claude login commits retain the begin-time snapshot. hostsvc/config commits credentials first, then publishes the candidate with saveAndSetLocked, retaining SaveSnapshot's returned baseline in current. Conflict leaves live configuration unchanged, but does not roll back credentials. Server login handlers serialize commit plus rebuild and report saved/not-activated failures.
- RemoveCloudProfile is the bounded profile/credential removal path; enterprise Logout belongs to enterprise authority, not this YAML writer. Removal failure diagnostics do not expose credential-store errors.
- UpdateConfig publication plus endpoint/provider activation, profile operations, login commits, routing, backup and runtime-detection publication use the same server configApplyMu boundary. The config service retains returned snapshots; UpdateConfig rereads its committed snapshot for activation. Runtime installation itself remains outside publication and is not rolled back.
- Watcher loads authoritative configuration under the same host boundary, adopts even no-diff revisions, checks disk revision before application, and never resaves its callback. Rejected and saved-but-not-applied outcomes are logged. SetConfigPersistence remains startup adoption, not a save.

## Evidence

Earlier narrow baseline reproduction recorded in the continuation history: stale profile upsert reported success and changed live configuration; no-diff watcher reload failed to adopt the baseline; B completed while A was paused after publication. Those are baseline failures, not current results.

Commands rerun successfully for this checkpoint from source/server, all with CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off:

```
go test -race ./internal/hostsvc/config ./internal/hostsvc/credentials -count=1
go test -race ./internal/server -run 'Test(ProfileSnapshot|WatcherSnapshot|UpdateConfigSnapshot|ConfigWatcher|UpdateConfig|UpdateRouting|BackupSaveBoundary|RuntimeSaveBoundary)' -count=1
go test ./... -run '^$'
```

The last command is API/test compilation only, not the full suite. Fixtures use temporary files/fake stores; no native opener, real provider, production subprocess startup or live-user activation was exercised. New tests cover stale profile upsert/selection/removal, login conflict with explicit credential-saved outcome, watcher no-diff baseline adoption/no writeback and stale callback rejection, and the paused-publication update interleaving. Existing focused tests cover update/routing/backup/runtime rejection boundaries and watcher behavior.

No config/credential atomicity, installation rollback, power-loss durability, exhaustive cross-process/platform qualification or Phase 3 completion is claimed. Guarded setup reset and config storage implementation are separately owned.
