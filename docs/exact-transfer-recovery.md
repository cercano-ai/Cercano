# Recovery protocol: current replacement scope

Normal reopen uses `LifecycleResume`: read current durable ownership, try supported
cooperative controls first, and offer contained-source recovery when cooperation
cannot safely complete. After explicit single-use, host-wide interruption consent,
revalidate source process identity, source epoch, selected destination and ownership;
contain/stop the source and use destination-authorized `LifecycleRecover`. Refuse
uncertain containment or changed ownership. Never replay interrupted external actions.

Retained-work refusal or a three-second cooperative deadline offers separate
**Terminate the old host and recover this conversation** consent. This interrupts
all host conversations/tasks and loses unpersisted output; affected scope is
unknown when inventory is unavailable. Decline leaves ownership unchanged.
The authenticated native adapter supports older hosts without a shutdown RPC,
using verified TERM/bounded wait/KILL followed by existing guarded recovery,
never termination on authentication/identity mismatch. See
[forced resumption](agent/forced-server-cutover.md) for limits and warnings.

The modern `LifecycleRetryReceipt`, `LifecycleRetryRelinquish`, `LifecycleRetryClaim`
and all `LifecycleTakeover*` RPCs, their handlers, retry client wrapper, and
original-operation discovery/confirmation coordinator have been deleted. No journal
count or original-destination discovery gate authorizes contained recovery. Ordinary
cooperative retries still use durable operation journals, receipts, source fencing
and exact claim provenance; journals are not authority to steal a current epoch.

Preserved controls: Resume, Prepare, ValidateReceipt, Relinquish, ForceRelinquish,
Claim and Recover, plus ordinary Acquire/Release/List/Retire. Legacy whole-host
stop protection and its CLI confirmation remain necessary for older ordinary paths.
Terminal-attachment `TakeoverConversation` is separate from lifecycle ownership and
is retained. See [safe takeover scope](safe-takeover.md).

`LifecycleOriginalRecovery` / `original_recovery` and `confirm_original_recovery`
are explicitly retained **wire-envelope names only**, used by `live_recovery.go` for
contained recovery. There is no parallel original-operation handler. The envelope's
legacy/takeover booleans remain unsupported rejection sentinels, not modes.
Request field 16 and `confirm_legacy_recovery` are reserved; no removed wire number
or name is reused. Protobuf service methods have no numeric IDs or `reserved`
syntax: the removed Retry/Takeover method names must not be reintroduced with a
new meaning. Go bindings were regenerated using `source/proto/generate.sh`.

## Bounded deletion verification (2026-11-09)

Passed normal and race tests for server conversation/broker/server/agentclient with
`Ownership|Recovery|Resume|Handover|Claim|Journal|Acceptance|Relinquish|Takeover`,
and CLI UI with `Recovery|Resume|Reopen|Takeover|Ownership` (all `-count=1`).
Both server and CLI passed `go test ./... -run '^$'` module compilation.
The interrupted-transfer test retains uncertain persistence, exact journal retry,
source fencing, current owner/epoch and stale competing claim checks; it no longer
expects the removed discovery gate to prohibit creating a competing journal.
No full cmd runtime suite, live host/config/VM operation, or deployment was performed.

---

## Historical implementation and verification (superseded)

The following records the deleted design and its historical verification. It is not
current API documentation or evidence that the deleted RPCs remain available.

### Explicit exact-transfer retry recovery

This implements the retry-only boundary missing from
[the offline characterization](exact-transfer-recovery-characterization.md).
It does not update running hosts or authorize live recovery.

## API

`agentclient.(*LifecycleControls).RetryTransfer(ctx, destination, originalRequest)`
is the narrow retry-only callable entrypoint. Both controls must already be
authenticated to and pinned to the original source and destination identities.
The coordinator/CLI discovery policy described below wraps the original-operation
journey; it does not broaden this retry-only API. The request must pin management mode,
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

## Coordinator and CLI original-operation discovery

Before minting a new transfer, the selected coordinator examines private original
operation journals. These are **hints, not authority**: journal endpoints and
credentials are not trusted routes. Trusted private peer records select routes and
CLI credentials, and authenticated, identity-pinned peers must validate the exact
original destination acceptance and receipt. Missing authority, ambiguous or
changed operations, unavailable peers, and pending or uncertain writes block
recovery; they do not authorize a competing move, new acceptance, or fallback to
a different operation or destination.

The CLI presents the original source, epoch, operation and destination, explicitly
warning that recovery can fence the source and cancel conversation work. The newly
selected host does not receive the original transfer. Recovery requires fresh,
explicit, single-use coordinator-local consent, bound to the requester and exact
operation; offers expire and do not survive restart. Decline, Enter, Escape and
stale UI callbacks do not authorize recovery. The coordinator revalidates peers,
selection and operation after confirmation.

Modern recovery uses retry-only controls and requires an existing source
reservation. Unsupported retry handlers require a **separate legacy confirmation**,
not silent fallback from modern consent. Explicit legacy consent permits ordinary
controls to initiate or finish the **ORIGINAL** transfer, including creating its
missing source reservation. This is deliberately not an existing-reservation-only
retry. It cannot create a new destination acceptance or invent a competing move.

Before source mutation, the coordinator reaffirms the original journal in place,
without replacing it: the exact operation, private regular-file mode and inode
must match before and after syncing the file, parent and publication directory.
A failed durability barrier blocks source calls and retains the exact operation
for retry; a concurrent journal replacement is rejected, not overwritten.

A successful modern claim must report `current_owner`; both modern and legacy
paths also require the exact durable handover result and matching current durable
owner/state/epoch. A historical claim, clean release, fresh epoch or subsequent
movement cannot be presented as current recovery success. This is a **current-owner
snapshot, not a lease**; subsequent work still requires normal admission.

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

## Focused verification of original-operation discovery (2026-11-09)

All runs below passed on PATH Go `go1.26.3 darwin/arm64`, with `GOPROXY=off`,
`GOSUMDB=off`, `GOTOOLCHAIN=local`, synthetic HOME/XDG directories, and existing Go
build/module cache paths captured before environment isolation. Full verbose
outputs are retained outside tracked source at
`/tmp/cercano-force-reopen-verification.ktLAFr/` in this worktree's execution
environment; this is a local artifact path, not a portable repository artifact.

From `source/server` (the first command was run normally and with `-race`):

```sh
go test ./internal/server ./pkg/agentclient -run '^(TestOriginalRecovery|TestResumeDiscoversOriginalBeforeMint|TestResumeNewDestinationInterruptedTransfer|TestRetryTransfer|TestExactRecovery)' -count=1 -v -timeout=180s
go test -race ./internal/server ./pkg/agentclient -run '^(TestOriginalRecovery|TestResumeDiscoversOriginalBeforeMint|TestResumeNewDestinationInterruptedTransfer|TestRetryTransfer|TestExactRecovery)' -count=1 -v -timeout=180s
go test ./cmd/cercano -run '^TestDevelopment(Peer|SourceResolver)' -count=1 -v -timeout=180s
```

Both server/client runs passed **22 top-level tests and 69 subtests** each.
The command run selected and passed exactly two tests:
`TestDevelopmentPeerRequiresPrivateRecord` and
`TestDevelopmentSourceResolverExactPrivateRecord`.

From `source/clients/cli`:

```sh
go test ./internal/ui -run '^(TestOriginalRecoveryUIConfirmation|TestForcedResumeConfirmationAndStaleCallbacks|TestDevHostOrdinaryReopenStartsCommand)$' -count=1 -v -timeout=180s
go test -race ./internal/ui -run '^(TestOriginalRecoveryUIConfirmation|TestForcedResumeConfirmationAndStaleCallbacks|TestDevHostOrdinaryReopenStartsCommand)$' -count=1 -v -timeout=180s
```

Both CLI runs passed **3 top-level tests and 12 subtests** each. Zero-test
compile-only checks also passed: server `go test ./cmd/cercano -run '^$'
-count=1 -timeout=180s` and CLI `go test . -run '^$' -count=1 -timeout=180s`.
Those checks ran no tests and are not additional behavioral evidence.

The original-recovery cases cover historical claim release/fresh epoch/movement,
failed journal sync with no source calls followed by exact recovery, and journal
replacement during sync without overwrite. These are passing verification runs
of the reviewed partial feature: **no retained new pre-fix red/green logs were
found, and no historical red reproduction is claimed for this increment**.
Fixtures only: no live/deployed host, credential, real recovery, or older-binary
compatibility validation was performed. Legacy behavior is simulated by fixture
handlers returning `Unimplemented`, not by running an older binary. No push,
merge, deployment, or live configuration/database changes are part of this work.

## Earlier offline verification

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
