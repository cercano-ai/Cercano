# Execution notes

## Initial regression slice

Implementation worktree: `/Users/bryancostanich/git_repos/cercano-ai/Cercano-independent-subagent-lifecycle`, branch `feat/independent-subagent-lifecycle`, based on planning commit `909704e57865`.

Added `source/server/internal/server/agentic_lifecycle_test.go`, checkpoint `a69663c86535`. The deterministic blocking child tool observes cancellation of its execution owner. This is a characterization of the shared synchronous primitive, whose cancellation support must remain intact; it is not yet coverage of independent public dispatch.

Passed checks:

- Server: `TestRunAgenticDispatch_RTierDefault`.
- Server: `TestAgenticExecutionHonorsOwnerCancellation`.
- Worker: `TestWorkerRunner_ContextCancel`, `TestBuildWorkerToolSvc_WiresDispatch`, `TestDispatchEventProxy_Cancellation`.
- CLI: `TestCancelCurrentStreamSilently_FinalizesChildTabsBeforeNewTurn`, `TestCancelCurrentStreamWithNotice_FinalizesChildTabs`, `TestSubAgentTerminalEvent_ResolvesStaleInProgressTool`.

The existing CLI tests still require child finalization on main cancellation; those expectations will change only when independent task ownership is wired. No production behavior has changed.

## Trace findings

The shared `capabilities.Services.Dispatch` hook also serves review, research, Git workflows, and other synchronous consumers. Preserve their result contracts when the public dispatch/workflow interface changes.

The worker builds its own tool service and dispatch engine; its dispatch-event persistence proxy is scoped to the worker stream and known conversation identities. Independent public task launch must reach host-owned execution rather than leave children inside the originating worker's lifetime.

Conversation persistence owns child identity, transcript history, and dispatch events. Task terminal state and notification handling must complement these records, not infer liveness from child-tab dismissal or transcript presence.

Main-turn orchestration is in `server.go:streamProcessRequestWithToolLoop`. It calls BeginTurn with the initiating stream context and creates a stream-bound permission requester. The requester sends PermissionRequired directly on that stream, then waits on the permission broker. AttachConversation is explicitly observe-only and does not currently supply a replacement approval path.

## Decision requiring user input

Automatic completion wake-up when disconnected creates a main-agent turn with no initiating approval stream. Existing grants must remain authoritative and no new action may be implicitly approved. For newly gated actions, choose between retaining a pending approval for reconnection (suspending execution until approval) and returning an explicit approval-unavailable result (requiring a later retry).

Approved by the user: retain a pending approval for reconnection, without executing the gated action. This retains intent and requires reconnectable permission-request lifecycle behavior. Existing grants remain unchanged; disconnected execution never implies approval. Implementation resumed after this approval.


## Durable task foundation and approval barrier

Checkpoint `57f2a6df62e1` adds host-owned task-manager and SQLite task-state foundations. Durable acceptance precedes launch acknowledgement; task identity and reserved child identity survive restart. Terminal outcomes and pending notifications are written together. Wait cancellation does not cancel execution, explicit stop is parent-scoped, shutdown interrupts execution, and restart reconciliation never replays tools. This is not yet wired into public dispatch, the host lifecycle, or worker transport.

Race-enabled full suites passed for `internal/taskrun`, `internal/conversation`, `internal/dispatch`, and `internal/hostsvc/tools`.

Checkpoint `93eb0c88e389` fixes a reproduced duplicate-approval deadlock and adds registration-before-notification support. Focused race-enabled agent/server approval tests and the full permissions broker suite passed. Reconnect prompt retention/delivery itself remains unwired.

## MCP task-control authorization boundary

`mcpadapter.makeHandler` sends capability name and raw arguments to InvokeCapability. `hostsvc/tools.Service.InvokeCapability` constructs a capability Call without ConversationID, and delegates permission gating to the external MCP host. `TestInvokeCapabilityDoesNotBindConversationFromArguments` passed, confirming that even a supplied conversation_id is not a host-bound conversation identity.

New task inspection and stop controls therefore cannot apply the internal agent's parent-conversation authorization rule unchanged on the MCP surface. A security policy decision is required before wiring this interface: bind MCP calls to a host-established conversation/session identity, or explicitly treat the local MCP client as a host-wide operator allowed to address tasks across conversations under its existing external permission gate. The latter must not be inferred merely from caller-supplied identifiers. Recommendation: conversation-scoped binding; additional session-binding implementation is needed.

No task-control or asynchronous dispatch capability has been exposed yet. Existing public dispatch remains synchronous.

### Follow-up: conversation binding transport

The user directed investigation of the actual host binding rather than requesting that information from them. The MCP host uses shared per-server connections, not per-conversation connections. Although the tool loop sets a conversation ID in its local Go context, `mcp_host.conn.call` sends only tool name and arguments. The receiving adapter forwards only capability name and arguments. A transport session is therefore not a reliable conversation scope.

`TestConversationScopeProbe` (checkpoint `74ddb63a6603`) observes two conversation contexts on one connection and reconnects the first: all three calls arrive without metadata, a conversation ID, or a stream-transport session ID. This does not establish behavior for every external MCP client.

The server scope fixture had a string/pointer mismatch; correcting its observation field and assignment makes its intended assertions execute. Race-enabled focused checks now pass for `TestMCPConversationScope*`, `TestInvokeCapabilityDoesNotBindConversationFromArguments`, and `TestConversationScopeProbe` in the server and MCP-host packages.

The user has now explicitly selected per-chat isolation: each external chat may see and manage only the background agents it launched. Application-wide ownership is not allowed. This resolves the ownership-policy question; do not ask it again. A model-supplied conversation_id and a per-connection random ID cannot substitute for a trustworthy per-chat binding when a connection is shared. Existing unrelated synchronous tools need not change.

### Per-chat isolation regression coverage

`TestTaskAccessIsIsolatedByChat` now exercises two owners against the real task manager and SQLite store. Each owner lists only its own task; foreign inspect, wait, and stop return not-found without leaking a record or result. The checks run both during execution and after terminal persistence. Both tasks complete successfully despite foreign stop attempts. `go test -race ./internal/taskrun ./internal/conversation` passed. This proves manager-level isolation only; the plan's authorization task remains in progress until transport binding and public controls enforce the same boundary.

The external-client transport implementation remains to be established. The existing probe demonstrates that Cercano's current MCP transport drops its local chat context; it does not prove that every external client supplies a usable chat identity. Do not claim external per-chat isolation is implemented merely by adding request metadata. Native agent calls already carry host conversation context. Preserve that trusted scope, and require evidence of an external integration's binding before exposing cross-request task access through it.

## Approved unbound-client policy and task controls

The user approved failing closed for asynchronous task operations when a client lacks trustworthy per-chat identity. Missing binding is an integration failure, not normal operation. Do not silently fall back to synchronous dispatch or infer identity from tool arguments or shared transport sessions. Unrelated synchronous capabilities remain unchanged.

Added a separate capabilities.Tasks interface and host task-service adapter, plus task_list, task_inspect, task_wait, and task_stop. Controls require surface-injected ConversationID; a model-provided conversation_id is never used for ownership. Wait is bounded and does not stop execution. Public dispatch itself still uses the synchronous hook pending worker/launch integration. The worker task proxy and external trusted binding remain unwired.

Verified with `go test -race ./internal/capabilities/... ./internal/toolstack ./internal/server ./internal/taskrun`. Tests cover unbound external RPC rejection with no result payload, active/completed foreign-task rejection, and wait cancellation without child cancellation.

## Worker task-control bridge

Added dedicated TaskControlRequest/Response protobuf envelopes and a worker Tasks proxy. The host takes parent ownership from the active worker request, never from tool-supplied ownership fields. Launch transfers only the public task/grant/routing inputs; reserved child IDs and progress callbacks do not cross the boundary. The task manager remains host-owned. Wait requests have a bounded timeout and stream cancellation does not cancel accepted child execution.

`go test -race ./internal/worker ./internal/server ./internal/capabilities/... ./internal/taskrun` passed. The bridge test exercises protobuf round trips, launch/inspect/list, foreign-access rejection, and task completion after the initiating stream disappears. This is not yet full subprocess-death coverage. Public dispatch still awaits the asynchronous contract change; completion wake-up remains pending.

## Public asynchronous dispatch

Public dispatch (and its workflow synonym) now uses Tasks.Launch and returns task_id, child_id, and accepted status, without holding a main-turn progress callback. A conversation_id argument can only match the already-bound owner; it cannot establish identity. The synchronous Services.Dispatch contract remains unchanged for review, research, and other synchronous consumers. Terminal result metadata, tool evidence, suspicious-result flags, and partial errors remain in the persisted task result rather than the launch response.

Race-enabled capability, worker, server, host-tool-service, task-manager, and toolstack suites pass. A real bufconn worker plus deterministic HTTP provider test exercises public dispatch, confirms a write-grant approval, returns from the main turn while the child is blocked, and observes the child's later terminal result. External MCP launch is intentionally unavailable until its trusted chat binding is implemented. Automatic completion delivery remains pending.

## Idle turn arbitration and unresolved external integration

Added Broker.TryBeginTurn for atomic idle-only acquisition. Unlike a HasActiveTurn/BeginTurn pair, it cannot cancel intervening user work. Rejected claims preserve replay and generation; a later human turn still supersedes the background turn. Race-enabled broker/server suites pass, including concurrent claim arbitration. Completion scheduling itself is not wired yet.

Correction after repository tracing: the missing binding in the existing stdio MCP path is not a reason to ask the user to identify a client or to block native lifecycle implementation. The native CLI/agent path already supplies conversation ownership; server.go owns main-turn execution and AttachConversation, and autonomy_continuation.go provides existing host-authored continuation machinery (currently confined to an open user-initiated stream, not yet a disconnected completion scheduler). mcp_host.conn.call sends only tool name/arguments, while mcpadapter.makeHandler forwards only name/ArgsJson. These are concrete implementation boundaries to investigate, not evidence that every external client behaves the same way. Preserve the approved rejection of unbound task operations and the per-chat isolation requirement. External binding remains unfinished, not declared out of scope; native completion scheduling and reconnect delivery do not require another client-selection decision from the user.

## Approval publication and attachment replay

A deterministic immediate-reply integration test reproduced the server requester publishing PermissionRequired before registering the approval waiter. The permission broker now exposes the existing WaitWithNotify barrier, and the server registers before publication (checkpoint 6b0afa3e06f1).

A server-owned live-prompt registry now retains pending approvals separately from historical turn replay. AttachConversation subscribes to coalesced change signals and snapshots live prompts, so a reconnecting observer sees pending approvals without implicitly granting them. Prompt removal follows requester completion/cancellation or publication failure. Tests cover defensive snapshots, chat isolation, observer disconnect/reconnect, explicit approval, and cleanup. This does not yet detach the original human turn from its client; pending approval retention for disconnected host-generated turns still depends on the completion scheduler integration. Race-enabled server, permissions, worker, and runner suites pass.

## Host completion wake-up and safe turn boundaries

Implemented per-chat coalesced completion scheduling from durable task callbacks and main-turn release. Idle acquisition uses TryBeginTurn, never turn replacement. Completion turns run without a CLI stream through the shared main runner and use system-role task metadata, bounded to 64 notices per batch; results remain in task inspection rather than being injected as instructions. New user input can supersede these turns. Claims and handled state remain separate: successful current turns acknowledge their batch, while interrupted claims remain inspectable and are supplied at a later human-driven turn boundary rather than automatically retried forever. Busy main turns also receive notices at a successful turn boundary.

Disconnected turns do not clear explicit autonomy blockers or begin an autonomous chain. Permission requests use the existing gated requester and live prompt replay. Added explicit background-work admission wired to enterpriseHost.Begin in the production bootstrap, because the gRPC policy interceptor does not run for host-generated work. Policy denial leaves notices unclaimed, and admission lifetime covers inference.

Verification: race-enabled server, taskrun, broker, worker, permissions, and conversation suites pass; cmd/cercano tests pass. New deterministic tests cover busy non-interruption, coalescing, supersession without retry loops, recovery of claimed notices on a later human turn, disconnected approval/reconnect, policy denial/context retention, and callback-to-main-turn delivery. Remaining acceptance work includes real worker-main completion integration, CLI independent tab/event lifecycle, external transport binding, and the broader restart/resource cleanup audit.

## CLI task-status restoration and cancellation cleanup

Completed the partial child-list task metadata implementation already present in the worktree. A failing database-backed test showed that the listing service used a child conversation ID as a task handle, dropping status metadata. Added a parent-scoped GetTaskByChild lookup and coverage of differing task/child IDs, nested parent ownership, and foreign metadata isolation.

The CLI now applies one task-lifecycle helper on restoration, retains sparse host-owned children, and does not infer terminal status for unknown task states. Parent cancellation preserves known task-owned children while retaining legacy activity cleanup. Terminal metadata clears streaming and unresolved tool spinners. Regression tests exercised the actual gRPC child-list/resume path rather than only assigning tab fields.

Verification: full race-enabled CLI UI suite passes, as do conversation, persistence, server, taskrun, worker, and agentclient suites. This is status restoration, not independent live progress subscription: live event delivery and fresh-launch discovery remain unfinished, as do external trusted binding and the final acceptance audit.

## Approved independent task subscription

Implemented the user-approved SubscribeTasks RPC, SDK subscription, and CLI observation path. The stream is parent-scoped and independent of main-turn generations. It sends fresh lifecycle snapshots and immutable dispatch-progress records with per-task cursors. Database reads use bounded 128-row pages; a 200 ms durable-tail interval supplies live updates at persisted model-response/tool boundaries, not token-by-token. Model-request and summarizer internals are not included in the progress feed. Slow observers are backpressured rather than queued without bounds. Completed, drained children stop generating progress queries; lifecycle scanning remains proportional to the parent's retained tasks.

The SDK reopens observation with delivered cursors and relies on the standing connection watcher for shared reconnection, so canceling one chat observer cannot mark the whole client failed. CLI chat switches cancel and generation-fence the old subscription. Replayed records are deduplicated, restored transcripts are replaced once before replay, and task status—not parent stream completion—controls child liveness. Persisted dismissal is honored without stopping execution.

Verification: real gRPC/SDK tests cover parent-turn replacement, live progress, owner isolation, observer cancellation, paginated snapshots, dismissal, and cursor replay. SDK tests cover reconnect cursor advancement and cancellation without shared-connection failure. UI tests cover post-cancellation progress, duplicate suppression, terminal status, dismissal, and stale chat generations. Race-enabled server/conversation/taskrun/worker/persistence/agentclient and full UI suites pass; cmd/cercano tests and CLI module build pass. Remaining acceptance items include worker-main wake-up integration, background main-response presentation, external trusted MCP binding, restart/resource audit, and documentation/consumer migration.

## Restart observation acceptance and consumer guide

Added a disk-backed reopen integration test: seed running work plus durable evidence, close/reopen SQLite, construct a fresh task manager, and observe through the real gRPC/SDK subscription. The task is reported interrupted, its earlier progress is replayed, and the executor counter remains zero. The race-enabled subscription test group passes.

Added docs/agent/independent-tasks.md and linked it from the agent README. It documents acceptance receipts, explicit inspect/wait/stop, cancellation versus dismissal, no automatic restart replay, ownership requirements, progress granularity, and outstanding acceptance work.

Main-response presentation trace: completionTurnStream.Send currently consumes the final response only to mark successful handling; runner events reach the conversation broker. The CLI has no AttachConversation subscription and SubscribeTasks presently carries child state/progress only. Consequently automatic main-agent follow-up is not delivered through the new CLI task stream. This is a distinct integration gap from child progress and must not be treated as completed merely because the host ran the wake-up.

## Approved main-conversation attachment delivery

The user approved receiving automatic parent replies through the existing main-conversation attachment, retaining the separate child-task subscription. A failing regression proved that completionTurnStream discarded the final response instead of publishing it for attachment. The host now publishes background finals (and a non-success notice on failed handling), tags broker events with a host epoch, turn generation, sequence, and background flag, and carries pending approval ownership metadata. Attachment observers use bounded pages from the existing replay buffer and coalesced change signals rather than a drop-on-full queue.

The SDK shares response decoding with normal chat, resumes attachment cursors, and tracks current turn identity. The CLI scopes attachment to the selected chat, ignores ordinary request-stream duplicates, renders background replies in separately owned transcript state, deduplicates approval prompts, and prevents delayed background tokens from modifying foreground text. Another failing regression exposed overlap with restored history; completed replay now carries the existing saved turn ID so it can be suppressed without text matching or a schema migration.

Verification: real host/gRPC/SDK automatic-final test (including saved identity), 300-event paged replay and cursor test, UI foreground isolation/replay/approval/chat-fence tests, and saved-history deduplication regression pass. Race-enabled server, broker, runner, worker, agentclient and full UI suites pass; cmd/cercano tests and CLI build pass. Remaining work is acceptance coverage and consumer/resource auditing, not another choice of transport.

## Final acceptance audit

Added actual worker-protocol main-turn completion coverage while no CLI is connected, including subsequent SDK attachment of the automatic reply. Strengthened the public worker launch test by stopping its originating worker service before allowing the host-owned child to finish. Added concurrent stop/completion evidence-retention and handle-release coverage, backpressured observer cancellation, attachment reconnect cursors, and typed-input preservation.

The approval audit reproduced an unsafe UI recovery assumption: a background approval was being treated as permission to replay an unrelated earlier foreground request. Background approvals now retain their original tool-use identity on reconnect; a restarted host rejects an expired waiter instead of re-running work. Its regression and the complete UI suite pass with the race detector. Completed background render buffers are released while sequence metadata remains for duplicate suppression.

Updated runtime steering/Git protocol text and agent/CLI guides so public dispatch receipts are not mistaken for final results. Internal synchronous dispatch-service consumers remain unchanged. Unbound external MCP clients continue to fail closed, as approved; no host-wide authorization fallback was introduced.

The full evidence table and precise test limitations are in acceptance.md. Broad affected server suites pass with the race detector, the full CLI UI suite passes with the race detector, both modules build, command tests pass, and protobuf regeneration is byte-identical. No paid inference or manual production terminal smoke test was used. All plan tasks are reconciled to this evidence; all work remains unpushed in the implementation worktree.


## Ordinary failed completion followup ownership policy (2026-11-25)

Approved narrow correction: after execution and callbacks join, an ordinary model
error or returned cancellation is not persistence uncertainty. Normal close/reopen
is allowed while notification_pending and wake_claimed both remain true. Resume
does not retry claimed work; a later human-driven boundary acknowledges only after
successful handling under the current owner. Failed mutations and unknown teardown
remain ownership fences even when cancellation also occurs. No reset/requeue API.
Regression and historical characterization, commands, and limits are recorded in
`docs/real-completion-close-investigation.md`; the historic incident ticket origin
remains unproven. Await independent review before landing/pushing.
