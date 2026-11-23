# Process close acceptance probe

Base: `9a3aaffe9353`. Test-only changes; no worker/pool, agentclient or CLI production changes.

## Execution

From `source/server`:

```sh
go test ./internal/server -run '^TestProcessCloseInstalledCallbacks$' -count=3 -timeout=90s
go test -race ./internal/server -run '^TestProcessCloseInstalledCallbacks$' -count=2 -timeout=120s
```

Normal `go test` builds the test executable; the acceptance copies that compiled executable into a private generation as its worker binary and executes independent host and worker processes. There is no optional binary lookup or silently skipped acceptance. This is NOT installed launcher/production CLI binary acceptance. Helper tests return when not invoked by the parent.

## Matrix

| Boundary | Evidence |
| --- | --- |
| Actual callback installation | `ConfigurePreparedWorker`, not test-installed ledger/evidence wrappers |
| Actual persistence failure tracking | SQLite autonomy INSERT abort trigger; unresolved work survives transport EOF and cleanup |
| Real worker transport | Policy-spawned child, Unix gRPC socket; deterministic callback-returned/drop-response file barriers |
| Real control transport | Loopback gRPC, development token authentication, exact pinned host/conversation/epoch/terminal metadata |
| Intentional close before worker completion | LifecycleRelease while callback has returned but peer has not completed RunTurn |
| No authority from transport loss | Durable owner and rejected acquisition before completion; failure also rejects acquisition after EOF |
| Ordinary outcome | EOF without TurnDone, no injected callback error; durable release required |
| Cleanup | Explicit production CleanupOwnedWorkers and socket absence assertion; worker has self-exit watchdog if host is killed |
| Replacement | Independent host process, same SQLite store, exact durable new epoch, different from old epoch |
| Evidence | Replacement reads preserved assistant transcript and completed delegated task row |

## Limits / remaining acceptance

This is a narrower process probe, **not completion of the full close/rebuild/resume matrix**. The callback barrier is after SQL callback return, not inside a pending callback. The peer receives its response before withholding TurnDone: this is not lost-callback-response coverage. The earlier prepared_worker_mutation tests remain the lost-response coverage and still manually install evidence scopes for autonomy/dispatch.

No fresh production binary rebuild, restored agentclient binding, stale RPC binding rejection, ordinary callback pending deadline regression, exact child process-exit assertion, route SET/autonomy UPDATE or successful task-insert lost-response controls are claimed here. Replacement uses the lifecycle server directly rather than client reconnect orchestration. Task evidence is fixture-created, not newly launched by this worker.

The pending-callback deadline/retirement case still depends on the bounded teardown branch; client binding restoration still depends on the reconnect branch. Neither is integrated or inspected by this work. These passing probes must not be cited as verification that join-before-cleanup is fixed. Socket disappearance and callback completion are explicitly not process-exit evidence.

All host subprocess waits have parent deadlines; the worker gates and test worker itself have deadlines. Only test-owned processes are started or cleaned up. Host HOME/config roots and stores are disposable. No live configuration or credentials are used.
