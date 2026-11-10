# Bounded Phase 2/3 evidence closeout

Base: 9a879b345ee8. Darwin native tests, not live recovery. The only production changes are per-call dependency seams: prepared inventory dependencies and dead-owner transaction Commit. Defaults retain existing behavior; no global hooks, new persistence semantics, schema changes or weakened checks.

## New executed tests

- `brewrestart/writers_attribution_darwin_test.go:TestPreparedWriterAttributionBranches`: argv denied + unrelated fallback accepted; matching fallback refused; both denied refused; initial ESRCH skipped without fallback; fallback ESRCH conservatively refused; empty/relative identity and malformed successful argv identity refused. This tests native decision logic deterministically, while existing PreparedGenerationInventory exercises kernel inventory and an unlinked surviving generation process.
- `conversation/dead_owner_boundary_test.go:TestDeadOwnerUncertainCommitReconciliation`: inject error after actual Commit and after Rollback at the existing transaction boundary. Both return an unusable capability retaining root AND descendant locks, reject writes and concurrent recovery. Explicit durable inspection followed by capability Close and retry produces exactly one receipt/epoch: committed retry refuses stale source, rolled-back retry completes once. No automatic resolution or invented data-loss policy.
- `server/recovery_uncertain_test.go:TestRecoveryUncertainOutcomeNeverReady`: separately exercises coordinator capability-plus-error contract. It retains capability, never marks ready and two RPC retries cannot publish Ready or acquire another epoch. This is composition with the store test, not a claim of a SQL fault injected through RPC; coordinator fixture deliberately uses a successfully committed capability plus an acknowledgement error.
- `conversation/dead_owner_boundary_test.go:TestDeadOwnerNotificationDoesNotReplayAction`: abandoned running task becomes interrupted; completed JSON results survive, prior completion claim survives, pending evidence survives; StartTask rejects every terminal task, duplicate FinishTask cannot overwrite, automatic wake claim cannot be reacquired. Explicit acknowledgement consumes notification only. This proves persisted task admission/non-reexecution, not rollback of arbitrary external effects.

## Existing matrix mapped, not duplicated

Phase 2:
- Unrelated executable exclusion: new attribution table; PreparedGenerationInventory (unrelated database opener outside trust boundary); DeadHostKernelVerification (different host accepted). Process-stop nonmutation: UpgradeMultipleActiveHosts / UpgradeHostWithUnrelatedRecognizedWorker. Relaunch exclusion: DevelopmentRecoveryRegistryExclusion holds native launch lock until release. Historical competing launcher and blocked-timeout refusal remain the actual-old-process evidence in final-verification.md (not rerun here).
- Cancellation: LaunchLockContentionCancellation; PreparedShutdownOwnerRetainsAfterCancellation; UpgradeCancellationAfterTERMFinishesCleanup. PID reuse: SameProcessDetectsPIDReuse. Surviving writers: PreparedGenerationInventory refuses generation process even after unlink; LiveRecoveryRefusesUnqualifiedWriters refuses before recovery callback. These are layered boundary tests, not a single exhaustive cross-product native end-to-end simulation.
- Consent: LiveRecoveryConsentMutationBeforeStop covers **12 cases**: confirmation and final-pre-stop crossed with epoch, reservation destination, handover, endpoint, token, source. Each requires error, zero stops and no destination ownership. LiveSourceRecovery covers owned/reserved recovery and refusals. Timeout: ExactRecoveryTimeoutAndExactRetry plus historical blocked-timeout (previous run).

Phase 3:
- DeadOwnerRecovery preserves committed history, reconciles descendants, leaves unrelated task/ownership usable. DeadOwnerRecoveryRejectsStaleAndReserved covers stale epoch, foreign descendant and held descendant lock. ReservedDeadOwnerRecovery serializes a competing late claim and rejects duplicate recovery/late claim after success; ReservedRecoveryRollbackAndStaleConsent prevents partial reconciliation.
- LiveSourceRecovery covers owned/reserved lost successful response, repeated ready resumes, stable operation ID and epoch, only one stop, and rejection of receipt metadata after new acquisition. ExactRecoveryTimeoutAndExactRetry covers timeout identity on the existing exact-recovery route. New uncertain boundary tests close the commit-error/readiness gap, without treating an uncertain transaction as ready.

## Completion notifications are not action replay

Recovery sets wake_claimed while preserving notification_pending and terminal result JSON. This suppresses automatic completion wakes, not durable evidence. Existing `TestClaimedCompletionIsRecoveredAtHumanTurnBoundary` intentionally runs a human turn followed by a system notification turn carrying retained child outcome, then acknowledges it. `TestDisconnectedCompletionWaitsForReconnectedApproval` covers approval on reconnect; completion coalescing/admission tests retain evidence under busy/denied admission. A legitimate later user-triggered notification/model turn is not restart of the interrupted delegated action. Neither recovery nor these tests promise that arbitrary future user-authorized model turns cannot choose new external actions.

## Commands actually run

From source/server with GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off, normal and `-race`, all PASS (nonzero behavioral selections):

```
go test [ -race ] ./internal/conversation ./internal/brewrestart ./internal/server ./cmd/cercano -run 'Test.*(DeadOwner|DeadHost|ReservedRecovery|Prepared|LiveRecovery|LiveSourceRecovery|Recovery|SameProcessDetectsPIDReuse|Completion|ClaimedCompletion)' -count=1
go test [ -race ] ./cmd/cercano ./internal/brewrestart -run 'Test(LaunchLockContentionCancellation|PreparedShutdown|UpgradeCancellationAfterTERMFinishesCleanup|UpgradeMultipleActiveHosts|UpgradeHostWithUnrelatedRecognizedWorker|SameProcessDetectsPIDReuse)' -count=1
```

Phase 2/3 semantic checklist marked done on this bounded scenario evidence, not universal qualification. Historical executable acceptance remains prior evidence; no rebuild of exact dirty live binary, universal SQL/file-descriptor fence, non-Darwin runtime or VM claim. Broader independent review/platform/Phase 4 work remains separate. No live hosts/config changed, push or merge. `.tmp-old-recovery/` preserved.
