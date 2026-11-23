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

## Follow-up coverage and remaining boundaries

The matrix now additionally runs autonomy UPDATE and route SET success/SQLite
abort cases through `ConfigurePreparedWorker`. Success asserts the actual SQL
row, not just lack of an error. Route validation uses an in-memory dummy secret
and an invalid network endpoint; no inference or live credentials are used.

`blocked-success` places a channel barrier inside the installed autonomy callback's
store call (a forwarding store decorator, not a failure marker). After callback
entry, authenticated terminal release must leave ownership durable and reject
replacement acquisition. Only after opening the barrier may callback completion,
worker EOF, release and independent replacement occur.

Both hosts use real authenticated pinned agentclient APIs. The original client
validates its binding before close and rejects `ReconnectPinnedSession` after
replacement. The replacement explicitly creates a new binding to the exact durable
epoch and uses `ResumeConversation` to read the preserved transcript. The completed
task row is separately verified in SQLite. No Chat call or automatic replay occurs.
This tests explicit binding/resume, not new reconnect orchestration or stale Chat RPCs.

`TestProcessCloseWaitResult` observes production `workerHandle.Kill`'s `cmd.Wait`
ProcessState for the exact test-owned child PID and SIGKILL, then verifies it is
already reaped. This is a separate worker-package test: the installed-callback
probe still only observes its pooled child's socket disappearance, not that child's
Wait result. These must not be conflated.

`TestPreparedWorkerRealMutationLostResponse/task/success` now covers both EOF and
cancellation, verifies the successful task INSERT before dropping the response,
and requires zero unresolved mutation tickets. It uses real `serverTasks.Launch`
with an observation wrapper, not test-created mutation evidence. It is not yet a
policy-spawned installed-task-callback probe. Task evidence in replacement remains
fixture-created. Route/autonomy process probes receive callback responses before
withholding TurnDone; they are not lost-callback-response probes.

The combined bounded callback-deadline/worker-retirement/new reconnect integration
is still unverified. Current `worker/host.go` unconditionally waits on callback
completion before cleanup; this work tests ordinary blocked-callback completion,
not a context-ignoring callback deadline or an integrated retirement branch. No
pending test is silently skipped and no deadline defect reproduction is claimed.
No production changes, launcher/CLI rebuild, or automatic ambiguous chat replay
coverage are claimed.

### Follow-up verification

From `source/server`:

```sh
go test ./internal/server ./internal/worker -run '^(TestProcessClose(InstalledCallbacks|WaitResult)|TestPreparedWorkerRealMutationLostResponse)$' -count=3 -timeout=90s
go test -race ./internal/server ./internal/worker -run '^(TestProcessClose(InstalledCallbacks|WaitResult)|TestPreparedWorkerRealMutationLostResponse)$' -count=2 -timeout=120s
```

During development the route fixture initially failed profile/credential validation
rather than reaching SQLite; configuring the prepared profile and in-memory secret
fixed that. Darwin's second Process.Wait returns ECHILD rather than ErrProcessDone;
the reaping assertion accepts either platform result. Neither was a production defect.
All stores, sockets, auth tokens and processes are test-owned and disposable.
