# Upgrade cancellation boundary regression (2026-10-30)

This is a bounded native-shutdown correction, **not combined upgrade acceptance**.
The ordinary-upgrade safety guard remains unchanged. Fixture-scoped discovery is
user-authorized for the future combined acceptance, but that run is not established
by this regression. Production discovery is unchanged.

## Failure established before correction

Expected: after the worker has exited on TERM, canceling the caller must return a
cancellation error only after bounded cleanup of the verified shutdown targets.
The host must not resume to create additional work. Before TERM, an already
canceled request must leave the host and worker running.

The new `TestUpgradeCancellationAfterTERMFinishesCleanup` cancels only after native
inspection observes the disposable worker's exit. Against the original shutdown
implementation it failed with `host survived` (exit 1). The error path previously
resumed the host with pending TERM and returned before verifying its exit.

## Correction and assertions

- Reject an already canceled context before inspection or signals.
- Preserve reversible refusal behavior before any TERM can be sent.
- Once entering TERM delivery, do not resume the producer on errors. Finish KILL
  and exit verification with a separate three-second cleanup deadline. All
  targets retain native executable/start identities; no process-group signals.
- Return the original cancellation/error (joining cleanup failures), so callers
  cannot interpret cancellation as migration authorization.
- The regressions assert host and worker absence, no historical RPC cancellation
  marker, and continued worker progress after an already canceled call.
- Fixture cleanup now also uses `signalIdentity` for both host and worker.

If identity-checked cleanup itself fails, report that failure and do not resume a
possibly partially terminated producer. This is bounded failure handling, not a
claim of guaranteed termination under kernel errors.

## Evidence

From `source/server`, using `/opt/homebrew/bin/go`:

```
go test ./internal/brewrestart -run '^TestUpgradeCancellationAfterTERMFinishesCleanup$' -count=1
go test ./internal/brewrestart -run '^TestUpgrade(StopsRealHostAndActiveWorker|CancellationAfterTERMFinishesCleanup)$' -count=3
```

The first command was the failing pre-fix probe. The second passed after the
correction (three repetitions). Commands used a fresh disposable HOME, TMPDIR,
XDG config/runtime/state/cache/data, disabled credentials, `GOTOOLCHAIN=local`,
`GOPROXY=off`, `GOSUMDB=off`, and existing user Go caches. No inference was used.
Only owned disposable fixture host/worker processes were signal targets.

## Limits / remaining acceptance

The native unit fixture is a copied test executable; it is not a production
`cmd/cercano` build and must not be represented as production-worker acceptance.
The existing `test/fixtures/production-stop/run.py` harness uses Python host-only
termination, not `StopUpgradeHost`; it was read but not executed for this unit.
No SQLite backup, migration, replacement startup, production tool-descendant
inventory, shared-model-host survival, or in-flight fork/exec coverage is claimed.
No launcher guard or list/inspect implementation/tests were changed.
