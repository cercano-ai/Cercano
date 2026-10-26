# Compiled execution acceptance: completion metadata blocker resolved

2026-10-26. The reproduced completion blocker at fbac8b3a45bd is resolved by host metadata replacement. The original failed run is retained below; see the resolution and actual reruns at the end.

## Implementation

`devhostfixture` build-tagged CLI/worker injection uses synthetic provider/tools, a registered forbidden sentinel, the existing pinned spawn policy, and a file barrier for an independent task launched via the actual worker task bridge. Untagged execution hooks remain disabled. The fixture wires prepared-server tasks and worker fixture tools only under the tag. Temporary generation artifacts/stores and isolated HOME/config are used, with credentials disabled. No live installation, real model/provider, Keychain, push or merge was used.

Expanded the compiled test to cancel the originating foreground RPC context, assert the independent task is still running after B startup and journey transfer, refuse busy sibling release/transfer, release the barrier, and require terminal completion notification drain and exactly one durable completion turn. Existing assertions cover A/B persisted foreground identity, stale session rejection, and busy retirement.

## Reproduced blocker

Plain focused test failed twice; after gofmt, the race harness reproduced the same failure at `cmd/cercano/dev_host_test.go:512`:

- Journey foreground persisted `fixture-build-exact` on A, then `fixture-build-B` on B after transfer.
- The independent sibling remained running after its caller context was canceled and after B startup/journey handover.
- Busy retirement/release/transfer assertions were reached without failure.
- Barrier release produced durable status `completed` and result text `independent completed fixture-build-exact`.
- Notification remained `NotificationPending:true, WakeClaimed:true`.
- Host stderr: `task completion turn sibling: rpc error: code = Unavailable desc = worker turn failed: rpc error: code = InvalidArgument desc = worker: invalid initial capability profile`.

The worker rejects duplicate or malformed private initial-profile metadata (`internal/worker/worker.go:272-277`). Host transport appends that metadata (`internal/worker/host.go:718`). Inherited duplicate metadata is a suspected cause, not independently established here. The production validator was NOT relaxed, and no fixture bypass was added. Completion acknowledgement/exactly-once delivery and subsequent clean release/retirement remain unverified. This is a concrete failing completion case rather than a toolchain limitation.

## Commands actually run

From `source/server`, with `GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1`:

- `go test ./cmd/cercano -run TestDevHostCompiledExecution -count=1`: FAIL (twice), completion drain/profile error above.
- `go test -race ./cmd/cercano ./internal/worker -run 'TestDevHost|TestPinnedFixtureSubprocessGenerations' -count=1`: FAIL, same compiled completion blocker; worker package PASS (3.657s). No race warning printed. Compiled CLI children are built by the test without `-race`; this is race instrumentation of the test harness/worker test package, not those child binaries.
- `go test ./cmd/cercano ./internal/worker -run 'TestDevHostCompiledStartup|TestPinnedFixtureSubprocessGenerations' -count=1`: PASS (8.228s / 1.616s).

The registered `run_command` sentinel was blocked by the plan profile in the actual subprocess logs. Existing unrelated `plan.md` modification excluded from checkpoint. Modelhost acceptance is unchanged.

## Resolution: probed, not inferred

Before changing propagation, temporarily expanded the worker's invalid-profile error to print the received values and reran `go test ./cmd/cercano -run '^TestDevHostCompiledExecution$' -count=1` under the same offline/credentials-disabled environment. It failed in 14.471s with the same completed task and `NotificationPending:true WakeClaimed:true`. The worker reported **two identical valid JSON plan profiles**, each with `AllowedTiers:{"R":true}` and the fixture's existing extra capabilities. This establishes duplication, not malformed JSON, as the concrete cause. The diagnostic was then removed; `worker.go` is unchanged.

The host uses the profile-bearing context for `handleTaskControl` (`internal/worker/host.go:998`); independent launch retains context values via `context.WithoutCancel` (`internal/taskrun/manager.go:179`). The completion wake therefore inherits the prior outgoing profile. Appending a new host profile produces the two observed values.

`withInitialProfileMetadata` now copies outgoing metadata and **sets** only `cercano-initial-profile` to the current authoritative host profile. It derives from the original context: unrelated metadata, context authority, deadlines and cancellation survive; the retained parent metadata is not mutated. No changes to task ownership, worker rejection, execution gates, production fallback or modelhost. Nil profile-reader behavior remains unchanged. Existing absent/null/empty-object legacy profile behavior is not a general compromised-worker sandbox claim.

Regression tests cover absent, empty-object, null, malformed, stale and duplicate inherited metadata; repeated replacement; unrelated appended metadata; parent immutability; context-value identity; deadline and cancellation preservation. A real bufconn worker still rejects directly supplied duplicate and malformed metadata with `InvalidArgument`.

### Actual post-fix runs

From `source/server`, all with `GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1`:

- `go test ./cmd/cercano ./internal/worker -run 'TestDevHostCompiledExecution|TestInitialProfileMetadata|TestWorkerServerRejectsInvalidInitialProfileMetadata' -count=1`: **PASS**, 9.589s / 1.041s. Complete compiled acceptance reached terminal result, notification drain, exactly one durable completion (including delayed recheck), sibling release, clean A retirement, and continued B usability.
- `go test -race ./cmd/cercano ./internal/worker -run 'TestDevHost|TestPinnedFixtureSubprocessGenerations|TestInitialProfileMetadata|TestWorkerServerRejectsInvalidInitialProfileMetadata' -count=1`: **PASS**, 18.418s / 4.101s; no race warning. Includes startup and complete execution acceptance. Compiled child binaries are not race-instrumented by this harness.
- `go test ./internal/worker -count=1`: **FAIL** under globally disabled credentials (18.368s command elapsed). Failing credential-dependent tests: `TestWorkerBackupFailover_WrapsCompositeAndFailsOver`, `TestWorkerBackupFailover_NoBackupIsBareProvider`, `TestBuildWorkerProviders_PrimaryFetchFail_UsesConfiguredBackupWithoutNilPanic`, `TestSubscriptionBackupDoesNotFetchCredentialsDuringBuild`, `TestWorkerRefreshAndHostLoginShareCredentialOwner`, `TestWorkerRunner_TokenSourceSingleFlightPerProfile`, `TestWorkerRunner_TokenSourcesUseConfigOwner`, `TestDestinationRedirectWorkerExecution`. Errors include `credentials disabled by CERCANO_DISABLE_CREDENTIALS`, missing cloud providers and zero credential calls. No claim of full-package green; did not enable credentials to bypass this constraint. Focused worker regression and scoped race above pass.

These runs preceded concurrent, separately owned changes noticed in `cmd/cercano/dev_host_execution_fixture.go`; those fixture changes are not part of this metadata checkpoint. `plan.md` also remains excluded. No install, live provider, real credentials, push or merge was used.
