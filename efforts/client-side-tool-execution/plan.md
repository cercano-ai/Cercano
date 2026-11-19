# Client-side workspace execution

Implement the approved spec in this directory. Spec approved 2026-11-18; this execution plan awaits approval. Preserve host execution, keep orchestration on the host, and never substitute host execution for unavailable client execution. Containers, workspace synchronization, machine handoff, and a persistent client daemon are out of scope.

Execute phases in order. Delegate bounded mechanical extraction, path inventories, and independent review with explicit scopes. After approval, create an isolated feature worktree using the repository worktree workflow; do not perform substantial implementation on main. Checkpoint tested units with explicit paths and conventional commit subjects. Do not push. Exact new filenames and package placement below are implementation targets, not claims that those files already exist. If the initial inventory exposes a consequential architecture choice, stop for human review before implementing it.

## Phase 1 — Close the routing inventory and establish baselines

Objective: establish every workspace access path and the concrete dependency boundary before changing behavior. Files to inspect: source/server/internal/capabilities, internal/agent, internal/worker, internal/dispatch, internal/context, internal/mcp, internal/server, internal/gitflow; source/clients/cli/internal/ui; source/proto/agent.proto; both Go modules and applicable AGENTS.md files. Record the completed inventory in this plan during execution. Tests: existing targeted capability, context, worker, dispatch, server transport, and CLI lifecycle tests; add characterization tests for any undocumented behavior on which extraction depends.

- [x] Create the effort feature worktree after plan approval and read applicable repository instructions and executing-plans protocol.
- [~] Inventory registered operations and classify each as workspace execution, host orchestration, or a host-coordinated operation with workspace sub-operations.
  - [ ] Trace native capability, provider tool, MCP, InvokeCapability, legacy InvokeTool, worker, dispatch, and nested delegation entry points to actual execution.
  - [ ] Trace project instructions, workspace skills, project detection, repository discovery/state, checkpointing, and other filesystem/process access outside explicit tools.
  - [ ] Record file and symbol, location authority, permission boundary, required host services, target routing seam, and test for each path.
- [-] Verify authenticated connection identity, conversation persistence, ownership/reconnect controls, protocol generation commands, and current compatibility negotiation.
- [ ] Identify the shared workspace implementation package and dependencies: use an importable package outside server/internal, retaining host-service adapters inside the server; verify CLI/module build legality before extraction.
- [ ] Run targeted existing tests and record actual baseline commands/results; reproduce pre-existing failures separately rather than silently attributing them to this effort.
- [ ] Gate further implementation on a complete inventory and concrete dependency map. Return genuine new security, durability, or module-boundary forks for review; do not treat incomplete tracing as approval to guess.

Phase 1 execution note (2026-11-18): isolated worktree created on feat/client-workspace-execution at planning commit d3643ffef9e4. No implementation or tests have run. Audit remains partial. Verified ordinary DialExisting -> connect -> connectIntent in source/server/pkg/agentclient/client.go uses insecure.NewCredentials and only adds authentication for a pinned intent with pin.auth. Delegated security trace additionally located loopback-only authenticated pinned sessions in pkg/agentclient/pinned_session.go and shared per-host token checks in pkg/devhostauth/auth.go, neither establishing ordinary remote executor identity. This blocks enabling remote execution safely: authenticated transport and executor enrollment/authorization need explicit security-scope review before implementation. Other confirmed routing work includes local worker capability assembly and workerCtxHistory.LoadProjectContext in internal/worker/worker_dispatch.go; direct context/scanner filesystem reads; and externally gated InvokeCapability with allow-all RequestPermission in internal/hostsvc/tools/tools.go. Full inventory, persistence tracing, extraction dependencies, and baseline tests remain incomplete. Recommended shared-code placement is provisionally source/server/pkg/workspace, not yet dependency-verified.

## Phase 2 — Extract reusable workspace operations without behavior changes

Objective: let host and client use the same workspace implementations without importing host internals into the CLI. Files: importable workspace execution package selected in Phase 1, source/server/internal/capabilities/builtins adapters, context/Git helpers identified in the inventory, and necessary module declarations. Tests: operation parity, relative and absolute path behavior, working-directory isolation, command environment/output/cancellation, and Git fixture tests.

- [ ] Define minimal workspace operation input/output/progress interfaces with explicit execution context and no model clients, account credentials, conversation store, or host service registry.
- [ ] Extract filesystem, search, process, and Git implementation code into the shared package; keep capability names, schemas, permission metadata, and host orchestration wrappers compatible.
- [ ] Separate mixed operations such as checkpointing into host coordination and selected-workspace operations without weakening their approval rules.
- [ ] Expose project-context discovery/read primitives needed by host prompt assembly through the same workspace boundary.
- [ ] Keep all existing host execution routed through the host implementation and verify no host behavior change before enabling remote routing.
- [ ] Run shared-package and affected server tests plus CLI compilation; checkpoint the behavior-preserving extraction.

## Phase 3 — Define execution binding and protocol contracts

Objective: represent explicit workspace ownership and correlated operations over the existing gRPC service. Files: source/proto/agent.proto and repository-generated bindings, server conversation persistence and connection/authentication handlers located in Phase 1, shared protocol-facing types, CLI transport. Tests: persistence/migration, negotiation, identity validation, and state-transition unit tests.

- [ ] Define persisted conversation binding with host/client location, authorized executor identity, workspace identity/root, and ownership generation. Keep isolation a separate concept; do not claim container support.
- [ ] Preserve legacy conversations as host-bound without automatically migrating them; prevent changing an established binding through ordinary request work_dir fields.
- [ ] Define executor registration and reconnection on the existing authenticated interface, independent of conversation observation. Use the existing interface's streaming/control conventions; no new listener or port.
- [ ] Define request, acceptance, progress, terminal outcome, cancellation, acknowledgement, and reconciliation messages correlated by original execution ID, conversation, workspace, executor identity, and ownership generation.
- [ ] Distinguish stable machine authorization from process incarnation so a restarted process cannot falsely claim that it still owns an in-memory running command.
- [ ] Specify transitions for queued, accepted/running, awaiting reconciliation, cancel requested, terminal, and unknown/interrupted outcomes; a transport failure is not an execution failure.
- [ ] Negotiate client-execution support and operation availability. Old clients retain host mode; unsupported client execution fails explicitly rather than downgrading.
- [ ] Generate bindings using repository tooling and test affected server/client protocol compilation and compatibility fixtures.

## Phase 4 — Implement host routing and permission enforcement

Objective: keep orchestration on the host while routing every workspace operation through its bound executor. Files: server execution coordinator, capability adapters/services, tool loop, worker/dispatch paths, context assembly, MCP and external invocation handlers identified in Phase 1. Tests: fake-executor routing, nested delegation, permission denial, unavailable executor, and cross-client/conversation result rejection.

- [ ] Add the host execution coordinator with host-local and connected-client implementations; resolve binding before workspace access, including prompt preparation.
- [ ] Validate client registration against authenticated identity and explicit client authorization; observers must not acquire execution ownership.
- [ ] Apply existing host operation permissions before dispatch, and bind the authorization to the exact operation and execution identity. Preserve host-only permission and planning fences.
- [ ] Route native tools and supported external entry points through the coordinator; explicitly reject client routing where an entry point lacks sufficient authenticated context.
- [ ] Propagate binding through workers, dispatch, nested delegation, and autonomous continuations; never derive it from worker process cwd or an observing client.
- [ ] Route automatic workspace reads through the same selected executor while keeping prompt assembly, host configuration, credentials, conversation storage, and model calls on the host.
- [ ] Reject duplicate/unsolicited/stale results and mismatched ownership. Do not dispatch new client work while its required authorization or connection is unavailable.
- [ ] Add tests that fail if host filesystem/process helpers run for a client-bound operation; checkpoint routing with host-mode regression coverage.

## Phase 5 — Implement the client executor and reconnect reconciliation

Objective: execute authorized operations in the CLI process independently of observation streams and transient network connections. Files: CLI executor package outside UI rendering, CLI connection lifecycle, shared workspace package, host coordinator reconciliation handlers. Tests: deterministic disconnect barriers, lost acknowledgements, duplicate delivery, cancellation races, process-incarnation changes, and resource-pressure behavior.

- [ ] Implement a client executor supervisor whose operation contexts are not canceled merely because a stream or conversation observer disconnects.
- [ ] Register only explicitly authorized workspace capabilities; validate each incoming request's binding, generation, operation ID, and authorization before executing with the client's OS user/environment.
- [ ] Record acceptance before launching work and deduplicate requests for the same execution ID; identical duplicates return existing state, conflicting duplicates are rejected.
- [ ] Retain running state and terminal results until the host acknowledges them. Reserve bounded result capacity before accepting new work, refuse new work under pressure, and never evict an unacknowledged terminal outcome silently.
  - [ ] Reuse existing output limits where available; otherwise introduce named, tested limits with explicit truncation metadata and output-draining behavior so command pipes cannot deadlock.
  - [ ] Bound pending-record count and output bytes, coalesce nonterminal progress, and test exhaustion without losing execution identity or outcomes.
- [ ] On reconnect reconcile original execution IDs and replay only retained progress/results, not command execution. A lost acknowledgement must not repeat side effects.
- [ ] Track cancellation separately from disconnection; queued cancellation is applied on reconnect and remains pending/uncertain until the executor reports an outcome. Test completion winning a cancellation race.
- [ ] Handle client shutdown/restart honestly: preserve host-received results, mark unresolved prior-incarnation work unknown/interrupted, and never automatically retry. Do not promise orphan-process cleanup or command survival without evidence.
- [ ] Test host reconnect/restart against persisted binding and operation records: recover only provable outcomes and refuse automatic replay where acceptance/outcome is uncertain.
- [ ] Checkpoint executor and recovery implementation with targeted race-enabled tests for concurrent state where supported.

## Phase 6 — Expose explicit selection, consent, and execution status

Objective: make execution location and authority clear without changing the location of existing conversations. Files: CLI configuration/connection setup and source/clients/cli/internal/ui conversation, tool-event, approval, and lifecycle surfaces; relevant CLI/agent guides. Tests: UI model tests, transport integration, reconnect/resume, and multiple-observer fixtures.

- [ ] Add explicit host/client workspace selection for new conversations using existing configuration and UI conventions; require deliberate consent for direct execution on the client.
- [ ] Show selected machine/workspace and online, awaiting reconciliation, or unknown/interrupted status; label tool execution location and clarify the OS identity/environment used.
- [ ] Keep selection distinct from isolation and do not expose unimplemented container choices as working options.
- [ ] Ensure opening/resuming on a second client does not rebind execution or authorize that observer. Unavailable original executors remain unavailable; handoff is not offered.
- [ ] Explain missing protocol support, missing operation support, denied authorization, and uncertain outcomes with actionable errors, without offering automatic unsafe retry.
- [ ] Document permission scope, reconnect behavior, process-exit limitations, and lack of filesystem sandboxing; test affected UI and lifecycle behavior.

## Phase 7 — End-to-end contract verification and review

Objective: prove the public client-host interface and approved failure behavior, not only isolated helpers. Files: focused server/client integration fixtures, affected package tests, and feature documentation. Tests: a real gRPC host/client pair with separate workspace roots and deterministic fault injection; no model-provider dependency is needed for routing/recovery fixtures.

- [ ] Build a host/client integration fixture with conflicting project files and separate temporary Git repositories; assert file, search, write, command, Git, checkpoint, and automatic-context operations use only the bound workspace.
- [ ] Exercise main-agent, worker, nested delegation, and each supported external invocation route; verify rejected unsupported routes never reach host workspace helpers.
- [ ] Interrupt the network during a side-effecting command after a deterministic execution barrier; verify the live client finishes, reconnect returns its original outcome, and a marker records exactly one invocation.
- [ ] Cover disconnect before acceptance, lost acceptance, lost result acknowledgement, duplicate request/result delivery, stale ownership, wrong-client results, concurrent observers, authorization denial/expiry, cancellation races, and process loss.
- [ ] Verify old/new protocol compatibility and host-mode regression behavior; run affected server/client integration tests and focused race tests, not just shared-package unit tests.
- [ ] Review the Phase 1 inventory row by row: every workspace path has a routed implementation or explicit rejection test, and host-only services remain host-local.
- [ ] Obtain independent security/correctness review of ownership, authorization, hidden host bypasses, cancellation, and replay behavior; resolve findings and rerun relevant tests.
- [ ] Record actual test commands/results, remaining limitations, and final spec acceptance mapping in this plan; do not mark acceptance complete on child assertions alone.
- [ ] Checkpoint final changes, then delegate local Git landing and verify branch/SHA and worktree cleanup under repository policy. Do not push; retain the worktree with a stated reason if landing is blocked or not authorized.
