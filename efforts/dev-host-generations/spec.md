# Development host generations

Status: specification approved by the user. The execution plan remains subject to its separate approval gate; implementation is not approved. Approval is based on document review, not implementation or test verification.

## Problem and motivation

Rebuilding the development agent currently conflicts with keeping existing conversations and independent tasks running. Launch and reconnect discovery can select a different executable, the development launcher kills older agents, and broker ownership is process-local. Starting a second host against the same conversation database also exposes unscoped unfinished-task reconciliation. Removing the launcher kill alone is therefore insufficient.

The first slice enables a same-machine A/B journey: build and start B without disrupting A, explicitly move one quiescent conversation to B, and leave other conversations and independent tasks on A. Each terminal client can move independently. A can retire after its remaining sessions are explicitly released or transferred and its work has drained.

## Goals

Publish complete immutable development builds. Preserve the selected agent executable and host identity through launch, worker spawning, discovery, and ordinary reconnect. Do not silently fall back to another build.

Allow protocol-capable A and B to share authoritative conversation storage and the existing model runtime safely. Make ownership conversation-scoped, protecting foreground turns, independent task acceptance and completion, automatic continuation, recovery, and owner-controlled persistence.

Provide explicit quiescent transfer and non-destructive per-session release. Provide a minimal way for another client to list retained sessions on a selected host and explicitly release a disconnected session. Listing and release must identify the selected host and conversation unambiguously. Disconnection is not release.

Expose the connected agent's identity separately from the terminal client's build and the model runtime. Preserve the requested version-details access through `/version` and the clickable version label. Capability profiles and cloud-routing profiles must not be presented as a stable local user identity.

## Terminology and terminal attachments

A **conversation** is the durable history and associated task records, and is the unit of ownership. A **session** is a host's retained ownership of that conversation under an ownership epoch; it survives terminal disconnection. A **terminal attachment** is a client connection to that session, bound to its host instance and epoch. These are terms for the existing ownership and client/server mechanisms, not a new session service.

Terminals referencing different conversations can move independently. Multiple terminals referencing the same conversation share one ownership unit: one terminal cannot move just its own copy of that conversation to another host. For the first slice, transfer or release refuses while any other live terminal attachment references that conversation. The requesting conversation attachment may remain until successful relinquishment, when it becomes invalid. A management client can list and release a disconnected session without becoming a conversation attachment; in that case any live conversation attachment prevents release.

Check attachment eligibility and close conversation admission, including new attachments, atomically. If attachment eligibility fails, leave the existing session usable. If later readiness checks fail, restore normal admission where safe without cancelling work. Disconnection removes an attachment but does not release the session. A disconnected terminal reconnecting after transfer or release receives an explicit stale-epoch or released-session result; it cannot silently migrate, reacquire, or respawn a retired host. Moving to B requires explicit attachment to the new session after acquisition, not ordinary reconnect of the old attachment.

Moving a conversation to B does not replace or re-execute its terminal user interface (TUI). `/version` and clickable version details must separately identify the running TUI build and the connected agent host's build and instance. The TUI build remains unchanged across a host move.

## Non-goals and boundaries

No live-task migration, whole-host drain as a substitute for per-conversation transfer, general session registry, cross-machine ownership, bulk release, inactivity-based release, or automatic migration on disconnect. Do not integrate the updater or reopen its completed controlled-update milestone. Do not merge the updater branch merely to obtain credential isolation.

No modification of the live installation, restart of current agents or runtime, push, merge, or release publication is authorized by this effort's planning approval. Verification must use isolated fixtures and dedicated processes after execution approval.

## Compatibility and storage contract

Acceptance uses a deliberately compatible pair of development builds A and B, not arbitrary historical or development builds. Both must enforce the same ownership protocol and support the same persisted conversation/task representation contract, including turn blocks, task outcomes and notification state, as well as the same database schema. Equal schema versions alone do not establish compatibility. There is no protection guarantee against an older binary that bypasses these checks.

Use a narrow, explicit coexistence contract identifier declared by each build, covering ownership semantics and persisted conversation/task representations, alongside the expected schema version. The isolated shared store records that contract. Participating hosts validate exact agreement with the store and selected peer before acquisition or transfer; unknown or mismatched identifiers fail closed. This is an exact-match development contract, not version negotiation, a compatibility matrix, a conversion layer, or a general compatibility framework. Changing incompatible persistence semantics requires changing the identifier; the deliberately selected pair must also pass shared-state round-trip and task tests rather than relying on labels alone.

This slice assumes a same-machine authoritative SQLite database on a filesystem with supported locking. Before invoking any store-opening path that migrates, writes initialization state, changes persistent database settings, or reconciles tasks, perform a read-only compatibility preflight against the existing store metadata and build declaration. A missing marker or mismatched schema/contract refuses participation without store mutations or reconciliation. The compatible fixture store is prepared explicitly before either host participates; automatic adoption or upgrade of existing stores during coexistence startup is out of scope. Revalidate the contract inside ownership-controlled write transactions so the preflight is not the sole write fence. Startup must never silently launch an incompatible fallback or mutate shared storage speculatively.

Build identity, host-instance identity, and ownership epoch are distinct. A build may produce multiple host instances; each ownership acquisition must use a fresh epoch. Never overload the existing process-local broker generation as a cross-process ownership token.

## Ownership and admission invariants

Use conversation-scoped operating-system locks plus durable owner/epoch metadata, integrated with the existing conversation store and admission paths. Keep lock identities stable; do not unlink an inode while another participant might still use it.

Every owner-controlled persistence mutation must validate the current owner and epoch atomically with the mutation, in the same SQL transaction or conditional statement. A preceding token read is insufficient. No database transaction may remain open during model or tool execution. Rejected stale writes must also suppress dependent completion wakes and notification effects.

Admission covers user turns, automatic completion turns, task acceptance, and other paths capable of starting conversation work. A transfer or release closes admission for that conversation before checking readiness. Observing an idle broker is not a reservation.

Quiescence requires no active turn or continuation, accepted/running task, in-flight task acceptance, completion callback, or unfinished persistence work for that conversation. Unresolved persistence failures block readiness. Tokens do not waive this requirement and cannot fence external tool effects. Never cancel work to manufacture readiness.

## Transfer contract

A atomically checks attachment eligibility and closes admission for the selected conversation, then establishes quiescence while retaining its operating-system lock and ownership. B must be a compatible, explicitly selected host instance, not merely a build or endpoint that another instance could reuse.

Before unlocking, A conditionally records durable clean relinquishment as a **pending handover reserved to B**, bound to the conversation, A's expected owner/epoch, B's exact host instance, and a unique handover identifier. This is a state of the existing durable ownership record, not a coordination service. It is not the freely resumable state produced by ordinary release. A's old epoch becomes ineligible for work, writes, and attachment at this commit; old callbacks cannot reopen admission.

After obtaining the conversation lock, every claimant must inspect the durable ownership state before making any ownership mutation. Only B can conditionally consume the matching pending handover and acquire a fresh epoch. C must refuse even if it obtains the operating-system lock first, release that lock without changing the reservation, and admit no work. Lock availability never overrides a destination reservation. B admits work only after its claim is durable. Retrying a completed handover reports its recorded result without allocating another epoch or duplicating work.

B reloads durable conversation state and checks durable pending task outcomes rather than depending on A's process-local wake queue. Completed outcomes may transfer; live execution may not. Sibling conversations and their tasks remain on A. Transfer success is reported only after B's acquisition is confirmed; A's relinquishment alone is reported as pending, not success.

Failures before committed relinquishment restore A's ordinary admission only when ownership and lifecycle state still permit it. Uncertain commit results require rereading durable state before restoring admission or unlocking. After committed relinquishment, A must not automatically roll back, reacquire, or redirect the reservation. If B fails before claiming, the reservation persists without timeout-based reassignment; neither C nor a replacement process using B's build may claim it. The same B instance may explicitly retry while the reservation still matches.

For a stranded pending handover, an explicit recovery operation may clear that exact reservation to cleanly released state only while holding the conversation lock, conditionally matching the handover record, and establishing that the designated B instance has ended under the recovery evidence rules below. A mere failed connection or available conversation lock is insufficient. If evidence is uncertain, leave the reservation unchanged and report the blocker. Clearing the reservation does not acquire ownership: a separate explicit resume takes a fresh epoch. If B already committed acquisition, pending-handover recovery must fail its conditional check; ordinary owner-scoped recovery applies instead. No recovery path replays external effects or cancels work to obtain readiness.

## Retained-session release

Release is explicit, per-session, and non-destructive. It preserves conversation history, completed task evidence, and the ability to explicitly resume later. A terminal disconnect, terminal exit, timeout, or transport error retains the session and ownership.

Another client must be able to list retained sessions on a selected host and request release of a particular disconnected session. Reuse the existing client/server control surface rather than introducing a general registry. Release runs through the same conversation-scoped admission and quiescence rules as transfer.

A busy refusal restores ordinary admission where safe. A failed persistence operation must not report success or silently relinquish ownership. Record clean relinquishment durably before releasing the operating-system lock. If durability is uncertain, fail closed and surface the condition rather than guessing.

A stale reconnect after release must not reacquire ownership, migrate the conversation, or respawn a retired host. It must report that explicit resume is required. Explicit resume constitutes a new acquisition with a fresh epoch. Ordinary reconnect remains bound to the selected host; it does not serve as implicit resume.

## Recovery and crash evidence

An available operating-system lock proves exclusion, not death of the previous host. Distinguish durable clean relinquishment from abandoned ownership. Neither transport disconnect nor PID-only checks are adequate evidence of abandonment.

Recovery must establish evidence appropriate to the recorded owner instance before modifying its unfinished work. Lock acquisition alone cannot classify ownership as abandoned. If clean relinquishment or abandonment cannot be established safely, refuse recovery and leave unfinished work unchanged. The implementation and tests must make the evidence contract explicit, including process-identity reuse and crash windows around durable relinquishment and unlock.

Never run broad startup reconciliation against another live host's tasks. Recovery and interruption are scoped to the proven eligible owner and epoch. Existing recovery is interruption, not execution replay; this effort must not introduce replay of external effects.

## Host retirement

Retirement is explicitly requested, never automatically triggered by the last release. Releasing the last session merely makes the host eligible; it does not request retirement or imply bulk session release.

A retirement request fences both new ownership acquisitions and new terminal attachments before its final readiness check. Retire only after no owned or retained sessions remain and all relevant host work, completion callbacks, and persistence activity have drained without unresolved failures. Pending destination claims and transfer callbacks count as relevant work; a host must not retire across an unhandled reservation addressed to it. Admission for accepting a handover destination must participate in the retirement fence. A bounded cancellation shutdown is not a readiness check.

If not ready, refuse without cancelling work and restore normal ownership and attachment admission where safe. Retirement failure must be visible and leave admission in a known state; it must not report completed retirement while work remains.

Successful retirement releases only that host's participation in the shared runtime. It must never stop or restart the shared backend or unload resources held by sibling hosts. Stale reconnects must not respawn the retired host.

## Immutable builds, discovery, and shared runtime

Publish a complete immutable generation before making it selectable. Scope agent launch arbitration, discovery, logs, worker resources, and cleanup to the relevant identity. Preserve sibling executable resolution within the immutable build and prohibit fallback to unrelated PATH binaries in coexistence mode. Cleanup must not target sibling generations by process name or binary timestamp. Automatic deletion or garbage collection of old immutable generation directories is deferred; release and retirement do not delete those directories.

Use the existing shared model-host discovery and per-client participation facilities. Coexistence mode must prohibit automatic backend replacement and in-process fallback on connection or compatibility failure. Guard disruptive global runtime operations separately from configuration writes, including restart, shutdown, global unload/stop, and shared model/download mutations. Releasing one's own participation remains permitted.

## Temporary shared-configuration freeze

Shared configuration is read-only for the entire explicit coexistence mode. Do not introduce host-count-based freeze/unfreeze coordination. Reject affected operations clearly, including automatic startup migration/save paths, setup, credential/profile writers, and indirect tool or RPC mutation paths.

Preserve per-session selections that do not modify shared state. Conversation history, task persistence, notification state, and research/task evidence remain functional; the configuration freeze must not disable them. Document this as a temporary development limitation, not final multi-profile behavior.

The source audit must enumerate actual writers and shared-runtime mutations before claiming coverage. The existing partial audit identified startup credential migration and download recovery as well as interactive paths; it is not an exhaustive inventory.

## Credential-isolation prerequisite

Bring in the credential-disabled facility through a bounded integration and verification task sourced from the updater worktree, not an implicit merge of that branch. Prior read-only reconnaissance located it in `/Users/bryancostanich/git_repos/cercano-ai/Cercano-cross-platform-updates`, branch `feat/cross-platform-updates`, inspected HEAD `9e581d3176faa9242345e3867c2416eb07e7fe0e`. That branch attributes credential disabling to `1be362efd52f` and unsafe agent-reuse refusal to `7b8079bae1c2`. These references require verification against the execution baseline.

The prerequisite must reject backend credential access before opening native Keychain, handle malformed isolation settings safely, propagate isolation to workers, and refuse unsafe reuse/reconnect. Verification must cover the actual native startup and reconnect paths without reading real credentials. Temporary HOME is not Keychain isolation. Do not run live-provider acceptance until this prerequisite is verified; no paid-provider access is necessary for this slice.

## Approved decisions

### Ownership: locks plus durable epochs

Chosen by the user: conversation-scoped operating-system locks and durable ownership tokens, attached to existing store and admission paths. Locks provide exclusion; atomic epoch checks reject stale persistence. Neither establishes permission for non-quiescent transfer.

| Axis | Lifetime conversation lock only | Lock plus durable ownership epoch |
|---|---|---|
| Complexity | Lock lifecycle and admission integration | Same plus metadata and conditional writes |
| Risk | Missed lifecycle drain permits stale effects | Missed epoch checks leave individual writes unprotected |
| Outcome | Same-machine exclusion | Exclusion and stale-write rejection |
| Side effects | Greater dependence on callback lifecycle discipline | More explicit stale-owner failures and write predicates |
| Verification | Claim, release, crash, and admission races | Same plus stale mutation and notification rejection |

The strongest case for lock-only ownership is reduced persistence changes in a strictly quiescent local workflow. The chosen additional fencing protects the independent-task and recovery paths, without claiming to fence external effects.

### Release: explicit per-session only

Chosen by the user: per-session release, including listing and releasing disconnected retained sessions from another client. No bulk release in the first slice.

| Axis | Per-session release only | Per-session plus bulk release |
|---|---|---|
| Complexity | One scoped operation and listing/control surface | Same plus enumeration and partial-success handling |
| Risk | Explicit target; busy sessions refuse | Same checks with greater accidental scope |
| Outcome | Individual non-destructive release | Faster release of many sessions |
| Side effects | Repeated actions for many sessions | Concurrent arrival and batch semantics |
| Verification | Admission, persistence, and stale-reconnect races | Same plus batch concurrency and partial failures |

Bulk release is convenient but unnecessary for the first A/B journey. Explicit intent is preferable to inferring release from disconnect or inactivity.

### Shared state: fixed coexistence-mode freeze

Chosen by the user: shared configuration remains read-only throughout the explicit mode, with independent guards for disruptive runtime operations. Dynamic host-count coordination is excluded. History/task persistence and session-local choices remain enabled.

### Attachments: refuse while other live attachments remain

Chosen by the user: conversation-scoped ownership, retained epoch-scoped sessions, and client terminal attachments. Refuse transfer/release when another live attachment remains; atomically check eligibility and close admission. Disconnection removes only the attachment. Management release does not create a conversation attachment. Refusal leaves the session usable; stale reconnect still requires explicit handling.

| Axis | Refuse with other live attachments | Permit with explicit invalidation |
|---|---|---|
| Complexity | Attachment accounting and atomic eligibility gate | Same plus invalidation delivery and client state transitions |
| Risk | Stale accounting may block a valid request | Missed invalidation may mislead an attached client |
| Outcome | No displacement of other connected terminals | A requester can displace other connected terminals |
| Side effects | Other terminals must detach first | Other terminals must explicitly reattach afterward |
| Verification | Attach/disconnect races and usable refusal | Same races plus multi-client invalidation |

Invalidation would improve convenience, but refusal keeps this first slice's user intent explicit. Neither permits stale clients to migrate or reacquire automatically.

### Retirement: explicit request only

Chosen by the user: last release establishes eligibility, not automatic retirement. An explicit request fences acquisitions and attachments, checks retained sessions and relevant work, and refuses reversibly without cancellation if not ready. Successful retirement releases only the host's backend participation. No bulk release is implied.

| Axis | Explicit retirement request | Automatic retirement after last release |
|---|---|---|
| Complexity | Request path and host-wide readiness fence | Last-release trigger and same readiness fence |
| Risk | User may leave an eligible idle host running | User may lose an idle host they intended to reuse |
| Outcome | Shutdown requires explicit intent | Unneeded hosts retire without another action |
| Side effects | Separate action after session release | Session release also initiates host lifecycle changes |
| Verification | Request races, refusal restoration, backend survival | Last-release races, refusal restoration, backend survival |

Automatic retirement would reduce manual cleanup, but explicit retirement separates conversation release from host shutdown and is the approved behavior.

## Acceptance criteria

Initial native acceptance targets macOS, using isolated fixtures and dedicated A/B processes with verified credential disabling, including native Keychain exclusion. Linux and Windows remain unqualified for native coexistence acceptance in this milestone; portable unit-test coverage alone must not be described as platform qualification. No network-filesystem or cross-machine coordination is claimed.

Demonstrate with a deliberately compatible, isolated pair of protocol-capable builds A and B that B can start without terminating A, replacing the backend, falling back in-process, changing shared configuration, or interrupting A's tasks. Verify generation-pinned launch, worker spawning, and reconnect.

Move one genuinely quiescent conversation to B while another conversation and independent task remain on A. Demonstrate fresh epochs on every acquisition and atomic rejection of stale writes, task completion, and notification handling. Race foreground admission, task acceptance, completion, simultaneous claims, transfer, and release.

Demonstrate busy release refusal with safe admission restoration, persistence-failure refusal without unlock/success, non-destructive release from another client, explicit later resume, and stale reconnect refusal without reacquisition, migration, or respawn.

Attach two terminals to one conversation and verify transfer/release refuses without making that session unusable. Race new attachments against eligibility checking and admission closure. Disconnect the other terminal, retry successfully, then reconnect that old terminal and verify explicit stale-epoch/released-session refusal without migration, reacquisition, or host respawn. Verify management release does not create an attachment, and moving a different conversation leaves sibling attachments untouched.

Race B and C after A commits destination-bound relinquishment, deliberately letting C obtain the lock first: only B can consume the reservation. Fail B before lock acquisition, after locking but before durable claim, and after claim but before acknowledgement. Verify persistent reservation, idempotent result handling, no false success, and owner-scoped recovery after a committed claim. Test explicit clearing of a stranded reservation with sufficient destination-death evidence and refusal with uncertain evidence; ordinary C resume must never bypass a pending reservation.

Verify last-session release leaves the host running and its immutable generation directory intact. Race an explicit retirement request against new acquisition, new attachment, and destination-handover admission. Confirm both admission classes are fenced before the check; retained sessions and relevant pending work block retirement; refusal cancels no work and restores admission where safe. Successful retirement releases only A's runtime participation while B and the backend continue without restart. No session is bulk-released.

Test clean relinquishment, abandoned-owner evidence, uncertain ownership, process-identity reuse, and crashes around durable state changes. Startup must leave a live sibling's tasks untouched. Uncertain evidence must fail closed.

Test missing and mismatched coexistence markers, incompatible ownership protocols, incompatible persisted representations despite equal schema versions, and schema mismatches. Prove refusals precede store-opening mutations and reconciliation and leave shared state unchanged. For the intentionally compatible pair, round-trip conversation blocks, task outcomes, and notification state across A/B. Do not describe this as arbitrary-build compatibility.

Test configuration/runtime mutation rejection through all inventoried entry points, immutable publication failure, and credential-disabled native startup and worker inheritance. Through `/version` and clickable details, verify that the unchanged running TUI build is distinguished from the newly connected B host build and instance after transfer. Tests must distinguish automated source/fixture coverage from any manual terminal or native-platform verification not performed.

## Source grounding and remaining verification

Prior read-only inspection identified the main integration points: `source/server/internal/conversation/store.go` and `tasks.go`; `internal/taskrun/manager.go`; `internal/broker/broker.go`; `internal/server/server.go` and `task_completion.go`; `pkg/agentclient/client.go` and reconnect support; `internal/worker/spawn.go` and `reap.go`; `internal/modelhost`; `cmd/cercano/main.go`; and `source/server/scripts/cercano-launcher.sh`. Client identity/details integration touches the agent protocol and CLI header/slash/mouse paths.

The inspected baseline was main `6c38577e0344bc05b241ab2b9b6a4324581bbd41`; this is historical evidence, not a fresh status check. Exact mutation coverage, platform lock behavior, crash-evidence implementation, and schema compatibility enforcement must be verified before implementation claims. A foundational incompatibility must return to human review rather than silently weaken this contract.

## Approval and review

The user has approved this specification and requested the execution plan. The completed plan must pass the planning approval gate before implementation. The user's planning clarifications require B to accept destination participation under its retirement fence before A commits a reservation, and stale reconnect handling to prevent automatic host respawn before attempting launch, not merely reject a subsequent connection. These clarify the existing safety contract. No review worktree has been created; these documents are in the current repository and can be reviewed by absolute path. Worktree creation and execution remain separate from document approval.
