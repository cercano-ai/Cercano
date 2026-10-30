# Upgrade discovery preflight — parent review required

Scope: corrected discovery/progress plus lightweight WAL-aware compatibility
inspection. SQLite SHM coordination is now explicitly permitted during READ ONLY
inspection. Nothing here authorizes migration.

## Build and proposed invocation

From `source/server`, build without running:

```sh
go build -o /tmp/cercano-upgrade-discovery-preflight ./cmd/upgrade-discovery-preflight
```

**Only after parent review**, using the actual configured numeric loopback endpoint:

```sh
/tmp/cercano-upgrade-discovery-preflight --address 127.0.0.1:8080
```

8080 is an example, not inferred configuration. No config/database is loaded. Exit
0 means this instant's bounded discovery passed, 1 means refusal, 2 invalid input.
No listener found is not proof that the port is free or that all writers are absent.
Renamed/custom non-listening writers, other users and future processes remain outside
the existing bounded known-name detector. This is not a writer fence.

## Exact production call graph (macOS/cgo)

* `cmd/upgrade-discovery-preflight.main` -> `preflight` (flags, endpoint validation,
  elapsed output) -> `brewrestart.PreflightUpgradeHost`.
* `PreflightUpgradeHost` -> `ListCandidates` -> `collectPIDs` ->
  `C.cercano_list_user` -> `proc_listpids(PROC_UID_ONLY, getuid())`;
  then `discoverUpgradeHost` on the returned production inventory.
* `discoverUpgradeHost` -> `inspectUpgradeCandidate` -> `Inspect` ->
  `C.cercano_identity` (`proc_pidinfo`, `proc_pidpath`, `proc_pidinfo`). On failure,
  `confirmedInspectionError` may use `C.cercano_identity_exists` (`proc_pidinfo`).
* Only ENOENT enters `missingExecutableEvidence`: `C.missing_info`
  (`proc_pidinfo`) -> `C.missing_args` (`sysctl(KERN_PROCARGS2)`) ->
  `C.missing_info`, kernel-name/path checks (`os.Lstat`), and repeated `Inspect`.
  Known names, possible aliases/replacements, unstable snapshots and unresolved
  evidence refuse. For remaining candidates, `holdsListenerInspected` uses this
  evidence reader before and after `C.cercano_listener` (libproc FD/socket metadata).
  An owned target listener or socket inspection error refuses. Only a stable,
  non-known-name, non-target-listener candidate is excluded; the zero identity
  grants no signal authority. Saved exec path is not argv[0]; kernel executable
  names additionally catch a known host launched through an unrelated symlink.
* Ordinary identities: `HoldsListener` -> `holdsListenerInspected` with `Inspect`
  before/after `C.cercano_listener`. Known non-listener agents additionally use
  `processRole` -> `C.upgrade_args` (`sysctl(KERN_PROCARGS2)`) -> `Inspect`.
  Only verified host/worker associations or model-host roles pass those checks.
* Returns to the command. **No edge to `StopUpgradeHost`, freeze/signal/restart,
  agent startup, config loading, SQLite, backup, chmod or migration.** The imported
  brewrestart package has no init function. Unsupported platforms refuse.

Shutdown shares `discoverUpgradeHost`, then continues its existing freeze/tree/
shutdown protocol; the preflight stops before that protocol. Refactoring introduces
no alternate signal authority or architecture choice.

## Disposable verification and evidence

`TestProductionEnumerationUnlinkedExecutable` calls the REAL `ListCandidates`,
asserts each disposable child is present, then restricts inspection/discovery to
that child's PID. It never inspects/signals ambient candidates or opens a DB.
It proves `Inspect` still returns ENOENT after unlink, while unrelated candidates
are excluded and historical/current hosts, model-hosts, workers and unknown target
listeners retain specific refusal. Intact known-name listener discovery is checked
before unlink. A symlink-host regression initially FAILED (incorrect exclusion)
and passes with the kernel-name/alias guard. Cleanup kills/waits only its own child.

`test_launcher_progress.py` uses an actual separate-process flock holder, not a
mock, to verify the contention message; uncontended locks do not claim waiting.
Phase timers preserve raised refusals. Launcher instrumentation covers inspection,
helper build, discovery/shutdown, backup and migration; Go separately times discovery.
Compatibility now uses normal SQLite `mode=ro` and one read transaction for
version, marker and historical-schema metadata. It does not copy the database,
checkpoint, or perform a full integrity scan. SQLite may create/update `-shm`;
application data/schema and existing permissions remain unchanged before consent.
Database, parent and existing sidecar ownership/alias checks remain enforced.
Unknown/incompatible metadata still refuses. This is metadata admission, not a
full corruption check; the post-consent backup integrity check remains intact.

Tests (from repository root unless noted):

```sh
# from source/server:
go test ./internal/brewrestart -run '^(TestProductionEnumerationUnlinkedExecutable|TestInspectionDisappearanceClassification|TestInspectReapedDisposableProcess|TestCollectPIDs.*|TestSameProcessDetectsPIDReuse)$' -count=1 -v
go test ./cmd/upgrade-discovery-preflight -count=1 -v
# from repository root:
PYTHONDONTWRITEBYTECODE=1 python3 source/server/scripts/test_launcher_progress.py -v
```

The new command has NOT been executed against live inventory. Production inventory
was enumerated only by the isolated regression above. No live database access,
ambient signals, installation, permission changes, landing or push are required.

## WAL correction verification

The correction baseline `2e41b2f4` copied DB/WAL, checkpointed the disposable copy,
then used immutable reads and `integrity_check`. Source-boundary regression tests
now prohibit those operations in compatibility inspection, while requiring the
post-consent backup integrity check. Disposable tests put committed metadata only
in WAL (including missing SHM), compare logical application data/schema, and check
unknown markers/versions and duplicate metadata refusal. Byte identity is not an
invariant because SQLite coordination is permitted.

Focused passing verification includes `test_launcher_compatibility.py`,
`test_launcher_progress.py`, `test-launcher-safety.py`, `test-launcher-feedback.py`,
and ordinary-upgrade WAL refusal, unsafe-sidecar, unsupported-before-prompt and
noninteractive/decline cases. Feedback assertions were aligned with the existing
nonblocking lock/progress behavior. Discovery verification additionally covers
`TestMissingExecutableFinalInspection`,
`TestMissingListenerRequiresConfirmedDisappearance` and
`TestMissingExecutableZeroFDAndConfirmedExit`. Preflight boundary tests inject
discovery and cover absent/invalid endpoint, config/database flags, extra arguments,
IPv6, success and refusal. No full-suite or live inventory run is claimed.

The preflight has **no default address**. Full production candidate enumeration can
be run without reading live config by supplying an explicit numeric loopback
endpoint; the documented 127.0.0.1:8080 remains an example, not the verified live
endpoint. Parent review and execution are still required.

Reviewed artifact built (not executed) from the corrected worktree:
`/tmp/cercano-preflight-reviewed.I9l68kCJ/upgrade-discovery-preflight`

SHA-256: `27e91271fed4b99cb6a63300d1ec1c59f5c903e4f6080ca8e4728ef203ea7cae`

Example only, for parent execution after review with the correct explicit endpoint:

```sh
/tmp/cercano-preflight-reviewed.I9l68kCJ/upgrade-discovery-preflight --address 127.0.0.1:8080
```
