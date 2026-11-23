# Combined close / retirement / resume coverage

Integration base: `45a1c3d7` (main). Scoped cherry-picks: `8c8c0a18`,
`ac1fd960`, `9a3aaffe`, `7ef1f492`, `6adc6103`, `80bb1697`, `00baa19a`,
`b360424d`, `cbc09573`; shared ancestors included once. No textual conflicts.
Bounded retirement supersedes join-before-cleanup without releasing callback authority.
Incremental correction `567d4aab18e375b84ea39ab96b0cdd4a92f8e4f4`
was applied as `365ac08a` on integration HEAD `3fb4b6d4`, without textual
conflicts. The accounting auto-merge was reviewed: process-local `source` session
identity and privacy-safe diagnostic correlation remain intact. No additional
production integration changes were necessary; same-child observable Wait,
per-session diagnostic connections and exact stale Chat admission refusal remain.
**Ready for final independent combined review, not landing-ready: retirement
re-review pending; separate reconnect review still pending.**

## Reviewed blockers addressed by the correction (independent re-review pending)

| Reviewed blocker | Correction / regression evidence | Review boundary |
| --- | --- | --- |
| Cancellation could downgrade protocol/authority rejection to ordinary retirement while callbacks retain authority | Explicit unsafe default; only actual cancellation outcomes select ordinary retirement; authority/protocol rejection escalates before callback joins. `TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure/authority-cancel` and `TestRetirementUnsafeWinsCancellation` exercise containment. | Integration verification passed; not an independent approval. |
| Terminal accounting ownership either resurrected or retained dead connections indefinitely | Exact connection closes under registration synchronization before session/state deregistration; permanent gRPC Shutdown rejects reuse; injected ownership cleared. `TestRetirementAccountingRegistryBounded`, `TestRetirementAccountingEnsureTerminalRace`, terminal-resurrection and timeout probes cover lifecycle. | No retained manager tombstones; session diagnostic metadata preserved. |
| Startup failure must reap its own child | `TestSpawnWorkerStartupFailureReapsExactChild` covers cancellation and early exit with temporary children. | Supplementary regression, not installed-launcher acceptance. |

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

Re-executed after incremental correction (all passed, no test failures).
Containment/accounting/startup selection: normal 10 repetitions (124.111s),
race 5 repetitions (64.324s). Combined ProcessClose: normal 3 repetitions
(server 24.820s, worker 0.810s), race 3 repetitions (server 71.916s,
worker 1.510s). Full server and CLI suites and production builds passed.
Broader focused race suite also passed (server 107.801s, worker 67.893s,
agentclient 93.442s; runner, hostsvc and conversation passed).

From `source/server`:

```
go test ./internal/worker -run 'TestRetirement|TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure|TestSpawnWorkerStartupFailureReapsExactChild' -count=10 -timeout=300s
go test -race ./internal/worker -run 'TestRetirement|TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure|TestSpawnWorkerStartupFailureReapsExactChild' -count=5 -timeout=300s
go test ./internal/server ./internal/worker -run '^TestProcessClose' -count=3 -timeout=120s
go test -race ./internal/server ./internal/worker -run '^TestProcessClose' -count=3 -timeout=300s
go test -race ./internal/server ./internal/worker ./internal/runner ./internal/hostsvc/... ./internal/conversation ./pkg/agentclient -count=1 -timeout=300s
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
