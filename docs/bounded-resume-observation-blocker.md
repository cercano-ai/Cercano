# Bounded resume observation: pre-fix investigation

Status: blocked; no production fix applied.

## Smallest reproduction

`TestBoundedPreflightIgnoredContextDiagnostic` calls only `boundedPreflight`
with a callback waiting on an owned channel, ignoring context. The caller's
20 ms deadline expires; after an additional 100 ms the call still has not
returned. Releasing the callback yields `context.DeadlineExceeded`. Every
callback is released and joined. This is a diagnostic of existing behavior,
not a regression test asserting the requested fixed behavior.

Recorded commands (source/server):

- `go test ./internal/server -run '^TestBoundedPreflightIgnoredContextDiagnostic$' -count=1 -v`: PASS, reproduced, 0.12 s test duration.
- `go test -race ./internal/server -run '^TestBoundedPreflightIgnoredContextDiagnostic$' -count=1 -v`: PASS, reproduced, no race reported.

Mechanism: `resume_gate.go` calls the callback synchronously and checks the
phase deadline only after it returns. This confirms that a deadline alone
cannot implement the approved stop-waiting semantics.

## Compatibility/safety boundary requiring resolution

`developmentDialIdentity` authenticates GetDevelopmentHostIdentity, then calls
LifecycleList to validate the full LifecycleIdentity, including Store.
GetDevelopmentHostIdentity's historical response has no Store field.
LifecycleList calls observeLaunchLocked and is not an observation-only callback.
Containment's shutdownAuthenticated uses this same dial before process
inspection and termination. Moving this dial into an abandonable goroutine
therefore does not satisfy observation-only isolation; leaving it unchanged
leaves confirmed recovery dependent on List.

A new observation-only full-identity RPC/field can solve this for upgraded
hosts, but historical peers do not provide it. Replacing the historical List
check with GetDevelopmentHostIdentity plus a trusted record changes the source
of the live exact-store assertion. The approved use of records for independent
*discovery* does not unambiguously authorize replacing that assertion before
termination. Inferring success from an intentionally invalid receipt RPC would
instead overload an unrelated API and depend on historical error semantics.

Do not silently drop Store validation, change List's launch-observation
semantics, or wrap mutating/exclusion-holding coordinators in background work.
The remaining compatibility choice is whether historical stop authentication
may use fresh authenticated host/build/contract/schema plus the trusted record
for the store binding (still retaining fresh exact-process/consent/epoch
checks), or must fail closed when a live observation-only full-store identity
cannot be obtained. The latter restricts historical recovery compatibility.

No production behavior, receipt reconciliation, admission fencing, ownership,
journal, authentication or termination code was changed. No live configuration,
database or process operations were performed. Broader tests were not run,
since this checkpoint contains only the isolated diagnostic and this report.
