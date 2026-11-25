# Real completion / ordinary close characterization

Historical test-only investigation, 2026-11-25 (baseline `4d000f74876629503edb3e4f28b74e691d82c5fa`). The characterization below is preserved as historical evidence, not current expected behavior. See the approved correction below.

`TestRealCompletionOrdinaryClose` launches a real prepared task with scoped dispatch,
commits its terminal result, and lets `notifyOwnedTaskChange` automatically run the
parent follow-up through the production in-process runner and scripted provider.
The provider barrier is entered before ordinary authenticated `LifecycleRelease`.
The disposable fixture authorizes loopback lifecycle calls; no force path, manual
ticket marking, or callback suppression is used. This is not a worker-process test:
the prepared fixture worker image is non-executable, so the production in-process
runner is selected. Cancellation means a provider returning `context.Canceled`,
not cancellation of a real remote transport. The write-failure control reports a
synthetic persistence failure via the existing seam; adjacent genuine failed-write
coverage is unchanged.

Observed after joining manager and owned wake teardown:

| Provider outcome | WriteFailure | notification_pending / wake_claimed | session work / unresolved | ordinary destination resume |
|---|---|---|---|---|
| success | nil | no pending notification | 0 / 0 | succeeds |
| execution error | nil | true / true | PersistenceWork=1 / 1 | refused |
| context.Canceled | nil | true / true | PersistenceWork=1 / 1 | refused |
| reported persistence failure | fixture persistence failure | true / true | PersistenceWork=1 / 1 | refused |

Before release, the exact retained owner has CallbackWork=1, no host work and zero
unresolved work; its task is durably completed and wake-claimed. Ordinary release
closes authority and removes the terminal attachment. Before destination resume,
all cases have Closed=true, Draining=true and host PersistenceWork=1 with zero
unresolved host work: that host ticket belongs to the ordinary release closure,
not a second failed provider write. Success persists `followup persisted`; execution
error/cancellation leave the system completion input as latest parent output.
Child result JSON remains durable. The failing cases retain the source owner.

## Interpretation and spec boundary

`server/task_completion.go:185-200` routes any unsuccessful owned completion turn
through `w.fail`; `:331-334` unconditionally converts the callback ticket to failed
persistence. Thus nil `WriteFailure` does not prevent unresolved persistence
accounting. The reproduction supports this specific origin hypothesis, not the
historic three-ticket origin (still unconfirmed), and disproves “ordinary close /
resume always fails”: the success control resumes on B.

`efforts/independent-subagent-lifecycle/spec.md:15,33,93` requires durable results,
no lost notifications, serialized wakes and no endless delivery loop. A pending
notification is explicitly information, not unfinished inference. Meanwhile
`efforts/dev-host-generations/spec.md:55` requires all callbacks and persistence
to finish and unresolved persistence failures to block release. Consequently the
current refusal is correct *given* the failed ticket, but labeling a joined failed
inference with known durable terminal/pending state as persistence uncertainty is
not required by these specifications. Pending delivery cannot simply be discarded
or acknowledged as success. The current wake-claimed state also means a correction
must establish how destination/human-boundary delivery recovers it; this test does
not prove that merely dropping the ticket is safe.

Recommended next approval: narrowly distinguish inference failure from unknown
write outcome, durably settle/reconcile pending delivery and wake-claim state, then
complete the callback only after producer teardown and known persistence outcome.
Preserve failed-write blockers and require ordinary destination delivery/no-loss
regression coverage. No production fix made here.

## Commands and outputs

From `source/server`:

- Initial focused attempt in this continuation reached providers but failed all
  cases with `PermissionDenied: lifecycle authentication trust not configured`.
  This was fixture setup, not behavioral evidence. Added disposable authorization
  and a real terminal binding/attachment; no production bypass was changed.
- `CERCANO_DISABLE_CREDENTIALS=1 go test ./internal/server -run '^TestRealCompletionOrdinaryClose$' -count=1 -v`
  — PASS, all four controls, 0.12s; exact outcomes above.
- `go test ./internal/server -run '^TestRealCompletionOrdinaryClose$' -count=5`
  — `ok cercano/source/server/internal/server 1.227s` (includes destination resume).
- `go test -race ./internal/server -run 'Test(RealCompletionOrdinaryClose|PreparedOwnedCompletion|PreparedDispatch|PreparedForeground|ForcedCutoverCancelsScopedDispatch|Incident)' -count=3`
  — `ok cercano/source/server/internal/server 19.165s`.

The green test is explicitly a characterization of current behavior, not an
expected-failure wrapper or proof that the behavior is desired. No broad producer
matrix was added because the first real completion producer reproduces the issue.
No full-suite/build claim; no live homes, stores or external providers used.

A destination-fixture draft initially failed compilation (incorrect constructor API); corrected to `NewCoexistenceServer` before the final repeated/race runs above.


## Approved correction and regression (2026-11-25)

User-approved policy: an ordinary failed automatic followup must allow normal
close/reopen. Keep `notification_pending=true` and `wake_claimed=true` unchanged;
resume must not retry it. A later human-driven turn can handle and acknowledge it
once under the current owner. Ending an attempt is not acknowledgement. Genuine
mutation uncertainty and unknown teardown continue to fence ownership.

The completion scheduler no longer converts a joined inference error or returned
cancellation into a persistence failure. The retained callback still spans runner
join, synchronous callbacks, and acknowledgement. Claim and human-boundary ack
errors now explicitly report write failure, independently of execution errors.
Direct setup mutation failures report through a host callback on runner.Request
(JSON-excluded to preserve the worker wire boundary); worker-host setup reports
locally. Runner panic reports unknown teardown; unexpected subscriber closure waits
for the producer before releasing authority. Existing transcript/output persistence,
session-model, worker ledger/evidence and descendant cleanup paths retain their
write-failure reporting and join fences. No reset/requeue API or resume retry was
introduced; prepared_reload's claimed-notification filter is unchanged.

Regression extends the original real prepared-task/scripted-provider fixture:
ordinary error/cancellation transfer to B, stale-owner ack rejection, repeated
resume without inference, retained pending/claimed flags, failed human handling
without a fence, then successful handling with one SQLite-audited acknowledgement.
Negative controls include cancellation plus write failure and real SQLite trigger
failures at setup, output and notification acknowledgement. These remain fenced.
The setup failure correctly persists no completion input/output. Existing callback
join and descendant tests remain in the race-enabled group.

Verification in this continuation (from source/server):

- Re-ran desired error/cancellation assertions with baseline task_completion.go
  temporarily restored: `go test ./internal/server -run
  '^TestRealCompletionOrdinaryClose$/(error|canceled)$' -count=1` — RED, both retain
  UnresolvedWork=1 at assertion line 194. Restored fixed file immediately. This
  reconfirms the earlier tests-first red evidence; it is not a new tests-first claim.
- `go test ./internal/server -run '^TestRealCompletionOrdinaryClose$' -count=20`
  — PASS (7.087s).
- `go test -race ./internal/server -run
  'Test(RealCompletionOrdinaryClose|PreparedOwnedCompletion|PreparedDispatch|PreparedForeground|ForcedCutoverCancelsScopedDispatch|Incident)' -count=3`
  — PASS (27.097s).
- `go test ./internal/server ./internal/broker ./internal/conversation
  ./internal/hostsvc/persistence ./internal/runner ./internal/taskrun
  ./internal/worker ./cmd/cercano` — PASS.
- `go build -o /tmp/cercano-incident-review ./cmd/cercano/` — PASS.

Limits: deterministic temporary SQLite and in-process provider coverage, not live
transport cancellation or a worker-process end-to-end reproduction. Synthetic
write-failure controls supplement (do not replace) SQLite mutation failures.
This does not establish the historic three-ticket incident origin. Independent
review is required before landing/pushing, especially callback completeness,
producer join, worker/request serialization and stale-owner fencing.
