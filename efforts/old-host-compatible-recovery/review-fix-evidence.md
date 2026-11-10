# Bounded review repairs — 2026-11-09

Base: `9c4335553264`. Approved spec/plan read; no design reopening.

## Changes

- Containment-capable resume tries ordinary acquisition, exact reservation claim, and existing old-compatible relinquish controls before offering host-stop recovery. Fresh confirmation resumes only its consent-bound recovery, not another cooperative attempt. No new old-host RPC dependency or journal enumeration prerequisite on this route.
- Installations without containment retain the existing receipt-based coordinator and adapter-less cooperative transfer. Full-suite execution exposed regressions when these handlers were removed; restoring that compatibility branch fixed them.
- Local ready ownership goes through ordinary acquisition, retaining completed-journal `OperationId` metadata.
- Regression tests assert zero source ShutdownAgent/contained-stop calls on successful ordinary paths; exact claims do not contact the source. A separate completed-owner fixture tests ready retry metadata directly.
- Consent tests mutate epoch, reserved destination/handover, selected endpoint/token, and source identity both before confirmation and at the adapter's final pre-stop validation boundary. Every mutation rejects without stopping/reassigning. The existing full-row validator is used immediately before STOP, not only after containment.
- Historical assertions preserve the full committed prefix and require exactly two additional rows, exact final response with no stream errors, and exactly one additional provider request. Completed-task notification is settled separately before recovery measurement.
- Historical drain confirmation stays pending with held work, then releases work during its 15-second bounded RPC. Both held rows must persist before the final recovered turn.

## Executed evidence

Commands below ran in `source/server` unless noted otherwise.

Baseline reproduction with new tests and the two production files temporarily restored from `9c4335553264` (restored afterward):

- `go test ./internal/server -run 'TestResumeOrdinaryBeforeContainedRecovery|TestDestinationResumeForceAndRetry' -count=1`: FAIL, as expected. Cooperative/reserved routes intercepted with `authenticated source and selected destination required`; adapterless/force routes failed with `live-source recovery requires a qualified containment adapter`. `/tmp/cercano-review-red.log`.
- `go test ./internal/server -run '^TestResumeReadyRetainsCompletedOperation$' -count=1`: FAIL, as expected; ready/current-owner response omitted operation ID. `/tmp/cercano-review-red-ready.log`.

Final tree:

- `go test ./internal/server ./cmd/cercano -count=1`: PASS (14.208s / 67.161s). `/tmp/cercano-review-normal.log`.
- `go test -race ./internal/server ./cmd/cercano -count=1`: PASS (52.792s / 138.849s). `/tmp/cercano-review-race.log`.
- From repository root: `OLD_RECOVERY_PRODUCTION_FIXTURE=1 OLD_RECOVERY_EXECUTE=1 sh efforts/old-host-compatible-recovery/fixture/build-production.sh`: PASS, historical production controller 40.60s; drain 14.00s, reserved 13.64s, blocked-timeout 12.96s. Owned/drain and reserved both logged multiple failed journals ignored, superseded consent refused, fresh consent recovering exact durable owner, and exact recovered provider turn persisted. `/tmp/cercano-review-historical.log`.

The historical run builds pinned `d7cbdeb21c44e0ceeefdf212203ed04a37d09150` offline with fixture tags and loopback provider adaptation. It uses disposable state and child processes, not live hosts/configuration. It does not establish arbitrary historical-binary compatibility.

## Remaining limits

The old held stream reports `unknown or stale ownership or ticket` during shutdown. The test explicitly reports this transport interruption rather than claiming a clean old-client stream; the held user/assistant rows persist exactly and coordinator recovery succeeds. Final recovered stream is separately required to have exactly one correct final and no errors.

No live recovery, VM exercise, cross-platform verification, push, or merge. `.tmp-old-recovery/` remains untracked and was not edited, removed, or used by the recorded historical build. Broader protocol cleanup and other unchecked approved-plan items are not claimed complete. Temporary `/tmp` logs are local evidence, not committed artifacts.
