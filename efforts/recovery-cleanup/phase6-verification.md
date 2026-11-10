# Phase 6 — final candidate acceptance and documentation

## Scope and candidate

Verified production candidate `9e7ff70a87efa8f40fdd8b5cf200b97467561e85` on `feat/recovery-cleanup`, Darwin/arm64, Go 1.26.3 (2026-11-10). Read AGENTS.md, CLAUDE.md, approved spec/plan, prior phase evidence and historical closeout/fixture guidance. This unit changes documentation only: no production or runner correction was needed. Corrected current-status paragraphs in both recovery specifications; historical red/blocker/qualification records remain unchanged.

Independent review is a separate delegated unit, not claimed by this verification. This worker did not modify its semantic task status. During execution that task changed from pending to done externally; preserve that separately owned update rather than reverting it. Integration must reconcile the independent report before final closeout.

## Supported historical entry point and executed acceptance

One supported entry point: `efforts/old-host-compatible-recovery/fixture/build-production.sh`, with `checked-go-test.py`. From repository root, executed:

```sh
env -u CERCANO_NATIVE_BREW -u CERCANO_NATIVE_AGENT -u CERCANO_NATIVE_STATE_PARENT \
  OLD_RECOVERY_PRODUCTION_FIXTURE=1 OLD_RECOVERY_EXECUTE=1 \
  sh efforts/old-host-compatible-recovery/fixture/build-production.sh \
  > .phase6-evidence/historical.log 2>&1
```

Exit 0. Wrapper exports `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`, builds historical revision `d7cbdeb21c44e0ceeefdf212203ed04a37d09150` with `developmentfixture,cercano_development_acceptance` and loopback inference adaptation, and the untagged current destination from the candidate checkout. Historical binary SHA256 logged: `29d9f063943e7148431e6b760478125c33040aff882190660f17ba970d360b9b`.

The retained checked runner executes `go test ./cmd/cercano -run '^TestHistoricalProductionController$' -count=1 -json -timeout=180s` with explicit binary paths. It requires exactly these four intended names to run/pass (not package PASS alone): parent plus `/drain`, `/reserved`, `/blocked-timeout`. All four passed, **zero skips/failures**. Child times: 15.00s, 13.77s, 13.00s; parent 41.76s; package 42.348s; complete build/run command 55.738s.

Assertions actually exercised:

- Owned/drain and reserved: `native_controller_test.go:669–710` compares the complete recovered history prefix with `reflect.DeepEqual`, including all persisted fields. Drain additionally requires the two exact held-turn rows. The subsequent recovered turn preserves that complete prefix and adds **exactly two** rows, user `exact-recovered-provider-turn`, assistant `native-controller-ok`; exactly **one additional** provider request.
- `nativeControllerFinal` at lines 797–813 requires exactly one final response with exact `native-controller-ok`, with no stream errors. These are executed assertions, not inferred from log wording. Both successful cases logged the persisted recovered provider turn.
- Blocked-timeout: `historical_runtime_test.go:83–110` requires deadline exceeded and no recovery callback. Following unblock, cleanup/observation reacquires exclusion without replaying shutdown or changing ownership. It verifies owned host/worker absence, a competing historical launcher blocked by exclusion, and unchanged store/WAL.
- All **three** children explicitly logged `owned process cleanup complete (no signals)` from native controller line 277. Cleanup uses fixture-selected authenticated identities and an exact fixture-executable inventory; no residual owned fixture processes reported. No broad kill, live host mutation, user database or VM access was performed.

This is pinned historical shipping host/worker execution with synthetic inference, not the exact dirty incident executable, arbitrary SQL-writer fencing, real-model qualification, deployment, or live-conversation recovery. Runtime support remains same-user Darwin prepared-generation containment. The runner's normal disposable build-directory cleanup and successful test-state cleanup ran; no historical evidence was manually deleted.

## Focused normal/race and compilation

Exact orchestration retained locally at `.phase6-evidence/run-focused.sh`. All Go commands below exported `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`. From `source/server`, both normal and with `-race`:

```sh
go test ./internal/conversation ./internal/server ./internal/brewrestart ./cmd/cercano ./pkg/agentclient \
  -run '^Test(Resume|LiveSourceRecovery|LiveRecovery|ExactRecovery|RecoveryUncertain|Ownership|Handover|Lifecycle|Takeover|Attachment|ConversationAttachment|ForegroundAttachment|LocalAttachment|DialExisting|DevelopmentLegacyProductionAdapter|LegacyAuthenticatedShutdownWait|LegacyShutdownAuthenticatedTransport|DevelopmentRecovery|DeadHostKernelVerification|PreparedWriterAttributionBranches|PreparedGenerationInventory|NativeControllerAssertions)' \
  -count=1 -json -timeout=180s
go test -c -o "$ROOT/.phase6-evidence/resume-probe-normal" ./cmd/cercano
go test -race -c -o "$ROOT/.phase6-evidence/resume-probe-race" ./cmd/cercano
```

`ROOT` denotes the absolute repository root. Probe compilation is not behavioral coverage. The environment-driven subprocess bridge is intentionally not selected in the server command; the CLI test explicitly launches it with disposable state.

From `source/clients/cli`:

```sh
CERCANO_RESUME_PROBE_BINARY="$ROOT/.phase6-evidence/resume-probe-normal" \
  go test ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1 -json -timeout=180s
CERCANO_RESUME_PROBE_BINARY="$ROOT/.phase6-evidence/resume-probe-race" \
  go test -race ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1 -json -timeout=180s
```

Counts from actual JSON `run`/`pass` events (identical in each mode):

| Package | Top-level passes | All named passes including subtests |
| --- | ---: | ---: |
| conversation | 16 | 122 |
| server | 25 | 112 |
| brewrestart | 3 | 11 |
| cmd/cercano | 7 | 39 |
| agentclient | 13 | 18 |
| CLI ui | 109 | 140 |
| **Total per mode** | **173** | **442** |

Every package has nonzero coverage. **Zero skips/failures**, no race reports in either mode. CLI includes `TestDestinationResumeAfterActualProcessExit` using the supplied probe, not a skipped process bridge. Process-control coverage explicitly includes `TestDeadHostKernelVerification`, prepared writer attribution/inventory, authenticated shutdown, adapter and launch-exclusion tests; it is not inferred from a generic Recovery regex.

Interface compilation, separately in each module:

```sh
go test ./... -run '^$' -count=1
```

Both exit 0: server 137 package result lines, CLI 11. These intentionally execute zero behavioral tests and are interface/compile gates only. No full suite or cross-platform runtime qualification claimed.

From repository root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s efforts/old-host-compatible-recovery/fixture -p 'test_*.py' -v
sh -n efforts/old-host-compatible-recovery/fixture/build-production.sh efforts/old-host-compatible-recovery/fixture/run.sh
```

Runner regressions: **8/8 passed**, 2.788s; shell syntax passed. Includes zero-selection, unrelated pass, missing child, skipped parent/child and nonzero intended execution controls. Combined focused/compile/runner orchestration exited 0 in 118.536s.

## References, diff and preservation checks

Executed `git diff --check` and `git diff 76061bbe1382f2f53737e41ca39b2b45b9ed92a8 HEAD --check`: passed. `git diff 76061bbe1382f2f53737e41ca39b2b45b9ed92a8 HEAD -- source/proto source/server/pkg/proto` is empty: unchanged wire schema/generated bindings.

Recursive source `.go`/`.proto` reference search found no `reaffirmResumeWithSync`, `ReserveTakeover`, `resumeConfirmedLegacy`, or `LegacyProcessStop`. `ContainedSourceStop` deliberately remains active, with `WithStoppedSource` as the production continuation. `ConfirmLegacyStop` remains in the wire/refusal/tests, not an executable legacy stop path; `LifecycleOriginalRecovery` / `ConfirmOriginalRecovery` remain transport names mapped to internal contained recovery (CLI `contained_recovery.go:14`). Wire migration remains deferred.

Inspected baseline-to-candidate name/status diff: deletions are the approved obsolete takeover primitive/test, legacy continuation and obsolete forced-resume test; renamed files are tracked as renames. No unrelated evidence deletion. Compared both archived fixture originals byte-for-byte against baseline via `git show`: identical. Existing historical verification and untracked artifacts preserved.

## Limits, local artifacts and remaining integration

Local untracked `.phase6-evidence/` (176 MiB at inspection) retains acceptance JSON/output, normal/race JSON, compile logs, runner regression log, exact orchestration script, two executable test probes, reference search, name/status diff, empty wire diff and baseline SHA. These are deliberately not staged or deleted. Successful historical fixture temporary stores/builds follow existing self-cleanup; failure preservation was not needed. No blocked behavioral checks or production changes resulted.

Two tooling-only unsuccessful attempts are not test failures: initial argv mistakenly supplied arguments to `pwd`; local Bash 3.2 rejected an empty array under `set -u` before any focused test ran (retained `harness-initial-error.log`, corrected only the untracked harness). `rg` was absent; the reference search was rerun successfully with recursive `grep`. No intended test result was discarded.

Scoped checkpoint includes this record, corrected status specifications and semantic plan updates only. Independent-review status changes are separately owned; this worker neither performed nor certified that review. Remaining step: coordinator reconcile independent review and report final branch/checkpoint status, retaining wire/platform/incident-binary limitations. No merge, push, deployment or live recovery is authorized or performed.
