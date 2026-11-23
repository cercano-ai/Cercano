# Combined close / retirement / resume coverage

Integration base: `45a1c3d7` (main). Scoped cherry-picks: `8c8c0a18`,
`ac1fd960`, `9a3aaffe`, `7ef1f492`, `6adc6103`, `80bb1697`, `00baa19a`,
`b360424d`, `cbc09573`; shared ancestors included once. No textual conflicts.
Bounded retirement supersedes join-before-cleanup without releasing callback authority.
**Not landing-ready: independent bounded-retirement and reconnect reviews pending.**

| Scenario / tests | Real paths exercised | Substitutions / limits |
| --- | --- | --- |
| `TestProcessCloseInstalledCallbacks/blocked-success` | ConfigurePreparedWorker; autonomy callback; authenticated pinned source close; real Unix pooled child; cancel active turn; owned Wait of captured same child PID within 4s accounting budget + 1s scheduling/reap allowance; original owner/epoch retained and successor refused while callback blocked; successful SQL completion then release; independent replacement host explicitly pins new epoch and ResumeConversation reads transcript | Test executable is policy worker binary, not installed launcher. Store decorator deliberately ignores cancellation until barrier opens. Turn cancellation is test-driven after close, not CLI cancellation. |
| Same test: autonomy INSERT/UPDATE and route success / write-failure | Real SQLite rows or abort triggers; same child Wait before authority assertions; failure retains original owner/epoch and refuses successor even after exit | Route uses dummy secret and invalid endpoint; no inference. Success response received before turn completion; not lost-response coverage. |
| Same successful journeys: stale Chat and reconnect | Actual authenticated pinned Chat RPC to closed source denied with exact admission-busy status; no input persisted; explicit reconnect rejects stale binding; replacement authenticated resume on same store | Source admission denies before pin validation; does not demonstrate stale Chat to replacement endpoint or full CLI rendering. |
| `TestPreparedWorkerRealMutationLostResponse` | Real serverTasks.Launch, SQL mutations, EOF/cancellation response loss, mutation accounting | Observation wrapper; process journey task row remains fixture-created, and its policy task adapter is not installed task-launch wiring. No claim of end-to-end delegated task launch. |
| `TestProcessCloseWaitResult`, `TestRetirement*` | Kill initiates owned asynchronous reaper; waitReaped joins exact PID; bounded detach/kill, accounting deadlines, displaced workers | Separate unit/process probes supplement, not substitute for same-journey Wait above. |
| `TestPinnedChatAutomaticallyRevalidatesWithoutResend` and agentclient suite | Fresh attestation after transport loss; exactly one chat request, no ambiguous resend | Separate client transport scenario, not transparent replay in process journey. |
| Lifecycle / accounting diagnostic tests | Original owner/epoch, worker connection identity, safe reasons, privacy assertions | Each terminal accounting session needs its own connection; no raw credentials/paths in diagnostic output. |

All processes, signals, stores, sockets, tokens and configurations are test-owned.
The process observer captures a pooled handle without killing/detaching it and waits
on its owned reaper channel; ProcessState is read only after Wait completion.
No installed launcher or full CLI rendering acceptance is claimed. Installed task
launch remains covered separately: replacing the policy adapter here would change
this deterministic worker protocol fixture, rather than prove installed CLI use.

## Executed verification

From `source/server` (all passed after integration adjustments):

```
go test ./internal/server ./internal/worker -run '^TestProcessClose' -count=3 -timeout=120s
go test -race ./internal/server ./internal/worker ./pkg/agentclient -count=1 -timeout=300s
go test ./... -count=1 -timeout=300s
go build -o bin/cercano ./cmd/cercano/
```

From `source/clients/cli`:

```
go test ./... -count=1
go build ./...
```

Integration adjustments: old Kill test assumed synchronous Wait; it now joins the
owned reaper. Diagnostic test reused a connection that bounded retirement now
closes; each session now creates its own, preserving all privacy/correlation
assertions. Initial stale-Chat probe expected a client pin error; actual closed
source denies admission first, now asserted as the precise server error plus no
persisted input. Initial full race run failed only the reused-connection test;
subsequent full focused race, server suite and CLI suite passed.
