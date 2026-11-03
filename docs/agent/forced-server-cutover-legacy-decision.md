# Historical forced-cutover decision boundary

> Historical investigation of a **live** legacy source. Its whole-host maintenance
> question does not apply when the source has exited. The subsequent
> [dead-owner recovery implementation](dead-owner-recovery.md) handles that case
> without shutdown or source RPCs. The report below is retained as history.

2026-11-02. Investigated from `fix/forced-server-cutover` at
`8af13b38ca86947bccfa2898c3a37a878463df6a`; historical source inspected at its
parent `5116c0f2`. **The product requirement remains incomplete.** No legacy
fallback, shutdown, ownership recovery or live deployment was implemented.

## Reproduction

`TestForcedTransferHistoricalServiceMissingRPC` registers a disposable in-memory
gRPC service with the forced method omitted from its descriptor. The real client
receives `Unimplemented: unknown method LifecycleForceRelinquish for service
agent.Agent`; the destination is not claimed. This reproduces the historical
wire contract, not execution of an archived historical binary. The existing
fake-client test separately asserts no cooperative-protocol fallback.

## Why the existing primitives do not suffice

* Historical `internal/server/lifecycle_controls.go:131-181` (at `5116c0f2`)
  uses `reserveRelinquishLocked`, not forced cancellation. Historical
  `internal/server/coexistence.go:295-321` closes the owner/terminal through
  admission and checks existing safety constraints. It cannot retrofit the new
  scoped cancellation and producer-drain protocol into a running executable.
* Historical `internal/server/server.go:2394-2423` implements host shutdown:
  either the host-wide shutdown controller or scheduled self-SIGTERM. The
  response may only acknowledge scheduling. Current `server.go:607-629` shows
  the cleanup scope: task manager, worker pool and runtime instances, not one
  conversation. That is not preservation of unrelated conversations.
* `internal/brewrestart/coordinator_darwin.go:221-237` distinguishes disappearance
  or PID reuse from same-lifetime exec. It is useful process-exit evidence, not
  conversation ownership or evidence that every independent writer/child has
  stopped. Endpoint failure is not exit proof.
* `internal/conversation/ownership.go:53-55,118-123,231-233` explicitly refuses
  reacquisition of abandoned `owned` or unmatched `reserved` records, even with
  an available OS lock. Process exit alone cannot authorize destination acquire.
* `internal/conversation/offline_upgrade.go:8-18,52` requires all historical
  writers stopped (and a backup for interrupted recovery), and does not reset
  ownership on an already marked database. `tasks.go:171-176` forbids global
  reconciliation in coexistence mode; legacy reconciliation marks unfinished
  work interrupted, not completed. Neither is a scoped dead-owner recovery API.

All source paths above are relative to `source/server/`.

## Exact decision required before implementation

May a separately confirmed legacy maintenance operation stop **the entire source
host**, interrupting unrelated conversations, and under what approved durable
reconciliation/recovery policy may an abandoned owned/reserved epoch be replaced?
The current conversation-specific force confirmation approves neither. Do not
implement that path without a decision. No lock deletion, failed-dial adoption,
completion fabrication or reuse of offline migration as ownership recovery is
permitted.

Viable alternatives:

1. Preserve unrelated conversations and fail closed for historical active owners.
   Use cooperative relinquishment only when the historical host can establish
   its existing quiescence conditions; deploy a supporting host through a
   separately coordinated clean maintenance transition for future forced moves.
   This is not completion of arbitrary historical forced cutover.
2. Explicitly approve a broader maintenance workflow with all affected work
   disclosed, exact writer/descendant fencing evidence, backup, truthful
   interrupted/uncertain-work reconciliation, and a separately designed audited
   epoch-recovery policy for marked stores. Existing process verification and
   offline-unmarked migration are components, not that policy.

## Actual checks

Passed race-enabled focused tests with disposable HOME/config/TMPDIR under
`/tmp/fcl.DKqywH`, `CERCANO_DISABLE_CREDENTIALS=1`, cached modules/build artifacts,
`GOPROXY=off`, `GOSUMDB=off`, and `/opt/homebrew/bin/go`:

* agentclient: `TestForcedTransfer*`, `TestShutdownReplacementGate`,
  `TestShutdownStatusDoesNotTreatAcceptanceAsCompletion` (`client.log`).
* conversation: `TestOwnershipTransitionsReservationAndAbandonment`,
  `TestOffline*` (`store.log`).
* brewrestart: `TestUpgradeGraceLifetime` (unchanged, exec, reused, gone;
  `lifetime.log`).

No live state/processes/config/credentials were accessed by these probes. No
installation, push or landing occurred. Only the regression and this decision
record are checkpointed; no unsupported transition is represented as solved.
