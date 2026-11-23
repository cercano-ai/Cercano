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

### Ordinary teardown remains incomplete (not ready to land)

`TestUnhealthyReleaseKeepsImmediateKillSemantics` only protects against invoking
idle retirement on unhealthy release; it is not an external prohibition on a
separate bounded ordinary-teardown path. The approved four-second accounting
budget can be used for eligible teardown without adding a CLI timeout.

The unresolved implementation choice is how to carry explicit lifecycle evidence
from RunTurn to Release: Release currently receives only `healthy`, conflating
local cancellation, pre-start failure, missing TurnDone/EOF, transport failure,
and protocol rejection. `workerHealthy` checks process existence and connection
state, not producer completion or accounting responsiveness. In particular a
Ready connection after local cancellation does not prove that the canceled
producer stopped. A safe new path must distinguish ordinary cancellation from
unsafe transport/protocol failure, remove the handle from reuse first, and retain
uncertainty unless the accounting handshake finishes within its existing budget.
Displaced handles and stale Acquire eviction need the same classification;
calling the idle hook unconditionally is not a fix. This revision does not yet
implement that path or its classification tests.

### Callback/transport cleanup ordering follow-up

The previous defer ordering ran transport/pool cleanup before canceling and
joining callbacks. Cleanup is now part of the callback cleanup defer: cancel,
join, then transport/pool cleanup, followed by the earlier registered authority
Close. The real gRPC history fixture asserts the connection remains open at the
callback cancellation barrier and is closed after callback join. Before this
change that assertion failed in completion and execution-error modes.

This reorders the existing callback wait; it does not add another wait or timeout.
The existing noncooperative-callback limitation remains: a callback ignoring
cancellation can block the join, and now also delays transport cleanup. Therefore
this fix alone is not evidence of bounded end-to-end ordinary teardown or unsafe
worker containment. A bounded callback policy cannot simply abandon the callback
and close its authority, because an in-flight mutation may still use that authority.

## Validation

Follow-up validation from `source/server` (this revision):

- Added connection-lifetime assertion before fixing defer ordering: `go test ./internal/worker -run '^TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure$' -count=1` — failed completion and execution-error cases with `transport destroyed before callback join`.
- `go test -race ./internal/worker -run 'TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure|TestAccountingEvictionVersusExplicitDrain' -count=20` — passed.
- `go test ./internal/worker -count=1` — passed.

Earlier revision evidence from `source/server` (not rerun unless listed above):

- Original host.go: `go test ./internal/worker -run '^TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure$' -count=1` — failed all three modes as above; fixed file restored afterward.
- `go test -race ./internal/worker -run 'TestAccountingEvictionVersusExplicitDrain|TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure' -count=20` — passed.
- `go test ./internal/worker -count=1` — passed.
- `go test -race ./internal/worker -run 'TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure|Test.*Accounting|TestUnhealthyRelease' -count=10` — passed (before adding eviction characterization).
- `go test -race ./internal/server -run 'TestWorkerPersistenceTerminalDrain|TestSessionClose|TestSessionDisconnect|TestCoexistenceLifecycleHandover|TestResumeFencedReconnect' -count=5` — passed.

Remaining gaps: production prepared-authority/disposable-store mutation failures
(session model, task insertion, autonomy ledger, dispatch evidence) coincident
with response EOF/cancel, successful-write lost-response and read/validation
controls; ordinary teardown classification/drain; combined real history transport
+ durable prepared authority release/reopen; disposable Unix subprocess close,
exit/socket cleanup and generation replacement fixture; CLI timeout/installed
launcher rebuild; broad full-server suite. No live host/config/database changes
or real user process signaling. Partial follow-up only, not ready to land.
