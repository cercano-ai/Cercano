# Close/resume regression coverage (review candidate)

## Coverage matrix

| Scenario | Exercise | Boundaries/substitutions and limitations |
|---|---|---|
| Completion, execution error, cancellation with outstanding read-only history | New `TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure` | Real gRPC/bufconn, production `workerRunner.RunTurn`, history callback and callback join. Scripted worker requests history and terminates; instrumented authority observes cleanup error and join order. No model/subprocess or durable authority in this test. Channel barriers, five-second deadline, joined goroutines. |
| Healthy idle teardown versus unhealthy eviction, zero writes | New `TestAccountingEvictionVersusExplicitDrain` | Real WorkerServer Accounting handshake, host accounting session, connection and pool Release/reapIdle. Healthy retirement callback uses bounded session close; drain-first control does likewise. Process handle has no subprocess. Unhealthy eviction **still reports incomplete**; this is characterization of the unresolved behavior, not a fix. |
| Intentional busy close, failed persistence, durable release/reopen across generations | Extended `TestSessionCloseBusyDrain` | Existing real lifecycle release handler, admission accounting, disposable same SQLite store and distinct host/build generations. Added exact durable owner/epoch checks during draining and after reopen, fresh epoch, old-owner attachment rejection on both hosts, fresh attachment success. Authorization/reload/participation callbacks are fixture substitutions; not installed rebuild/launcher coverage. |
| Idle close, transport disconnect, release failure and genuine write/cleanup uncertainty | Existing `TestSessionClose*`, `TestSessionDisconnect*`, `TestWorkerPersistenceTerminalDrain` | Reused rather than duplicated. Focused race run passed; persistent mutation/cleanup failure cases remain blockers. |
| Handover and fenced reconnect | Existing `TestCoexistenceLifecycleHandover`, `TestResumeFencedReconnect` | Disposable generations/store; focused race run passed. Does not exercise credential replacement or installed launcher. |

## Reproduction and narrow fix

With HEAD's original `host.go`, the new history test fails in all three modes:
`worker callback cleanup failed: EOF`. Production safeSend was classifying response
transport failure as durable callback cleanup uncertainty. It now leaves response
transport failures to the turn receive path; service write/persistence accounting
is unchanged. The callbacks are still canceled and joined before authority close.
No auth, credential, ownership, attestation or shutdown-budget changes.

The accounting test reproduces `incomplete=true` on unhealthy Release with zero
writes; normal idle retirement and explicit drain-first produce `false`. Do not
clear this uncertainty merely because the host saw no writes: that is not proof
that the disconnected producer had no outstanding observations.

### Accounting decision deferred

`TestUnhealthyReleaseKeepsImmediateKillSemantics` explicitly requires immediate
kill and forbids accounting-retirement delay on canceled/unhealthy workers.
Changing Release to synchronously drain changes that contract (up to the existing
four-second budget). Alternatively producer-stop/transport ownership could be
split so turn cancellation remains immediate while accounting drains separately;
that requires a larger lifecycle design. Neither choice is made here. Explicit
bounded drain remains the safe control; no unbounded shutdown wait was added.

## Validation

From `source/server`:

- Original host.go: `go test ./internal/worker -run '^TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure$' -count=1` — failed all three modes as above; fixed file restored afterward.
- `go test -race ./internal/worker -run 'TestAccountingEvictionVersusExplicitDrain|TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure' -count=20` — passed.
- `go test ./internal/worker -count=1` — passed.
- `go test -race ./internal/worker -run 'TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure|Test.*Accounting|TestUnhealthyRelease' -count=10` — passed (before adding eviction characterization).
- `go test -race ./internal/server -run 'TestWorkerPersistenceTerminalDrain|TestSessionClose|TestSessionDisconnect|TestCoexistenceLifecycleHandover|TestResumeFencedReconnect' -count=5` — passed.

Remaining gaps: combined real history transport + durable prepared authority
release/reopen in one fixture; production spawned-worker handshake/process
cleanup; CLI timeout/installed launcher rebuild; broad full-server suite. No live
host/config/database changes or real user process signaling. Ready for independent
main review; not independently reviewed or landed.
