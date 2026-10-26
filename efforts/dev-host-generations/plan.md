# Development host generations execution plan

The user approved spec.md on 2026-10-25 based on document review, not implementation or test evidence. The planning approval committed the specification and plan at 361e38879209. Execution is authorized; checkbox status and recorded checks, not approval, establish progress. No native acceptance result is claimed.

Execution clarification (2026-10-26): the user authorized the bounded approach: implement and verify normal A/B coexistence, explicit release/handover, reconnect, and idle-host retirement first, then conservative absence-based crash recovery. No further general process-identity research or additional infrastructure. Ambiguous recovery cases may remain unsupported with a clear diagnostic. In the supported same-machine, same-user development context, recovery requires acquiring the conversation lock, direct kill(positive_pid, 0) returning ESRCH, conditional matching of durable owner/epoch or reservation, and accounting for potentially surviving work without replaying external effects. Success, EPERM, other errors, and unresolved work refuse recovery. A held lock blocks crash recovery; the source holding its own lock during normal handover is expected. This explicit user clarification supersedes earlier requirements to qualify a general process-birth identity mechanism; normal workflow must not wait on that research. Recovery-specific implementation and qualification in the phases below follow normal-workflow verification. The bounded credential prerequisite may proceed while concrete Phase 1 inventories are completed; dependent ownership integration still requires those inventories.

Implement only the approved same-machine, deliberately compatible development A/B journey. Keep conversation-scoped operating-system locks and durable epochs, quiescent transfer, explicit per-session release and explicit retirement. Do not add live-task migration, a general session service, arbitrary-build compatibility, updater integration, automatic generation deletion, or host-count-based configuration freezing.

No live installation changes, current-agent/runtime restarts, merges, pushes, or release publication. After execution approval, use a dedicated linked implementation worktree and isolated fixtures/processes. Do not inspect real user credentials, configuration, or databases. Native acceptance is macOS only; Linux/Windows remain unqualified. No paid providers are needed. Checkpoint solved increments without pushing; do not land the branch under this authorization.

Each increment must record its commit, exact checks, result and limitations, fixture/build/host identities where relevant, and artifact locations. Planned evidence is not collected evidence. Run targeted package tests plus integration tests for changed interfaces, not indiscriminate full end-to-end runs. Use deterministic barriers and fault injection for races rather than timing-only sleeps. Native evidence listed below is required before declaring that increment verified; if it is unavailable, record the increment as unverified and block dependent native acceptance.

Foundational stops: if centralized write fencing cannot cover actual persistence paths, native instance-death evidence cannot safely distinguish identity reuse, the shared backend cannot remain protected, credential isolation is incomplete, or selected builds cannot satisfy exact persisted-state compatibility, stop for human review. Do not substitute broad reconciliation, PID-only recovery, temporary HOME isolation, timeouts, a new coordination service, or relaxed ownership. Resolve any genuinely new structural choice with the decision workflow before coding it.

## Phase 1 — Revalidate the baseline and close integration inventories

Objective: ground execution in current source before changing behavior. Files to inspect: AGENTS.md, source/server/internal/conversation, internal/taskrun, internal/broker, internal/server, internal/modelhost, internal/worker, internal/secrets, pkg/agentclient, pkg/config, cmd/cercano, source/proto/agent.proto, source/clients/cli, and the development launcher. Historical main and updater references in the specification are leads, not current evidence. Tests to design: admission/write coverage, compatibility-before-open probes, and native identity evidence probes. Native evidence: document the supported macOS process-instance evidence and lock semantics using only dedicated fixtures after isolation is verified; this phase alone does not qualify them.

- [x] Delegate baseline and worktree inspection, then create a new linked implementation worktree after approval; record branch, SHA and paths without disturbing existing worktrees or changes
- [x] Revalidate the credential-isolation source commits and enumerate the minimal production and test dependencies to extract without merging the updater branch
- [-] Inventory all conversation-owned writes and dependent effects, including turns, task acceptance/outcomes, notification claims/acknowledgements, autonomous state, deletion, recovery, and indirect persistence callbacks
- [ ] Map every work and attachment admission path to the existing broker/server/task manager and define the centralized enforcement seam and lock ordering
- [ ] Inventory actual shared-configuration writers and disruptive modelhost operations, including startup migrations, download recovery, tool, RPC and setup paths
- [ ] Document the native instance identity/death evidence contract, process reuse and uncertainty cases, and required crash probes; stop for review if the existing primitives cannot support it safely
- [ ] Establish a requirement-to-test inventory and record foundational blockers before dependent implementation

## Phase 2 — Extract and prove credential-disabled isolation

Objective: establish a safe prerequisite for all native application tests. Files: bounded extracts from updater-branch internal/secrets, agentclient isolation/reuse handling, worker inheritance, associated startup integration and tests; no updater machinery. Tests: absent/0/1/malformed settings, sentinel rejection before backend opening, environment inheritance and unsafe reuse/reconnect refusal. Native evidence: actual dedicated macOS agent/CLI/worker startup and reconnect, with credential backend entry probes proving zero native Keychain opening/access and no provider calls. A temporary HOME is supplementary filesystem isolation only.

- [~] Extract only the credential-disable facility and dependencies verified in Phase 1, documenting source provenance and excluded updater changes
- [ ] Preserve fail-closed malformed-setting handling and reject credential access before any backend is opened
- [ ] Carry the isolation requirement through initial connection, reconnect and worker launch; reject hosts that cannot establish the required disabled status
- [ ] Add hermetic tests that never read real credentials, including instrumented backend-entry rejection and unsafe host-reuse cases
- [ ] Run targeted tests, then isolated macOS startup/reconnect/worker probes with backend access instrumentation; record exact commands, disabled-setting propagation and zero-access evidence
- [ ] Checkpoint the bounded extraction only after its verification; block native application testing if isolation is not established

## Phase 3 — Centralize store compatibility and ownership write enforcement

Objective: make the store the authoritative stale-write boundary before enabling coexistence. Files: internal/conversation/store.go, tasks.go, other inventoried persistence files and their callers; narrow ownership/lock helpers adjacent to the existing store; fixture preparation and store tests. Tests: exact-match preflight, transaction predicates, stale owner/epoch rejection and lock/crash behavior. Native evidence: dedicated macOS subprocesses contending for stable conversation locks and fixture SQLite writes, including process death and identity-reuse/uncertainty cases; no real store access.

- [x] Implement explicit fixture-store preparation with schema and coexistence representation metadata outside participating-host startup; never adopt or upgrade an existing store automatically
- [ ] Add read-only metadata preflight before migration, initialization writes, persistent database setting changes or reconciliation; reject missing or mismatched contracts without mutation
- [ ] Add stable conversation lock identities and distinct build, host-instance and acquisition-epoch identities; never unlink an in-use lock inode or overload broker generation
- [ ] Implement durable owned, cleanly released and destination-reserved states and conditional transitions in the existing ownership record
- [~] Centralize owner/epoch and contract validation atomically with every inventoried owner-controlled mutation; prohibit transactions spanning model/tool execution
- [ ] Make stale/rejected mutation results explicit to callers so rejected completion cannot trigger wakes or notification effects
- [ ] Add transaction, fault-injection and subprocess tests for stale writes, clean versus abandoned ownership, process identity reuse and uncertain durability
- [ ] Prove preflight refusal leaves fixture state unchanged, including logical contents and persistent metadata; round-trip blocks, task outcomes and notification state between the selected contracts
- [ ] Record native lock/store evidence and checkpoint; do not enable coexistence until the write inventory has no unexplained bypasses

## Phase 4 — Integrate admission, task lifecycle and owner-scoped recovery

Objective: enforce ownership over execution as well as persistence, without migrating or cancelling work to manufacture readiness. Files: internal/broker/broker.go, internal/server/server.go, task_completion.go, internal/taskrun/manager.go and store callers identified in Phase 1. Tests: turn/task/completion admission races, callback draining, scoped reconciliation and stale-effect suppression. Native evidence: two dedicated fixture hosts sharing a store; starting B leaves A's running task and sibling conversation unchanged, and stale completion produces neither writes nor notifications.

- [~] Route foreground turns, automatic continuations, independent task acceptance and completion through centralized conversation admission
- [ ] Account for accepted/running tasks, acceptance in flight, completion callbacks, persistence and unresolved write failures in quiescence; retain ownership on terminal disconnect
- [ ] Track epoch-bound terminal attachments and atomically check attachment eligibility while closing conversation work and attachment admission
- [ ] Replace broad startup task reconciliation with recovery scoped to an eligible recorded owner and epoch, requiring the approved native death evidence where ownership was abandoned
- [ ] Suppress wakes and notification side effects on stale or unsuccessful mutations, including conditional updates returning no affected row
- [ ] Reload durable pending outcomes after acquisition instead of relying on source-host wake queues
- [ ] Exercise deterministic races and failure probes, including disconnected independent tasks and uncertain recovery; record native two-host isolation evidence
- [ ] Checkpoint verified enforcement without claiming transfer or reconnect acceptance yet

## Phase 5 — Publish immutable generations and protect the shared backend

Objective: launch compatible dedicated hosts without replacing executables or disrupting sibling state. Files: source/server/scripts/cercano-launcher.sh, pkg/agentclient launch support, internal/worker/spawn.go and reap.go, cmd/cercano startup/modelhost paths, internal/modelhost, pkg/config and inventoried mutation entry points. Tests: publication failure, executable pinning, resource isolation, configuration rejection and runtime mutation rejection. Native evidence: immutable fixture A/B generations, pinned worker executable paths, unchanged configuration and shared backend process/instance continuity while both hosts run.

- [ ] Publish complete generation directories atomically before selection; remove stale-agent timestamp killing from coexistence launch without changing the live installation
- [ ] Pin initial agent and worker executable resolution to the selected generation; reject unrelated PATH fallback and partial generations
- [ ] Scope launch locks, discovery, logs, worker resources and orphan cleanup to relevant host/generation identities; prove cleanup cannot target siblings
- [ ] Enforce exact contract compatibility before participation and prohibit automatic modelhost replacement and in-process fallback in coexistence mode
- [ ] Enforce the mode-wide shared-configuration freeze at centralized persistence and exposed mutation paths; preserve session-local selections and history/task persistence
- [ ] Guard disruptive shared-runtime operations separately at the shared operation seam, including restart, shutdown, global stop/unload and model/download mutation; permit release of only the caller's participation
- [ ] Test every inventory entry and automatic startup path for explicit refusal without partial mutation; retain existing non-coexistence behavior in targeted regression tests
- [ ] Record native A/B startup and backend continuity evidence, then checkpoint; do not delete old generation directories

## Phase 6 — Implement destination participation, handover, release and retirement

Objective: implement the approved lifecycle on the existing control surface and ownership record, including the pre-commit destination fence. Files: source/proto/agent.proto and generated bindings, server control handlers, broker/admission helpers, conversation ownership transitions and agentclient control calls. Tests: protocol integration, deterministic handover/retirement races and persistence/crash windows. Native evidence: dedicated A/B/C processes with controllable barriers, demonstrating reserved acquisition, non-destructive refusal and backend survival.

- [ ] Add host identity and scoped lifecycle control operations through the existing client/server interface; bind requests to exact host instances, epochs and handover identifiers
- [ ] Require B to accept a specific incoming handover under the same atomic lifecycle fence used by retirement before A may commit a reservation
  - [ ] Bind accepted participation to the conversation, source host/epoch, unique handover identifier and B's exact instance, after compatibility validation
  - [ ] Count accepted participation as relevant destination work from acceptance through durable resolution; retirement cannot pass readiness while it exists
  - [ ] Make a winning retirement fence reject destination acceptance, so A cannot commit on the strength of mere reachability, compatibility or a stale acknowledgement
  - [ ] Do not drop accepted participation on timeout, disconnect or uncertain source commit; resolve it against the matching durable record and source operation outcome before releasing the hold
  - [ ] Test retirement winning before acceptance, acceptance winning before source commit, delayed acknowledgement, source failure and uncertain commit; surface any unresolved lifecycle incompatibility rather than add a coordination service
- [ ] Implement A's atomic attachment eligibility/admission closure and quiescence checks while retaining ownership; conditionally commit B-reserved relinquishment before unlocking
- [ ] Implement B's conditional reserved claim with a fresh epoch, state reload and idempotent result reporting; C must leave the reservation unchanged even if C obtains the lock first
- [ ] Implement explicit stranded-reservation recovery only with the conversation lock, matching durable record and sufficient destination-instance death evidence; clearing does not acquire ownership
- [ ] Implement per-session release and management listing/release without adding a terminal attachment, bulk release or cancellation
- [ ] Handle failed/uncertain persistence by rereading durable state; report pending versus success accurately and restore admission only while ownership/lifecycle state permits it
- [ ] Implement explicit retirement with acquisitions, attachments and destination participation fenced before final readiness; refuse reversibly without cancelling work and release only this host's backend participation on success
- [ ] Run A/B/C races and fail B before claim, during claim and after commit before acknowledgement; test abandoned-source acceptance holds, stale retries and recovery evidence uncertainty
- [ ] Record native lifecycle evidence, including last release leaving host and generation directory intact, then checkpoint

## Phase 7 — Enforce pre-launch reconnect safety and expose terminal controls

Objective: let each terminal explicitly move its conversation while ordinary reconnect remains exact-host and never resurrects a released/retired session. Files: pkg/agentclient/client.go, reconnect.go and launch arbitration, agent protocol integration, CLI connection/session state, slash registry, header and mouse/details paths. Tests: client state/launch-spy tests, protocol integration and terminal behavior. Native evidence: disconnected old terminals retrying after release, transfer and retirement with zero spawn attempts, plus TUI/host identity presentation before and after movement.

- [ ] Separate explicit initial launch/resume intent from ordinary reconnect in the client API and persisted/reconstructed connection intent where used
- [~] Make ordinary session reconnect use only the pinned exact-host connection path, never auto-launch; enforce this before executable lookup or process spawn, including shared launch helpers
  - [ ] Preserve host/epoch binding through disconnect and classify released/stale/retired results explicitly
  - [ ] When the host is unreachable and session status cannot be established, fail closed without spawn, migration or acquisition; do not infer permission from transport failure
  - [ ] Require a separate explicit launch/resume action for a new host/session acquisition; ordinary retry cannot convert itself into that action
  - [ ] Prove zero launch invocations with a launch spy and native process/event evidence when the old host is absent, not just server-side rejection after connecting
- [ ] Expose explicit selected-host transfer, release, retained-session listing, resume and retirement controls with clear busy, pending, uncertain and stale outcomes
- [ ] Keep other terminals and sibling conversations on their selected hosts; refuse movement with other live attachments and preserve usability after refusal
- [ ] Display the running TUI build separately from connected host build/instance through /version and clickable details; do not re-execute the TUI on movement
- [ ] Run native disconnected-terminal reconnect attempts after transfer/release/retirement, including a client that missed invalidation and launch-lock races; verify no respawn and no unrelated-generation fallback
- [ ] Record automated and manual terminal evidence separately, then checkpoint

## Phase 8 — Qualify the bounded macOS A/B journey and hand off

Objective: demonstrate the complete approved contract on a deliberately compatible pair, without extending platform claims or touching live services. Files: isolated native acceptance harness and tests, docs/agent/self-dev.md and appropriate CLI/development documentation; evidence records associated with this effort. Tests: integrated acceptance matrix from the specification and prior increments. Native evidence: actual dedicated macOS builds, fixture store/configuration, verified disabled credentials, shared backend identity, terminal behavior and injected crash/race outcomes.

- [ ] Recheck isolation and record exact A/B build identities, coexistence/schema declarations, host instances, fixture paths and dedicated backend identity before starting acceptance
- [ ] Demonstrate B startup while A's sibling conversation and independent task continue; transfer only a quiescent conversation and verify fresh epochs and durable history/task round-trip
- [ ] Run the attachment, acceptance/retirement, B/C claim, stale-write, busy-refusal, failed-persistence and crash-recovery matrix with deterministic barriers; collect final durable state and side-effect evidence
- [ ] Verify missing/mismatched metadata and equal-schema incompatible representation refusal precede all store mutation/reconciliation
- [ ] Verify every configuration/runtime inventory guard, pre-launch stale reconnect prevention and immutable-generation retention end to end
- [ ] Confirm explicit retirement leaves B and the shared backend running without restart or sibling resource unload; clean up only dedicated test processes and fixtures
- [ ] Document commands, outcomes, failure artifacts and limits; distinguish unit tests, subprocess integration, manual TUI checks and native qualification, and state Linux/Windows remain unqualified
- [ ] Update user/developer documentation for explicit mode, temporary configuration freeze, safe controls, compatibility limits and non-automatic generation cleanup
- [ ] Obtain an adversarial contract review, address findings with focused tests, and checkpoint the verified work; report branch/SHA/worktree and remaining blockers without merging, pushing or publishing
