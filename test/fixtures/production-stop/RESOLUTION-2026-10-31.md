> Historical evidence, superseded for upgrade acceptance by HOST-ONLY-2026-10-31.md.
> Descendant death, writer-tool tick cessation, descendant freeze/STOP, and private
> stdio shutdown are NOT current upgrade requirements. Workers/tools may survive;
> only independently verified DB-writing hosts are stopped. Independent writer
> uncertainty still refuses. Historical results below are not current assertions.

# Unresolved-child diagnostics investigation (not release acceptance)

Branch: `fix/upgrade-resolution-diagnostics`, based on `63bb5d6eb19cda49651f35ee6a6bbc5919e8985d`.

## Evidence and implemented scope

The earlier isolated probe reproduced the generic `unresolved host child: context deadline exceeded` with a compiled disposable host and an attached same-user worker whose executable basename was unrecognized. This is an intentionally refused child, not proof of the user's ambient cause. The resolution loop discarded both role-read errors and classification mismatches on every retry, leaving only the context error. This diagnostic loss is the confirmed defect fixed here. No ownership or writer gate was relaxed.

Expanded native child fixtures exercise recognized worker images, unrecognized worker image, host-like roles `agent` and `development-host`, and an arbitrary secret-like argv[1]. The latter is reported only as `unrecognized`. Unknown children remain alive and stop refuses; verified model-host exclusion and rollback coverage remain intact. The test helper preserves its original standalone worker entry point as well as supporting explicit child-role fixtures.

Final diagnostic examples (`source/server/artifacts/affected-final.log`):
- PID 47936: role=worker, reason=unrecognized executable identity.
- PID 47953: role=agent, reason=unsupported role.
- PID 47956: role=development-host, reason=unsupported role.
- PID 47959: role=unrecognized, reason=unsupported role (secret-like input not printed).

Errors retain the context cause and add host PID, unresolved count, omitted count and at most eight child PID/reason entries. Executable paths and argv values are not added to these diagnostics.

## Executed verification

From `source/server`:

- `FINAL_READ_ONLY_INVENTORY=0 go test ./internal/brewrestart -run '^TestUpgrade' -v -count=1`: PASS (`artifacts/upgrade-final.log`). Includes real host/worker stop, different images, cancellation cleanup, multiple hosts, unrelated worker survival, unknown roles, bounded/redacted diagnostics, STOP/exec rollback, model-host exclusion and published in-flight fork.
- `FINAL_READ_ONLY_INVENTORY=0 go test ./internal/brewrestart -v -count=1`: NOT PASS (`artifacts/brewrestart-final.log`). `TestMissingExecutableZeroFDAndConfirmedExit` and `TestSurvivingAliasConcreteWriterRisks` fail. The initial package run additionally caught a fixture-entry regression introduced while adding role fixtures; that regression was fixed and unrelated-worker tests subsequently pass.
- Re-ran the two remaining failures using HEAD versions of all three edited tracked Go files, temporarily removing the new diagnostic test and restoring all edits in a finally block: SAME failures (`artifacts/baseline-probe.log`). These are baseline failures, not waived sign-off. No full repository suite was run.

From the worktree root:

- `python3 test/fixtures/production-stop/build.py`: PASS (`source/server/artifacts/fixture-build-final.log`). Offline isolated fixture builds.
- `python3 test/fixtures/production-stop/run.py combined`: PASS (`source/server/artifacts/combined-final.log`). Disposable run root `/private/tmp/cstop-gm3dk6mp`; confirmation precedes native stop/verified exit, backup and migration. Migration version 2, replacement generation selection, four preserved history rows and fresh `FIXTURE_OK` reply. Unidentified fixture process and unrelated sibling worker survived upgrade; cleanup reports no remaining owned fixture processes.

The combined run uses the fixture-scoped candidate inventory seam, not global discovery. Native identity, socket, freeze and signaling logic run against disposable processes. Generation/backend fixtures do not qualify a production model/provider or installed launcher. Artifacts are local, untracked logs under `source/server/artifacts/`; the disposable database/configuration are under the run root above, not the user's state.

## Blockers / uncertainty

The user's actual unresolved PID/role/identity remains unknown. No ambient inventory was performed; no ambient processes were signaled, live configuration/database/credentials inspected, installed launcher executed, real history migrated, or infrastructure installed. Full ambient ordinary-launcher acceptance cannot be claimed under the fixture-only inventory constraint. Diagnosing that specific timeout requires the new bounded error from the user's failing run or separately approved narrowly scoped observation. Do not replace refusal with permissive classification based on these fixtures.

This is a diagnostic fix and expanded regression evidence, not a claimed behavioral fix for the reported timeout. Return to parent for independent review; no delegation tool was available. Do not land or push this checkpoint.
