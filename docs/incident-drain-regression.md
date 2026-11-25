# Closed/draining incident regression investigation

Scope: tests only, from main in `test/incident-drain-regression`. No live database,
RPC, signals, recovery, production behavior changes, landing, or pushing.

## Prediction and baseline

Before execution: successful production callbacks should drain after join;
actual SQLite mutation failures should retain unresolved accounting. A previously
reserved terminal closure should refuse forced cutover before the later
unresolved-persistence check. Terminal task status alone is not reconciliation.

Existing `TestProcessCloseInstalledCallbacks` passed unmodified (8.556s package
time). No clean-workflow red evidence was observed.

## Added coverage and observations

* `TestIncidentClosedDrainCharacterization` runs in a subprocess with temporary
  HOME/config and SQLite. It deliberately marks three admitted persistence tickets
  unresolved; **this does not reproduce their historical creation**. Ten durable
  tasks are terminal (nine completed, one failed). Real terminal release produces
  closed/draining, zero attachments, work persistence=3/unresolved=3, host
  persistence=1/unresolved host=0. Three destination LifecycleResume calls offer
  confirmation without changing the exact durable owner/epoch or stopping the
  source. Existing closure stays reserved; direct force relinquish returns
  `broker.ErrAdmissionBusy`, not the later unresolved-persistence refusal.
* Process-close cases now include successful TurnDone and ordinary worker
  execution error with an installed production autonomy callback blocked at the
  actual store seam. Terminal close happens before that channel barrier opens.
  Both join the turn, reap the owned pooled process, release durably, and resume
  via destination LifecycleResume without a termination confirmation. The
  existing canceled-turn case and real SQLite trigger failures remain covered.
  Success paths additionally require zero unresolved work and host work.
* Synthetic worker protocol peer remains the boundary: the new error is a gRPC
  execution error, **not an injected failure inside a real inference provider**.
  No claim is made that provider execution/task completion origins are covered.
  One callback is used rather than fabricating three historical completions.
  Existing process fixture polling remains for file/socket and durable-release
  observation; new ordering uses channels, not timing sleeps. All child commands
  are bounded and waited; production owned-worker cleanup and RPC joins remain.

Test development exposed only test mistakes: an incorrect RPC method name,
missing cutover destination, and assuming a closure-specific error string rather
than the actual admission-busy sentinel. These are not product failures.

## Verification

From `source/server`:

```
go test ./internal/server -run TestProcessCloseInstalledCallbacks -count=1 -timeout=120s
go test ./internal/server -run 'TestIncidentClosedDrainCharacterization|TestProcessCloseInstalledCallbacks' -count=3 -timeout=180s
go test -race ./internal/server -run 'TestIncidentClosedDrainCharacterization|TestProcessCloseInstalledCallbacks' -count=2 -timeout=240s
go test ./internal/server ./internal/broker ./internal/worker ./internal/conversation ./internal/taskrun -count=1 -timeout=240s
```

Baseline, repeated normal and race runs pass. Neighboring suites pass. No
failure-triggered origin tracing or production observability API was added.

## Gaps and recommendation

The supplied incident snapshot is downstream evidence only. Historical ticket
admission origins and PersistenceFailed call paths remain unknown; this branch
cannot establish a historical teardown root cause. Synthetic host/conversation
names and fresh exact epochs stand in for the live identifiers. Source build
and dirty-worktree differences are not reconstructed. The task-status fixture
is explicitly constructed, not a task/provider execution reproduction.

Independent review should assess this boundary before landing. Next bounded
investigation: run real prepared task/provider execution with an injected
provider error and cancellation through the same callback-close barrier. Only
if that clean workflow leaves uncertainty, add test-only origin/epoch,
PersistenceFailed stack, callback/producer completion and closure lifetime
tracing. Do not clear tickets or weaken accounting to make it green.
