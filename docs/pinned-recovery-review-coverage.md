# Pinned recovery review follow-up

Client/CLI only; no process launch, migration, or request replay is added.

* Task observers now use the existing bounded fresh-transport validation when a pinned stream returns Unavailable (or ends). One retry without any intervening delivered event is allowed; failure is emitted and the observer closes instead of spinning. Initial open Unavailable also validates before retrying this read-only observation. Unpinned watcher behavior is unchanged. Progress cursors still advance only after enqueue into the bounded FIFO.
* FinalResponse can end an intermediate autonomous turn, not the request stream. Only normal EOF proves normal transport stream completion. Every non-EOF receive error remains visible, including immediately after FinalResponse; Unavailable repairs the freshly attested connection for future use but retains uncertainty and never replays chat. Repair failure retains both the original transport error and repair error identities.
* Validation compares host instance, contract, schema, retired state, and retained conversation/ownership epoch. There is no independent store attestation field and build identity is not compared.

## Blocking intermediate-FinalResponse correction

The former Completed test encoded an incorrect terminal-event assumption and has been replaced, not counted as blocker coverage.

Against production code at `1ac4e5739db850ee51c919b062b0b6fac9d1437d`, the new `TestPinnedIntermediateFinalStreamOutcome` failed all four Unavailable cases: immediately after FinalResponse and after continuation output, each with successful/refused repair. Successful repair swallowed the error; refused repair falsely claimed completion and lost the original status. The new actual Model.Update regression also failed all four corresponding cases against that production code (warning absent). Both use real ephemeral loopback gRPC servers, not live hosts.

Green commands run from the respective modules (outer timeouts 240–300s):

```
go test ./pkg/agentclient -run TestPinnedIntermediateFinalStreamOutcome -count=10 -timeout=120s
go test -race ./pkg/agentclient -run TestPinned -count=5 -timeout=180s
go test ./pkg/agentclient -count=1 -timeout=180s
go test -race ./internal/ui -run 'TestMainDriverPinnedRecoveryDoesNotResubmit|TestMainAgentDriver|TestConn|TestPinned' -count=5 -timeout=180s
```

All passed. Client matrix also covers normal EOF with two FinalResponses, and Unauthenticated/PermissionDenied after FinalResponse without repair. Every matrix case asserts exactly one submission; repair failure retains original status and ErrPinnedSession. UI cases execute driver commands through Model.Update and assert warning retention/rendering and exactly one submission. Render assertions normalize ANSI/line wrapping.

Task-only recovery and progress cursor code is unchanged. Limitations: fixtures return gRPC Unavailable trailers rather than killing a live production host; no full repository/UI suite or durable-store restart integration was run. Existing task stop/rebind refusal and cursor continuation remain separate probes. No live host/config/data operations, protocol changes, landing, or push.
