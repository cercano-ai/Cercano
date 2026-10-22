# Bounded durable health reconciliation

## Contract

`activationtxn.ReconcileHealth` consumes an existing `JournalHealthVerified`
checkpoint under the directory-bound exclusive guard. It validates the current
operation/journal identity and target, exact selection, and persisted manifest
and executable image facts using the existing verification helpers. Both initial
calls and matching duplicate retries must pass these checks.

`state.Store.ReconcileActivationHealth` then rechecks current operation identity,
revision, target, lifecycle and both recovery flags, plus journal revision,
immutable facts and health-verified checkpoint, in **one write transaction**.
It restores the existing operation model, applies `EventHealthSuccess`, and uses
the adapter's connection-scoped writer. The operation becomes `StateCleanup`
with `HealthVerified=true`; the journal and its revision are unchanged.

A matching already-reconciled cleanup operation returns
`AlreadyReconciled=true` without a write. A stale revision still fails CAS, even
if another caller just reconciled; callers reload and retry through the guarded
entry point. Neither result is a fresh candidate health observation or a promise
of continuing liveness. A journal at a later checkpoint is not accepted as an
idempotent match.

## Verification (macOS host)

Passed:

- `go test -race ./internal/updatecoord/state ./internal/updatecoord/activationtxn -count=1`
- `go vet ./internal/updatecoord/state ./internal/updatecoord/activationtxn`

Tests use disposable SQLite installations and inert image fixtures. Coverage:
acknowledgement followed by close/reopen before reconciliation, duplicate retries,
operation and journal revision rejection, deterministic interleavings through a
second store handle (operation update, recovery request, competing reconcile),
journal advancement, recovery-needed/requested flags, lifecycle/identity/facts
mismatches, injected operation write failure with unchanged state after reopen,
selection/policy/manifest/image mismatch, wrong-directory and closed guards, and
selection/manifest rejection even after prior reconciliation.

## Limits

No schema changes, candidate launch/adoption/stop, cleanup execution, rollback,
overall completion, live installation access, push or merge. This is a bounded
primitive, not startup orchestration or end-to-end update acceptance. Cooperative
installation exclusion and trusted packaging policy remain required. The
unresolved startup-safety strategy is untouched. No native Windows/Linux run or
hard-kill/power-loss test is claimed; interruption is exercised by database
close/reopen at the durable acknowledgement boundary. Review was local under the
bounded no-delegation constraint.
