# Historical recovery fixtures

## Supported entry point

From repository root, explicitly opt in:

```sh
# Build only (does not claim acceptance):
OLD_RECOVERY_PRODUCTION_FIXTURE=1 sh efforts/old-host-compatible-recovery/fixture/build-production.sh
# Native historical acceptance (separate authorization/platform prerequisites):
OLD_RECOVERY_PRODUCTION_FIXTURE=1 OLD_RECOVERY_EXECUTE=1 sh efforts/old-host-compatible-recovery/fixture/build-production.sh
```

Acceptance selects exactly `TestHistoricalProductionController` in
`source/server/cmd/cercano/native_controller_test.go`, including `drain`,
`reserved`, and `blocked-timeout`. The checked runner requires all four names to
run and pass; missing selections, skips, and failed commands are errors.
Build-only mode is intentionally not test execution. Python 3 and the repository
Go toolchain/dependencies are prerequisites; the build remains offline.

This is a pinned historical host/worker with fixture-tagged loopback inference,
not qualification of arbitrary installed binaries or live data. Runtime support
is bounded to the same-user Darwin prepared-generation containment model.
Historical verification documents remain historical evidence, not new results.

## Retired composition fixture

`run.sh` now refuses with exit 2 before building or launching anything. Its
original contents and synthetic `oldmain.go.txt` are preserved byte-for-byte as
non-entry-point text under `archive/`. The old selection
`TestActualOldRecoveryCharacterization` has no remaining test definition; a
successful Go command selected zero tests. Do not execute archived text as an
acceptance runner. Older verification records referencing these paths describe
their historical baseline and have deliberately not been rewritten.

## Safe runner-only regression

```sh
python3 -m unittest discover -s efforts/old-host-compatible-recovery/fixture -p 'test_*.py' -v
```

Uses disposable standard-library-only Go modules, no native controllers, hosts,
user stores, or VMs. It does not certify recovery behavior.
