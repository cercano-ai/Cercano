# Same-lifetime exec review verification

Corrected source checkpoint: `4eb08e2fd4e17c7d495f2eb94661b34f4ee5df26`.
This report is a subsequent documentation-only checkpoint, not an exact-final-SHA test claim.

## Fix and regression

The grace loop now shares `processExitStatus` with `waitProcessExit`: ESRCH or changed start time means original exit; same lifetime with changed image refuses with manual-stop guidance. Changed identities are removed from the escalation/cleanup map before returning refusal. No STOP or descendant signalling was added to the upgrade policy.

The deterministic `TestUpgradeGraceLifetime` was established before the fix: the exec case failed with `expected actionable exec refusal, got <nil>`. It now covers unchanged identity, exec refusal with no remaining signal target, PID reuse, and ESRCH. TERM-policy coverage distinguishes initial EPERM, cleanup EACCES, and cleanup deadline via errors.Is; cancellation-after-TERM coverage remains passing.

Classification reviewed against cmd/cercano/main.go flag declarations and dispatch: `--mcp` without a nonempty `--grpc-addr` is embedded MCP (host); with an address it is a client. MCP takes precedence over --server. Classification never replaces kernel/start/listener verification. No classification change needed.

## Corrected-source commands/results

All commands below exited 0 after the source checkpoint:

```
python3 test/fixtures/production-stop/build.py test ./internal/brewrestart ./cmd/cercano -count=1
# brewrestart 29.964s; cmd/cercano 51.901s
python3 test/fixtures/production-stop/build.py
# fixtures freshly rebuilt, 11.987s
python3 test/fixtures/production-stop/run.py combined
# 41.939s; fixture_combined_acceptance=true
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo . --evidence /tmp/upgrade-exec-review-compatible
# 117.433s; PASS bounded compiled A/B rehearsal
```

Combined stdout: `/tmp/upgrade-exec-review-combined.log`.
Combined artifacts: `/private/tmp/cstop-bzgf0rvq` (result.json, migration evidence, host/client logs, synthetic DB snapshots).
Combined result records source_commit equal to the corrected checkpoint. One shutdown, one backup, one migration; four history rows preserved, fresh reply and replacement startup successful; sibling worker served after generation change, unidentified child survived upgrade. Generation cleanup remaining=[] and production-fixture cleanup_remaining={}.

A/B stdout: `/tmp/upgrade-exec-review-compatible.log`.
A/B report: `/tmp/upgrade-exec-review-compatible/compiled.json`.
A/B disposable artifacts: `/private/tmp/ca-brplkm3t`.
A/B provenance records checkout `4eb08e2fd4e17c7d495f2eb94661b34f4ee5df26`, fresh snapshot `2c1cbd0ed446c6f9451dc7b775dc1d10d7757ae5`, source tree `f7ca1ae59a6b809cea098ebb483cc082ff0bc6c1`. Compatible path used zero shutdown/migration helper calls; A remained available through B failures/publication, busy resume preserved A, later transfer preserved history and retired A; cleanup remaining=[]. This is bounded offline compiled acceptance, not live deployment parity.

Only disposable fixture processes/stores and loopback synthetic model services were used. No live DB, credential/model service, or installation access; no merge/land/push. Harness teardown signals to its owned fixture children are cleanup, not upgrade-policy signals. Disposable artifact directories are retained for audit; owned process cleanup completed.
