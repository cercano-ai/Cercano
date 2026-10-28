# Bounded incomplete shutdown decision — 2026-10-27

## Approval and limits

The delegated user instruction explicitly authorizes **bounded incomplete shutdown with deferred exactly-once disposal**. This records that decision without changing the approved specification. It does not authorize a helper process, hard process-exit deadline, forced termination, native credentials, live activation, replacement, or ownership release on timeout. Canonical generation activation remains gated.

## Implemented production boundary

`startGRPCServerWithLifecycle` and its wrappers now return a `*productionShutdown`, not a successful-looking `func()`. `Shutdown()` fences this host's ordinary RPC admission synchronously and waits at most 100ms for disposal. `Status()` is a non-secret, lock-free control-plane observation available to the owning caller without RPC/work admission. `Complete=false` is not retirement/readiness or permission to replace the host. Concurrent and repeated requests share one disposal operation.

The only synchronous stop callback is an atomic admission fence. Watcher cancellation, event-stream closure, watcher/initial-provider joins, transport drainage and resource disposal run outside the bounded caller. A synchronous credential/provider callback cannot be cancelled by this API. On completion the retained cleanup runs automatically exactly once. No forced gRPC stop is used for normal shutdown. Startup rollback returns its pending handle alongside the original startup error when cleanup is incomplete.

The standalone process owner reports incomplete and stays alive until disposal; crash diagnostics remain open. The second-signal forced-exit shortcut was removed. Embedded MCP retains its owned handle, including startup failure, and its error returns no longer bypass deferred cleanup. External MCP owns no server lifecycle. Process retention is intentionally not a bounded process-exit promise.

Retention is invocation-local, not a process-wide generation fence: only that composition's server/provider graph, stores, diagnostics, telemetry, runtime attachment, policy and integration resources are captured. Callbacks and previously admitted RPCs can still use those dependencies, so disposal cannot precede callback joins and graceful RPC drainage. Safe sibling admission is unaffected. This is not a claim that the gated canonical generation graph or all historical development-host retirement paths are qualified.

## Verification

Offline, credentials-disabled race test passed, three repetitions:

```sh
cd source/server
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./cmd/cercano -run 'TestProduction(SingleHostStartupShutdown|IncompleteShutdownControl|FailureStages|ListenerFailureClosesOwnedStore)$' -count=3 -timeout=90s
```

Coverage includes initial rebuild callback barrier, a real filesystem config watcher blocked inside its injected provider factory, both late-MCP-failure cases, ordinary RPC refusal while incomplete, independently accessible status, automatic eventual disposal, repeated/concurrent shutdown and retained store/audit/policy authority. The initial rebuild barrier is an invocation-local seam before `RebuildCloud`, not a native Keychain simulation. All fixture paths are temporary; runtime/provider/native-opener traps remain installed.

A broader `-run TestProduction` race run failed in existing `TestProductionCompositionServiceParity`: `open read-only credentials: credentials disabled by CERCANO_DISABLE_CREDENTIALS`. Credentials were not enabled to bypass that failure. No broad-suite pass is claimed. The targeted lifecycle suite above passed after the final edits.

Phase 1's composition/failure-handling task remains in progress: this bounded lifecycle slice does not complete generation parity. Separate compaction/persistence scheduling work owns its own verification; none of those files are included in this checkpoint.
