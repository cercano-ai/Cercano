# Config save boundary — partial implementation evidence

2026-10-28. Phase 3 remains in progress; not activation evidence.

## User decision

The current execution request explicitly approves an immutable explicit versioned
ConfigSnapshot API reusing ConfigCoordinator, not a stateful writer. Successful
saves propagate a new snapshot; copied drafts retain the original baseline.
Conflicts require reload/reconciliation, never stale-draft rebasing. Setup must
refuse malformed/unreadable existing configuration with path-bearing nonsecret
instructions. Creation requires genuine absence and the same publication lock.
No repair/reset, activation, thaw, native credential access or live-user data.
This supersedes the API-choice blockers in config-save-boundary-blockers.md.

## Implemented slice

Config.Snapshot and WithSnapshot carry value baselines. SaveSnapshot returns the
next ConfigSnapshot via ConfigCoordinator.Save. Legacy Save fails closed and does
not advance its by-value input. Coordinator revision comparison and publication
share the filesystem lock, including absent-file creation. Full save retains its
existing serialization semantics; precise Change remains distinct.

Production direct callers re-inspected in this run: startup credential relocation
(main.go:654), setup creation (:1420), setup final save (:1472), and hostsvc config
saveAndSetLocked (config.go:329). Each adopts the returned snapshot. Setup aborts
load failure before prerequisite work and the second save uses the first result.
Service SaveAndSet publishes before changing current config or canceling logins.
Routing assignments use it; UpdateConfig stages broadcasts, runtime/model,
retention, compaction, watchdog and cloud credential/provider effects until save
succeeds. Cloud application failure after publication is reported as saved but
not applied, not as rejected publication.

## Verification in this execution

All Go commands used CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off.
No executable startup, native credential opener or live-user config was invoked.

* Inherited regression: conversation history records a disposable-fixture
  TestSaveBoundaryChangeDisappearance failure and a subsequent explicit
  ErrConfigConflict check. That red run was not repeated in this execution.
* Green: go test ./pkg/config ./internal/hostsvc/config -count=1.
* Green: go test -race ./pkg/config ./internal/hostsvc/config ./internal/server
  ./cmd/cercano -run 'TestSaveBoundary|TestSaveAndSetBoundary|TestUpdateConfig|TestUpdateRouting|TestSetupConfigBoundary|TestCoordinatedTwoStore' -count=1.
* Green: compile-only go test for those four packages (-run '^$').
* Green: go build -o /tmp/cercano-config-boundary-check ./cmd/cercano.
* Green: git diff --check.

Existing inherited tests cover sequential snapshots, stale copies, competing
writers/creators, malformed and non-regular unreadable authorities, failed
publication, missing baseline/retarget, and rejected service/routing/config saves.
The earlier red reproductions described in conversation history were not rerun
in this execution; do not treat this note as new verification of those reds.

## Exact remaining gaps — task NOT complete

The broader writer/live-mutation audit still identifies unconverted behavior:

* internal/setupreset/reset.go:97,109 writes YAML using its own writeAtomic,
  outside ConfigCoordinator. Reset also applies live state/deletes credentials
  before publication. No reset was executed or new reset policy introduced.
* internal/server/cloud_backup.go:33-44 mutates backup profile and rebuilds before
  Persist; rejection can leave changed live state.
* internal/server/open_runtime_install.go:68 persists after Set; installation and
  live changes are not transactional with rejected publication.
* internal/server/server.go:957 persistConfig discards Persist errors; its profile
  and login/logout callers require a separate candidate/effect-order migration.

Therefore this slice does NOT establish all-production rejected-save safety or
eliminate all competing writers. General Phase 3 cross-host visibility and
credential invalidation remain outstanding. Full server/CLI suites, independent
process race qualification, native permission-denied fixtures, and exhaustive
UpdateConfig effect instrumentation were not run. Focused race coverage is not
native production qualification. No merge, push or launcher change.
