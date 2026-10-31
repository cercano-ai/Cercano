# Bounded unidentified-candidate upgrade regression

Validated on Darwin on `fix/upgrade-discovery-wal`, starting at
`e9c7e7287556089f57d0888c12be2cde54a129b4`.

## Behavior

Initial upgrade discovery ignores unrelated unavailable executable images rather
than requiring missing-image role/socket classification. Concrete ownership of
the requested listener with an unavailable executable still refuses shutdown.
The post-stop known-agent filter likewise excludes unidentified candidates, not
just recognized non-writers. This exclusion never authorizes signals. Identified
host filtering and native identity-checked shutdown/exit verification remain in
place. Shutdown and the known-agent check precede database opening.

## Validation in this run

- Disposable Go overlay restored HEAD's `classify_missing_darwin.go` and
  `upgrade_process_darwin.go` (adapting only the renamed predicate). Running
  `python3 test/fixtures/production-stop/build.py test -overlay=<temporary overlay>
  ./internal/brewrestart -run '^TestUpgradeIgnoresUnidentifiedUnrelatedProcess$'
  -count=1 -v` failed as expected (exit 1): the actual unlinked-image child
  blocked both initial discovery and the post-stop gate. Overlay removed.
- `python3 test/fixtures/production-stop/build.py test ./internal/brewrestart
  -run '^(TestUpgrade|TestScopedUnlinkedExecutableCandidates|TestDiscoveryInventoryAggregatesWithoutChangingRefusal|TestMissingWorkerRoleEvidence|TestProductionWorkerMissingExecutable)'
  -count=1`: PASS, 20.516s. Includes unavailable target-listener refusal,
  identified surviving-host gating, and scoped shutdown tests.
- `python3 test/fixtures/production-stop/build.py test ./cmd/cercano
  -run '^(TestOfflineUpgradeKnownAgentBeforeChanges|TestOfflineUpgradeDetectionLimits|TestOfflineUpgradeDiscoveryErrorBeforeOpen|TestOfflineUpgradeCheckOnlyNeverOpensStore|TestAutomaticUpgradeStopThenCheck)$'
  -count=1`: PASS, 0.673s.
- `python3 source/server/scripts/test-ordinary-upgrade.py`: 13 tests PASS.
- `python3 test/fixtures/production-stop/build.py`: PASS (offline isolated builds).
- `python3 test/fixtures/production-stop/run.py combined`: PASS. Expected before
  running: one shutdown phase stopping two hosts, one backup, one migration;
  confirmation then verified stop then backup/migration. Observed those counts,
  schema 2, four preserved history rows, fresh `FIXTURE_OK` reply. Unidentified
  child and sibling worker survived upgrade; both explicitly cleaned up afterward.
  Fixture reported `cleanup_remaining: {}` and generation `remaining: []`.
- `git diff --check`: PASS.

Only owned fixture PIDs/synthetic inventory and synthetic providers were used.
No ambient inventory, live transition, real database/credential access,
installation, push, or landing. Full repository suite was not run. Fixture output
is retained at `/private/tmp/cstop-66rjr5hb`; isolated build-driver logs and ignored
fixture binaries are retained. Pre-existing untracked inventory evidence and
`source/server/efforts/` were not cleaned up or investigated.
