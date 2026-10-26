# Compiled execution acceptance: concrete completion blocker

2026-10-26. Continued the retained test-only fixture changes after the pinned-worker slice. Acceptance is NOT complete.

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
