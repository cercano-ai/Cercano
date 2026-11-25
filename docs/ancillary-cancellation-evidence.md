# Joined ancillary inference cancellation accounting

## Fix and scope (2026-11-25)

On `test/ancillary-cancel-evidence`, original repro preserved at
`b0cb4d360ada6c13297920180f69a9f5829e33f6`. The characterization below is
historical, not the current expected behavior. Clean joined cancellation is now
the default regression expectation; the opt-in environment switch is removed.

`compactiongen.runCompaction` records per-pass provenance only when `Advance`
returns the actual summarizer error before reaching `SaveCompaction`.
`ScheduleOwned` retains the execution error but does not mark this proven
inference-only failure as persistence uncertainty when the independent write
observer is nil. This includes provider errors, not merely context cancellation.
The persistence bridge still independently fences reported write failures.
Repeated verification also exposed a close/registration race: a bounded retry can
pass the scheduler fence check, then have `startWork` refuse registration after
Close fences the generator. That pass has executed no store, inference, or usage
callback. It is separately tagged `executionRefused`, and only clears if the
independent write observer is nil. This is not an exemption for canceled reads,
unjoined callbacks, or uncertain writes. A deterministic refused-registration
unit test checks that no read/inference/save occurs. The final focused 20 normal
and 20 race repetitions passed after this correction; earlier intermittent
server failures are superseded by these runs, not dismissed as flakes.
No accounting reset, replay policy, summary clear, or live-state change is made.

## Mutation/reporting audit

- `GetTurns` and `GetCompaction` errors still fence, even with a nil observer.
  This deliberately does not relax all safe/no-write validation failures.
- `Advance` operates on local derived state. Empty-summary validation errors are
  not tagged as provider failures. If later segments fail after partial progress,
  `Advance` can return changed state without an error: writeback still runs and
  its result is never classified inference-only.
- `SaveCompaction` errors fence directly, without requiring `ReportWriteFailure`.
  SQLite abort, missing ownership, and simulated lost acknowledgement after a real
  committed write are covered. Successful commits from earlier bounded passes
  remain durable; provenance is fresh for each pass.
- After successful save, `recordUsage` can write context usage. The production
  `recordCompactionContextUsage` reports `SaveContextUsage` errors; both the
  observer and bridge fence. Inference cancellation plus an independent genuine
  context-usage ownership refusal also fences. Callback panics remain fenced.
- Elision-only execution has no inference exemption. Production elision changes
  an in-memory floor, then usage writeback uses the same observer. Its existing
  ignored optional read errors are not reclassified by this patch.
- Production `loopcompact.BuildSummarizer` performs routing/inference, in-memory
  summary usage accounting, and dispatch telemetry; it does not save compaction
  or context usage. Attempt accounting remains its separate existing lane.
  Custom summarizers must report independent durable write failures; this patch
  cannot discover errors silently swallowed inside arbitrary injected callbacks.
- `Regenerate`/`Clear` and their explicit mutation semantics are unchanged.
  Cancellation does not complete admission while any provider/callback remains
  unjoined; worker transport/lost-response accounting is unchanged.

## Regression coverage and verification

All stores/configs are disposable. The Core main-turn probe completes successfully
while recap/compaction are blocked, cancels and joins callbacks, requires zero
remaining work, durably releases ownership, and performs ordinary `LifecycleResume`
on a new host with a fresh epoch and no confirmation/terminate offer. Modes cover
recap, compaction, both, and combined explicit provider errors after cancellation.
A separate bridge test performs three combined cancellations under one owner;
this is not three full main turns and does not identify historical ticket IDs.
SQLite controls verify no write attempt and retention of the prior summary for
inference failures, successful writeback, and unresolved real/uncertain failures.

Before-fix source and original probe were temporarily restored from HEAD and run:

```sh
CERCANO_ANCILLARY_EXPECT_CLEAN=1 go test ./internal/server \
  -run '^TestPreparedAncillaryCancellationEvidence$' -count=1
```

Expected exit 1: compaction and both each reported `unresolved=1`; recap passed.
Working files were restored immediately afterward. Post-fix commands, from
`source/server`, all passed:

```sh
PAT='TestPreparedAncillary|TestPreparedDispatchJoinsChildFinalCallback|TestPreparedWorkerRealMutationLostResponse|TestOwned|TestCompactionInferenceAndSQLiteOutcomes'
go test ./internal/compactiongen ./internal/hostsvc/persistence ./internal/server -run "$PAT" -count=20
go test -race ./internal/compactiongen ./internal/hostsvc/persistence ./internal/server -run "$PAT" -count=20
go test ./internal/compactiongen ./internal/hostsvc/persistence ./internal/server ./internal/compactor ./internal/broker -count=1
go build -o /tmp/cercano-ancillary-verification ./cmd/cercano/
# After strengthening the prior-summary assertion:
go test ./internal/hostsvc/persistence -run TestCompactionInferenceAndSQLiteOutcomes -count=20
go test -race ./internal/hostsvc/persistence -run TestCompactionInferenceAndSQLiteOutcomes -count=20
```

## Limits and historical interpretation

Full computer-use worker/RunPrepared transport and shutdown attachments are not
covered by the new main-turn probe. Neighboring lost-response and final-callback
regressions passed, but they do not establish full transport equivalence. Commit
uncertainty is a simulated lost acknowledgement after an actual SQLite commit,
not a filesystem crash. Full repository suite was not run; relevant full packages
and the server build passed. No live operations, landing, or pushing. Independent
review remains required.

The supplied historical account places canceled compactions at **22:07:02,
22:18:32, and 22:30:06** during old-host tenure, with three unresolved persistence
tickets later. This mechanism and repeated test make that explanation plausible,
not proven: no live history was inspected and counts cannot map ticket identity.

---

# Historical characterization (before the fix)

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
