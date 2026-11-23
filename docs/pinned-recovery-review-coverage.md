# Pinned recovery review follow-up

Client/CLI only; no process launch, migration, or request replay is added.

* Task observers now use the existing bounded fresh-transport validation when a pinned stream returns Unavailable (or ends). One retry without any intervening delivered event is allowed; failure is emitted and the observer closes instead of spinning. Initial open Unavailable also validates before retrying this read-only observation. Unpinned watcher behavior is unchanged. Progress cursors still advance only after enqueue into the bounded FIFO.
* A FinalResponse is the definitive chat result: server `sendAttachedEvent` maps runner EventDone to FinalResponse, and the client maps it to TypeDone. Subsequent Unavailable repairs the connection without claiming delivery is unknown. Repair failure remains visible as a completed-turn connection error. Tokens/progress are not completion. No chat replay is performed.
* Validation compares host instance, contract, schema, retired state, and retained conversation/ownership epoch. There is no independent store attestation field and build identity is not compared.

## Executed probes

Red before fixes:

* `go test ./pkg/agentclient -run TestPinnedCompletedChatRepairsWithoutDeliveryWarning -count=1`: completed turn incorrectly reported delivery unknown.
* `go test ./pkg/agentclient -run TestPinnedTaskOnlyRecoveryBounded -count=1`: real task-only loopback stop/rebind to host B retried stale transport until the five-second test deadline rather than exposing validation refusal.

New loopback coverage includes ordered progress cursor continuation (sequences 1,2), completed response then Unavailable, partial token then Unavailable, authentication/permission refusals without validation/replay, and a real driver response passed through Model.Update and its commands until stream end, with warning retained and rendered and server submission count exactly one.

Targeted race/repeat commands (run from the respective module):

```
go test -race ./pkg/agentclient -run 'TestPinned(Task|Completed|ChatAutomatically|Partial)' -count=5 -timeout=90s
go test -race ./internal/ui -run TestMainDriverPinnedRecoveryDoesNotResubmit -count=5 -timeout=60s
```

The broader `go test -race ./pkg/agentclient -run TestPinned -count=5` invocation exceeded the external 120-second tool timeout; it is not reported as a pass. The non-race TestPinned selection passed three repeats before the last partial/auth tests were added. Additional fresh synchronous chat-open failure, shared failed-attempt concurrency, and automatic contract/schema replacement-specific probes were not added in this follow-up. Task stop/rebind refusal and cursor continuation are separate probes, not a claim of a single end-to-end durable-store restart test.
