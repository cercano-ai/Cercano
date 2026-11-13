# Bounded resume observation: pre-fix investigation

Status: resolved under the approved historical saved-record database binding.

## Resolution

Normal resolution and safe authentication use a lifecycle-wide eight-slot
observation pool and one shared five-second ordinary preflight budget. Expired
observers keep their slots until completion; late connections are closed by the
sole result owner. Recovery uses an independent saved-record resolver, never
reruns the wedged normal resolver, and freshly authenticates the historical
identity RPC without LifecycleList. The trusted private record supplies Store;
host/build/contract/schema still come from fresh authenticated identity.

Prepare, source stop, launch exclusion, journal and ownership operations remain
foreground/joined. Exact process identity, fresh consent, epoch fences and writer
absence remain mandatory. Caller cancellation cannot publish an offer. The
cooperative LifecycleList remains synchronous because it can mutate launch
observation; it uses its RPC deadline, not abandonable background execution.
An arbitrary context-ignoring mutating adapter is deliberately not bounded by
this observation mechanism.

Verified in disposable fixtures:

- `go test -race ./internal/server ./cmd/cercano ./test/integration -run 'Test(Bounded|Observation|Resume|LiveSource|LegacyShutdown|DevelopmentAuthenticated|DevelopmentLegacyProductionAdapter)' -count=1 -timeout 120s` (integration has no matching tests).
- `go test -race ./test/integration -count=1 -timeout 120s`.
- `go test ./... -count=1 -timeout 180s`.

All passed. Earlier runs exposed fixture omissions (loopback peer/destination
routing and the new authentication seam); these were corrected before the
final runs. No live host termination or configuration/database operation was
used for validation. The investigation below is retained as historical context.

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
