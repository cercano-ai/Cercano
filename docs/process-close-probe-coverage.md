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
**Final correction and independent combined review passed: READY TO LAND (not landed).
Retirement/lifecycle approval supplied with this task is preserved; its production
files are unchanged since f8b0b0e5. See final correction verification below.**

## Reviewed blockers addressed by the correction (retirement/lifecycle approved)

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
| `TestPinnedChatAutomaticallyRevalidatesWithoutResend` and agentclient suite | Fresh identity/session validation after transport loss; exactly one chat request, no ambiguous resend | Separate client transport scenario, not transparent replay in process journey. |
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

## Final reconnect integration verification

Validated code HEAD: `e6441d7de6c55f9220f070a337669e4b692516e3` on
`integration/combined-close-resume`, starting at
`f8b0b0e5145449326af5997fc410d4558c9dca5b`. Incremental reconnect
`1ac4e5739db850ee51c919b062b0b6fac9d1437d` cherry-picked without conflicts;
base b360 was not reapplied. No conflict resolution or production integration
edits were needed. Changes are client SDK, CLI tests and documentation only.
No landing or push performed. This earlier verification is superseded by the final correction review below.

| Reconnect scenario | Validated evidence | Boundary |
| --- | --- | --- |
| Exact validation | Host instance, contract, schema version, retired state, retained conversation and ownership epoch; existing connect-intent credential/isolation policy | No independent store attestation; build identity is not compared. |
| Task-only recovery | `TestPinnedTaskOnlyRecoveryBounded`, `TestPinnedTaskRecoveryPreservesCursor`: real loopback stop/rebind refusal, bounded retry, ordered progress sequences 1,2 | Stop/rebind refusal and cursor continuation are separate fixtures, not one durable-store restart journey. |
| Ambiguous/intermediate-final chat | `TestPinnedChatAutomaticallyRevalidatesWithoutResend`, `TestPinnedPartialAndAuthBoundaries`, `TestPinnedIntermediateFinalStreamOutcome` | FinalResponse may precede autonomous continuation. Only normal EOF establishes normal stream completion; non-EOF Unavailable remains uncertain with successful or refused repair, without resend. Authentication/permission failures remain visible without repair. |
| Identity/policy/stale epoch | `TestPinnedAttachmentTransportDrop` A/B/stale/credentials, `TestPinnedNoLaunchRefusals`, authenticated reconnect and DialExisting suites | Real transport with fake identity/session service; stale state represents epoch rejection, not actual ownership migration. |
| Cancellation/close concurrency | `TestPinnedConcurrentRecoveryAndCancellation`, `TestPinnedCloseDuringValidationCannotInstall`, attachment cancellation; full focused broad race | Shared failed-attempt concurrency remains without its own dedicated stress probe. |
| Actual CLI command loop | `TestMainDriverPinnedRecoveryDoesNotResubmit` drives Model.Update commands through stream end, retains/renders warning, exactly one submission | In-process model, not installed terminal/launcher acceptance. |

Final commands (all exit 0), from server module:

```sh
go test ./internal/server ./internal/worker -run '^TestProcessClose' -count=1 -timeout=180s
go test -race ./internal/server ./internal/worker -run '^TestProcessClose' -count=1 -timeout=300s
go test ./pkg/agentclient -run 'TestPinned|TestAuthenticatedPinnedReconnect|TestDialExisting|TestAttachmentCancellation' -count=1 -timeout=300s
go test -race ./pkg/agentclient -run 'TestPinned|TestAuthenticatedPinnedReconnect|TestDialExisting|TestAttachmentCancellation' -count=5 -timeout=300s
go test ./... -count=1 -timeout=300s
go test -race ./internal/server ./internal/worker ./internal/runner ./internal/hostsvc/... ./internal/conversation ./pkg/agentclient -count=1 -timeout=300s
go build -o /tmp/cercano-combined-final/cercano ./cmd/cercano/
```

From CLI module:

```sh
go test ./internal/ui -run TestMainDriverPinnedRecoveryDoesNotResubmit -count=1 -timeout=120s
go test -race ./internal/ui -run TestMainDriverPinnedRecoveryDoesNotResubmit -count=5 -timeout=180s
go test ./... -count=1 -timeout=300s
go build ./...
```

Exact captured test output is preserved in `verification/combined-final/`.
Process-close normal server/worker: 8.811s/0.485s; race: 24.684s/2.062s.
Pinned normal: 25.301s; five-repeat broad pinned race: 243.143s.
This replaces the previously unclassified external-120s timeout with a completed
pass (Go timeout 300s; external tool timeout 1000s for that command group).
No hang occurred, so no stack investigation or fixture fix was required.
Remaining command groups used external 900s deadlines, exceeding Go timeouts.
Broad focused race server/worker/agentclient: 107.314s/65.820s/100.037s.
CLI Model.Update normal/race: 0.488s/7.391s. Both full normal suites and
production builds passed; builds emitted no output.

Prior process-fixture boundaries above remain unchanged. Additional synchronous
chat-open failure and automatic contract/schema replacement-specific probes remain
uncovered by dedicated follow-up tests. Generic unary RPCs do not automatically
retry; initial attachment-open/takeover is not blindly replayed; budget exhaustion
surfaces an error. No live hosts/configuration/data/signals were used; only disposable
test fixtures. No other worktrees were modified or cleaned.

## Final narrow correction: independent verification

Reviewed and tested code SHA: `62ee7901d459eb2591169ccb3a59e715fd408a1b`.
Correction `48aa767116e4e2ef2ebc1fe51943fbfee41325d9` cherry-picked cleanly;
patch IDs match (`6392de30bbab65f84f211fcaa205e6fba2a47961`). Only the
expected two client production files, two tests, and coverage document changed.
No additional production changes. The following checkpoint adds documentation
and verification logs only, so its production/test tree is identical.

Independent review: no blockers. Server server.go:4400–4469 sends intermediate
FinalResponse before autonomous continuation. Client client.go:2274–2303 now
waits for EOF and exposes receive failures; pinned_session.go:175–183 retains
original and repair identities using multiple wrapping, repairs only Unavailable,
and never resubmits. Fresh connectIntent validates before transport installation;
identity/contract/schema/retirement/retained epoch checks remain enforced.
Unauthenticated/PermissionDenied are not converted into successful completion.
Task subscription review (tasks.go:20–124) retains bounded fresh validated
observation retry and cursor advancement only after FIFO enqueue; no changes to
this approved flow or new event suppression. Retirement/worker/server lifecycle
production diff against f8b0b0e5 is empty; the correction cannot change its
classification, accounting authority, budget or reaping behavior.

All final commands passed (exit 0), run on the code SHA above:

From `source/server`:
```sh
go test ./internal/server ./internal/worker -run '^TestProcessClose' -count=1 -timeout=180s
go test -race ./internal/server ./internal/worker -run '^TestProcessClose' -count=1 -timeout=300s
go test ./pkg/agentclient -run TestPinnedIntermediateFinalStreamOutcome -count=10 -timeout=120s
go test -race ./pkg/agentclient -run 'TestPinned|TestAuthenticatedPinnedReconnect|TestDialExisting|TestAttachmentCancellation' -count=5 -timeout=300s
go test ./... -count=1 -timeout=300s
go build -o /tmp/cercano-correction-build.M7hxwC/cercano ./cmd/cercano/
```
From `source/clients/cli`:
```sh
go test -race ./internal/ui -run 'TestMainDriverPinnedRecoveryDoesNotResubmit|TestMainAgentDriver|TestConn|TestPinned' -count=5 -timeout=180s
go test ./... -count=1 -timeout=300s
go build ./...
```

Process-close normal server/worker: 8.773s/0.911s; race: 25.451s/1.577s.
Matrix ten-repeat: 4.532s; broad pinned five-repeat race: 261.235s
(Go timeout 300s, outer 1000s). Other command groups had outer 900s limits.
Selected UI five-repeat race: 30.889s. Full normal server/CLI suites passed,
as did both production builds. Captured logs are in
`verification/combined-correction/` except process-close: its tests passed but
the initial tee destination was absent; the times above are from tool stdout,
not a saved log. No hang, failure, or production fix was needed.

Matrix covers immediate post-final loss and continuation-output loss, each with
successful/refused repair; multiple finals with normal EOF; post-final auth and
permission failure; original Unavailable and ErrPinnedSession identities; and
one submission. Actual Model.Update command-loop cases retain and render the
warning (ANSI/line-wrap normalized) with one submission. Reported prior red
results against 1ac4 were not rerun in this independent final verification.

Residual boundaries: intermediate-final fixtures use real ephemeral loopback
RPCs with Unavailable trailers, not a killed production host. Task stop/rebind
refusal and cursor continuity are separate tests, not one durable-store restart.
In-process UI is not installed terminal/launcher acceptance. Existing process
fixture substitutions and uncovered dedicated synchronous-open/shared-failed-
attempt/automatic contract-schema replacement probes remain as documented above.
No full-repository race suite claimed. Only disposable test processes/resources
were used; no live host state, landing, push, or worktree removal.
