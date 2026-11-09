# Explicit exact-transfer retry recovery

This implements the retry-only boundary missing from
[the offline characterization](exact-transfer-recovery-characterization.md).
It does not update running hosts or authorize live recovery.

## API

`agentclient.(*LifecycleControls).RetryTransfer(ctx, destination, originalRequest)`
is the narrow callable entrypoint. Both controls must already be authenticated to
and pinned to the original source and destination identities. No discovery or
new CLI recovery policy is introduced. The request must pin management mode,
conversation, source owner host/epoch, source and destination identities, original
handover ID and **nonempty original receipt**. Target is the source; the client
clones the request to target the destination without changing the original.

The new RPCs are `LifecycleRetryReceipt`, `LifecycleRetryRelinquish` and
`LifecycleRetryClaim`. They use existing authenticated loopback controls,
lifecycle locks, acceptance records, source closure and store authority contracts.
Missing state is not repaired or created. Unsupported servers return
`Unimplemented`; neither client nor source falls back to ordinary mutating RPCs.
The client checks destination capability before invoking the source.

The source requires its exact retained original closure before contacting the
peer, then rechecks that closure and source owner under its lock. It cannot start
a reservation or fence a new owner. Pending persistence remains fenced; uncertain
writes and unresolved commit/cleanup remain blocking. The destination checks its
exact existing acceptance under the lifecycle lock and uses only the existing
source-qualified store handover-claim path, whose transaction requires the reserved
conversation, destination, original handover ID and original source host/epoch.
It never takes the unqualified acquisition branch. Unresolved local initialization
cannot be reported as completion.

The existing ownership row retains source host/epoch while reserved, but the
existing completed-handover schema does not retain them. After a successful
source-qualified commit, the destination records its epoch on the existing
in-process acceptance authority (already bound to the full authenticated source
identity and receipt). Completed retries require that verified epoch to match
the durable result. Acceptance alone is never completion provenance. Unknown
historic provenance, including claims made outside this qualified path, refuses
rather than inferring a source. No schema migration is introduced; supporting
restart-persistent provenance would require a separately approved schema change.

A completed claim returns the recorded epoch idempotently, including after clean
release. `LifecycleControlResponse.current_owner` distinguishes ownership at the
observation from a historical result; it is not a lease or promise of future
ownership. No recovery journal is required, created, copied or retargeted. No new
move ID, Prepare/AcceptIncoming, normal Resume/Acquire/Claim/Relinquish, force
fallback, or manual state clearing is part of this entrypoint. A normal
`LifecycleClaim` now also rejects `Destination != Target`.

## Uncertainty and limitations

Deadline/cancellation is an **uncertain outcome**, not permission to start a new
move: retain the unchanged request and retry only this operation. A lost response
can follow a committed transition. Existing in-memory closure and acceptance
state must still exist; restart loss is not repaired by this API.

A remote receipt check is **not distributed atomicity**. Acceptance can disappear
after source validation; the source may commit its original durable reservation
while destination claim rejects. Local transitions fail closed, but this does not
supply cross-host rollback, a distributed lock, or automatic convergence.
Authentication remains the caller/dialer's existing host-pinning responsibility.
Fixture authentication does not verify deployed credentials or older binaries.

## Offline verification

Smallest regression evidence: `TestExactRecoveryDestination/destination-field-gap`
failed against characterization checkpoint `adde5e59c6be2fed92ede3d27c28a615a6938513`
because the original Claim accepted an unrelated destination. The same regression
now passes. The new retry tests initially encountered `Unimplemented`; an added
unresolved-destination regression also exposed a successful retry after reload
failure, fixed by checking local readiness under the same lifecycle lock.

Selected unit and authenticated bufconn integration tests, both normal and race:

```sh
go test -json ./internal/server ./pkg/agentclient -run '^(TestRetryTransfer|TestExactRecovery|TestResumeNewDestinationInterruptedTransfer)' -count=1
go test -race -json ./internal/server ./pkg/agentclient -run '^(TestRetryTransfer|TestExactRecovery|TestResumeNewDestinationInterruptedTransfer)' -count=1
```

Each run passed 15 top-level tests and 45 subtests (51 leaf cases); eight top-level
tests exercise the retry implementation. Cases include absent source reservation,
missing/mismatched identities and state, wrong target/destination, acceptance
loss, source ownership changes after preflight, pending and unresolved writes,
timeout followed by exact retry, completed/historical ownership, and unsupported
servers with no fallback. Rejections snapshot lifecycle/store state, including
journal counts, to detect unintended mutation. These are isolated fixture tests,
not live recovery or production compatibility certification.

### Source-provenance review regression

Before the provenance fix, `TestRetryTransferClaimProvenance` failed in all three
initial cases (`reserved`, `completed`, `historic`): a second preexisting,
authenticated acceptance differing only in source epoch consumed the reservation
or returned the original claim result. It now rejects without state changes;
the original receipt still succeeds idempotently and distinguishes current from
historic ownership. An additional `unknown-historic` case refuses even the
original receipt when retained verified provenance is absent.
`TestOwnershipSourceQualifiedClaim` covers host/epoch mismatches at the
transactional boundary and unchanged reservations on rejection.

Bounded offline normal and race runs both passed:

```sh
GOPROXY=off GOSUMDB=off go test ./internal/conversation ./internal/server ./pkg/agentclient -run '^(TestOwnership|TestRetryTransfer|TestExactRecovery|TestResumeNewDestinationInterruptedTransfer|TestCoexistence)' -count=1
GOPROXY=off GOSUMDB=off go test -race ./internal/conversation ./internal/server ./pkg/agentclient -run '^(TestOwnership|TestRetryTransfer|TestExactRecovery|TestResumeNewDestinationInterruptedTransfer|TestCoexistence)' -count=1
```
