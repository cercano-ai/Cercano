# Client-side workspace execution

Status: spec and execution plan approved by the user on 2026-11-18; autonomous implementation authorized.

Approved security addition (2026-11-18): use mutual TLS over the existing gRPC interface, not a new listener or communications system. Pairing uses a short-lived single-use code and trusted host certificate fingerprint verification. The client generates its private key locally; the host issues a per-client certificate and retains an authorization/revocation record, not the client private key. Protect host keys and use OS credential storage for client keys where supported. Pairing authenticates an installation; explicit local consent authorizes workspace execution, and existing operation approvals remain separate. Direct execution is not filesystem sandboxing. Revocation rejects future access and closes active execution connections but cannot undo side effects or guarantee stopping disconnected commands. Reconnect retains installation identity while process incarnation remains distinct; no automatic replay, host fallback, or invented completion after crashes. Detailed implementation and security tests remain pending.

## Problem and motivation

Cercano currently orchestrates conversations and executes workspace operations in the agent host. A client connected to a remote host therefore operates on the host's files and runs commands there, not on the client's machine. Users need the host to continue providing orchestration and model access while working against a workspace on a connected client.

Redirecting model-requested tools alone is insufficient. Host code also reads project context when preparing prompts and delegated work. Those reads must use the same selected workspace as explicit file and command tools. Otherwise the agent can receive instructions from one machine while modifying another.

## Goals

A conversation has an explicit, visible execution location and workspace: either the host or a particular connected client. Workspace file reads, writes, searches, commands, Git operations, and automatic project-context reads all respect that selection. Delegated work, including nested delegation, inherits it.

The host remains responsible for orchestration, model calls, the tool loop, conversation state, and permission coordination. The client executes authorized workspace operations and reports progress and results. It does not become a second independent orchestrator.

The CLI is the first client implementation. The protocol must describe client execution without depending on terminal UI concepts, so other clients can implement the same contract later.

Existing host execution remains compatible during this separation. Client execution never silently falls back to the host, including when a client lacks a required operation, disconnects, or returns an error.

## Non-goals

This effort does not provision containers, manage their lifecycle, synchronize files between machines, or implement workspace handoff. It does not add a persistent background client service or promise that commands survive client-process exit. It does not migrate existing host conversations to client execution automatically.

Location and isolation remain separate concepts. Host/client selection must not be represented as container/direct selection. The broader approved direction is container-by-default host execution with specifically authorized direct execution, and first-class client containers alongside explicitly authorized client-direct execution. Container enforcement and lifecycle are not implemented by this effort; preserving current host behavior must not be described as already delivering that future isolation policy.

## User-visible behavior

Before client workspace execution is enabled, the client explicitly authorizes it and selects its local workspace. Connecting to or observing a conversation alone does not grant authority to execute commands. The selected location and workspace are visible in the conversation UI, and tool activity identifies where it executes.

A conversation stays bound to its selected machine and workspace. Opening it from a different client does not move execution or grant that client execution ownership. Explicit handoff is deferred. Reconnecting the same authorized executor is recovery, not handoff; identity must not be inferred merely from a matching path string or machine display name.

If the selected client is unavailable, workspace-dependent work waits or reports that execution is unavailable. The host does not use a same-named host directory as a substitute. Host-only orchestration may continue where it does not require missing workspace information.

## Execution and workspace contract

The existing gRPC interface is extended for client execution; no separate server, port, or communications system is required. The contract includes executor identity, conversation/workspace binding, operation identity, arguments, progress, result delivery, and cancellation. A dedicated execution stream over the existing connection was proposed, but its exact wire shape is an implementation detail rather than a separately approved requirement.

The routing boundary covers all supported workspace entry points, not just the main model tool adapter. This includes delegated workers and external tool interfaces. An entry point that cannot carry enough authenticated ownership and workspace context must reject client execution explicitly rather than execute on the host by default.

Automatic project instructions, workspace-scoped skills, project discovery, and relevant repository state must be obtained from the selected workspace. The host still assembles the prompt. Host-owned configuration, account credentials, conversation storage, model access, and orchestration remain host-local; they must not be indiscriminately redirected or copied to the client.

Workspace operation implementations should be reusable between host and client, not copied into the TUI. Current server-internal packages cannot simply be imported by the CLI under Go's internal-package rules. The plan must identify the shared package boundary and its dependencies without exposing host services or secrets as part of the client executor.

Relative paths resolve against the selected workspace at the selected location. A host path string is not evidence of access to a corresponding client directory. Existing tool path semantics must remain explicit; this effort must not claim that selecting a workspace provides filesystem sandboxing.

## Permissions and security

The host continues coordinating existing per-operation approval. Client execution additionally requires explicit client-side authorization; host approval alone does not authorize an arbitrary connected device to run commands. Conversation observers cannot claim execution requests or submit another executor's results.

Execution requests and results must be bound to the authorized executor, conversation, workspace, and original call. Reject stale ownership, unexpected results, duplicate dispatch, and cross-conversation or cross-client substitution. Credentials needed for host orchestration remain on the host. Commands use the selected executor's operating-system identity and environment; the UI and documentation must make that consequence clear.

Cancellation and authorization expiry must be distinguished from network disconnection. Only already-authorized work may continue while disconnected; a disconnected client cannot obtain fresh approval by assuming consent. Cancellation requested while disconnected must not be reported as successful before the execution outcome is known.

## Disconnects, results, and process exit

A temporary network disconnect does not cancel work already running on a client whose process remains alive. The client retains execution records and results for reconciliation on reconnect. Pending results must not be silently discarded; resource limits and acknowledgement handling must be specified in the implementation plan.

The host marks outstanding work as disconnected or awaiting reconciliation. It does not reissue a command, infer failure from silence, or switch execution location. Reconnection recovers the original execution's progress/result using stable call identity and duplicate suppression. This is not a promise of exactly-once operating-system side effects across crashes.

Closing or crashing the client is different from losing its network connection. No continuation guarantee is made after process exit. If the host cannot establish an execution's outcome, it records that outcome as unknown/interrupted and preserves any result already received. It must not declare successful cancellation or automatically retry potentially side-effecting work. Recovery can require explicit user intervention.

## Decisions

### Host orchestration with client workspace execution

Approved: keep orchestration on the host and add client-side workspace execution first. Container support is a separate effort. Moving the entire agent into the client would violate this scope rather than offer an equivalent implementation.

### Shared access for explicit and automatic workspace operations

Both model-requested tools and host-initiated project reads must use the selected workspace. Routing only explicit tools cannot satisfy the agreed execution-location contract; it would mix project information from different machines. The host remains responsible for prompt construction.

### Continue running through temporary disconnection

Approved: already-running client commands continue while the client process remains alive, with results recovered after reconnect.

| Axis | Continue through network disconnect — chosen | Cancel on network disconnect |
|---|---|---|
| User outcome | Brief network outages do not terminate useful work | Useful builds or tests can be interrupted |
| Implementation | Requires retained records, acknowledgement, and reconciliation | Requires cancellation handling but still cannot undo side effects |
| Verification | Reconnect, duplicate delivery, stale results, and lost acknowledgements | Cancellation races and uncertain completion |
| Risk | Results must remain bounded and recoverable within the live process | Cancellation may be incomplete and outcomes still uncertain |
| Reason | Matches the approved behavior | Rejected by the user's selection |

### Pin execution; defer handoff

Approved: keep a conversation on its selected machine/workspace. Explicit handoff can be built later.

| Axis | Pin execution — chosen | Add explicit handoff now |
|---|---|---|
| User outcome | Opening another client does not move work | User can deliberately relocate a conversation |
| Implementation | Stable binding and reconnect ownership | Also requires workspace mapping and transfer rules |
| Verification | Multiple observers, reconnect identity, and no fallback | Also in-flight work, conflicting ownership, and transfer recovery |
| Risk | Original machine must be available for further workspace work | Handoff can mix files or duplicate work unless carefully reconciled |
| Reason | Approved initial scope | Deferred, not ruled out permanently |

## Acceptance criteria

A host and client using distinct temporary directories can demonstrate that explicit file/command/Git operations and automatic project-context reads use only the selected workspace. A deliberately conflicting host-side file must not contaminate client-side context.

Main-agent and delegated-agent execution, including nested delegation, preserve the selected workspace and executor. Host-only orchestration remains on the host. Supported external entry points either honor the binding or explicitly reject unsupported client routing.

Opening the same conversation on a second client neither moves execution nor executes requests twice. Unauthorized or stale executor results are rejected. Existing approval requirements and client authorization are exercised with both allow and deny cases.

A controlled network interruption during a side-effecting command permits completion in the still-running client; reconnect delivers the original outcome without repeating the side effect. Duplicate delivery and lost acknowledgements do not cause re-execution. Cancellation during disconnection remains pending or uncertain until reconciled.

Client-process loss produces an honest unknown/interrupted outcome where necessary, never silent host fallback or automatic replay. Host-mode regression tests continue to pass. Protocol compatibility behavior is explicit when one side does not support client execution.

## Audit evidence and remaining tracing

Read-only audits identified the shared capability Call/Execute boundary in source/server/internal/capabilities/capability.go and the agent adapter in capabilities/agentadapter/adapter.go. They identified separate InvokeCapability and legacy InvokeTool paths, MCP forwarding, and direct project-context loading in internal/agent/agent.go, internal/dispatch/engine.go, and internal/mcp/server.go. Existing conversation observation is distinct from task execution ownership in source/proto/agent.proto.

These findings are not a complete execution-path inventory. The precise worker builtin path, nested delegation propagation, project discovery, workspace-scoped skill reads, and every direct Git/filesystem bypass still require source-level tracing before their implementation steps are finalized. Audit agents returned partial findings and some path/tool errors; completion of a task is not evidence that those gaps were closed. No tests were run and no implementation was changed during this audit.

The execution plan must start with a bounded routing inventory, identify concrete shared-code dependencies and tests, and explicitly block completion on unaccounted workspace access. If that tracing exposes a genuine new architecture choice rather than an implementation detail, it must return for human review rather than silently change this contract.
