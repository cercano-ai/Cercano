# Upgrade discovery preflight — parent review required

Scope: independent discovery/progress fixes only. `compatible_store` is unchanged;
the choice about SQLite SHM effects remains pending. Nothing here authorizes migration.

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
The compatibility inspection function body and migration/backup semantics are unchanged.

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
