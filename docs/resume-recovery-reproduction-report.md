# Resume/recovery reproductions

Base inspected: main `5351fc14e80c339253b35bf814b57489be170e89` (the audit base is still current). Test-only branch `test/resume-recovery-reproductions`. No production edits.

## Predictions established before execution

1. Correct: contained recovery resolves the superseded exact incoming acceptance, allowing retirement after release. Predicted: confirmation calls `LifecycleRecover` directly (`live_recovery.go:87`), bypassing `resumeRecover` cleanup (`resume.go:490`), leaving incoming participation.
2. Correct: a later foreign epoch can negotiate a fresh operation after successful recovery. Predicted: journal replacement requires the old cooperative `HandoverResult`, which recovery does not produce (`resume.go:254`); later ownership triggers stale-operation refusal.
3. Correct: reconnect cannot advertise usable readiness while cutover keeps admission fenced. Predicted: `LifecycleAcquire` returns the cached ready owner and `LifecycleResume` sets Ready/CurrentOwner despite admission closure. Acquire itself does **not** set the wire Ready flag.
4. Correct: an unrelated/canceled request is independent of a stuck peer preflight. Predicted: lifecycle-wide `resumeMu` surrounds a `LifecycleList` call with no internally imposed deadline; even a canceled unrelated request parks on that mutex.

## Observed (all four reproduced)

Tests: `source/server/internal/server/resume_recovery_reproduction_test.go`.

- `TestReproResumeRecoverySupersession` (line 26) covers 1 and 2. Real Retain on A; existing fake peer refuses force with admission-busy after B has journaled/accepted; existing contained-stop fixture closes A's real capability lock during confirmed termination. Recovery returns ready, creates a recovery receipt, but the **same acceptance object**, matching journal source/epoch/operation/receipt, remains. After real ReleaseRetained on B: `incoming=1`, `owners=0`, retirement error `retained sessions or work prevent transition`. Old cooperative result is `sql.ErrNoRows`. Real Retain on C establishes another epoch; B resume returns `FailedPrecondition: resume owner changed; refusing stale operation`, leaving C's owner/epoch unchanged.
- `TestReproResumeFencedReconnect` (line 113): a real admitted persistence ticket reports PersistenceFailed. Forced cutover returns `FailedPrecondition` with `1 unresolved persistence outcomes`, retaining its fence. Acquire succeeds with the same epoch; resume returns `ready=true current_owner=true`; foreground admission returns `admission closed`.
- `TestReproResumeGlobalPreflightLock` (line 176): a channel-controlled fake peer holds the real preflight RPC. Its server context has no deadline. A separate, already-canceled request for `unrelated` is observed parked in `sync.(*Mutex).Lock` inside LifecycleResume. TryLock confirms the lifecycle mutex is held. Runtime stack observation plus channel entry avoids sleep/negative-time assertions. Cancellation releases the RPC; both resume goroutines are joined.

These intentionally assert defective behavior so the reproduction checkpoint is green. They are not desired-invariant regression tests.

## Commands/results

Run from worktree `source/server` after final edits:

```text
gofmt -w internal/server/resume_recovery_reproduction_test.go
go test ./internal/server -run '^TestReproResume' -count=20 -timeout=120s
ok cercano/source/server/internal/server 1.661s

go test -race ./internal/server -run '^TestReproResume' -count=5 -timeout=180s
ok cercano/source/server/internal/server 2.719s

go test ./internal/server -run 'Test(Coexistence|Lifecycle|DestinationResume|Resume|LiveSourceRecovery|LiveRecovery|ForcedCutover|ExactRecovery|Recovery)' -count=1 -timeout=180s
ok cercano/source/server/internal/server 7.097s
```

An initial harness run failed because direct control calls lacked loopback peer context (two PermissionDenied errors and a preflight watchdog); adding the existing fixture-style loopback context corrected the harness. Subsequent verbose, repeated and race runs passed. No observed hypothesis was left unreproduced.

## Limits/safety

Temporary SQLite stores only; local disposable gRPC fixtures; no live configuration, database, process inspection or signaling. The contained-stop fixture models termination by closing the real source ownership lock, not by inventing database rows. Persistence uncertainty uses the real admission ticket failure API; it does not simulate an actual filesystem fault. No crash/restart or actual OS containment behavior is tested. The queue probe combines unrelated conversation and pre-canceled context in one queued request; it does not measure production latency or separately test cancellation arriving after queueing. Stack symbol matching is Go-runtime-sensitive; five-second waits are failure watchdogs, not behavioral timing thresholds. The preflight is observed without a deadline, not literally run forever. Neighboring lifecycle tests were run, not the entire repository suite. No production fixes, landing, or push.
