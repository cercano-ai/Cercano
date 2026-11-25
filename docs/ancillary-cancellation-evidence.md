# Prepared ancillary cancellation evidence

## Scope and analytical expectation

Base: `cbd16185c270e6c46f9d5f4669c8aa2a38521fc9`, branch
`test/ancillary-cancel-evidence`. Test-only characterization, no production fix.
The supplied incident account describes successful final replies overlapping
ancillary cancellation, not missing task follow-up. No live history or DB was read.
Neither matching ticket counts nor this reproduction identifies historical tickets.

Analytical expectation: real retained post-turn admissions remain busy while
providers are blocked. Cancellation alone is not completion; lifecycle close must
join all callbacks. Recap inference-only errors are warnings and should clear.
Compaction currently classifies every execution error as persistence uncertainty,
even when the write-outcome observer is nil. Thus it is expected to remain busy;
this is characterization, not the desired clean-cancellation contract.

## Probe

`source/server/internal/server/ancillary_cancel_evidence_test.go` runs in an owned
subprocess with temporary fixture DB/config and scripted inference. It executes a
real Core main turn and persistence callback, obtains the normal final reply,
and completes its retained foreground ticket. Real prepared scheduling admits
recap, compaction, or both. Agent and Core both schedule recap: the barrier waits
for **both** provider entries, avoiding the misleading earlier one-entry probe
where the second recap could be canceled during a store read.

The actual scheduling lifecycle close cancels providers. Each provider stays
blocked after cancellation until released; close may not abandon it. After close,
all entered callbacks must have returned, their write outcomes must be nil, and
remaining persistence work must be unresolved rather than unjoined. Diagnostics
include owner identity/epoch, before/after local and host accounting, and delegated
recap store-read failures with context/write outcomes. Retry admissions can be in
CallbackWork before advancing; counts are bounded, not treated as ticket IDs.

Recap-only clears, releases durable ownership, and permits a new host to retain
with a new epoch without recovery. Compaction and combined cases retain unresolved
work and reject release with ErrAdmissionBusy. Their new-host success path is not
claimed. No PersistenceFailed or workerTurnDone is manufactured by this probe.

## Supported explanation and proposed later fix

`internal/compactiongen/owned_schedule.go:106-118` covers inference with a
PersistenceWork ticket, joins the run error with the persistence observer, and
calls PersistenceFailed for **any** non-nil result. The real first summarizer
returns context.Canceled before a successful summary/writeback; the observer is
nil, yet the error branch leaves uncertainty. This does not require a canceled
worker connection or a failed DB write. Recap distinguishes auxiliary inference
failure (`internal/recap/owned_schedule.go`) and does not take that branch.
The persistence bridge (`internal/hostsvc/persistence/owned_compaction.go`) has a
separate write-outcome callback; that callback is not necessary for this blocker.
Prepared scheduling close fences then cancels and joins the generators; generator
contexts are linked to both admitted parent and generator root.

Proposed later change: distinguish proven joined inference-only failure from
unknown or failed durable effects in compaction, analogous to recap. Do not simply
ignore all context cancellation or all errors with a nil observer: partial passes,
store failures, usage callbacks, and unreported writes require conservative handling.
Add typed outcome/provenance coverage before relaxing accounting. No such change
is made here.

## Validation

From `source/server`:

```sh
PAT='^(TestPreparedAncillaryCancellationEvidence|TestPreparedDispatchJoinsChildFinalCallback|TestPreparedWorkerRealMutationLostResponse)$'
go test ./internal/server -run "$PAT" -count=20
go test -race ./internal/server -run "$PAT" -count=20
go test ./internal/server -count=1
```

All passed. Existing real-mutation negative controls use SQLite abort triggers
(`prepared_worker_mutation_test.go:123-125`) and preserve failure accounting under
lost/canceled responses. Existing final-callback tests cover writeback join and
reported write failures. They were not modified.

Isolated inverted desired-behavior proof (expected exit 1; never enable globally):

```sh
CERCANO_ANCILLARY_EXPECT_CLEAN=1 go test ./internal/server \
  -run '^TestPreparedAncillaryCancellationEvidence$' -count=1
```

Observed two `desired clean inference cancellation: unresolved=1` failures,
for compaction and combined cases. The default characterization passes.

## Limits

This is not an end-to-end computer-use worker/RunPrepared or completion-wake test:
it runs Core directly using real retained admission and prepared scheduling. It
does not reproduce host shutdown attachments, transport cancellation, historical
source timing, ordinary resume against an already closed owner, partial successful
compaction segments, or non-cancellation provider errors. Callback return and
producer close are joined, but individual ticket IDs/transitions are not exposed
by AdmissionSnapshot; origin is established through isolated producers and source
paths, not counts. The genuine-write negative controls are existing mutation
paths, not an injected ancillary SaveCompaction failure. Full repository suite and
server build were not run; normal server package and focused race coverage were.
No live operations, production behavior changes, landing, or pushing.
