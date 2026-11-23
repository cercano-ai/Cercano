# Shared desktop broker and computer-use integration

Implement the approved spec using feat/client-workspace-execution as the integration foundation. macOS and Windows are required; Linux support and the full Avalonia application are excluded. All tasks below are pending. Prior Cua trials and historical execution tests are evidence for feasibility, not completion of these gates.

Planning documents currently reside in the main checkout. Before code changes, verify the actual feature base and carry the approved documents into an implementation worktree based on that branch. Do not replace the feature base with main, rewrite its work, remove its untracked evidence, or land either effort without explicit authorization. Failed Git inspection is an unresolved tooling/environment issue, not proof of repository damage.

Substantial implementation uses a linked worktree. Checkpoint solved units without pushing. Match tests to changed interfaces; record exact commands, environment, results, and limitations. Escalate genuine new architecture/security decisions before coding them. Approval of this plan does not authorize OS permission changes, installation into the user's live environment, or uncontrolled desktop interaction.

## Phase 1 — Verify foundation and close implementation boundaries

Objective: establish a trustworthy base and precise dependency map before implementation. Files: this effort; efforts/client-side-tool-execution/spec.md and plan.md on the feature branch; source/server/go.mod; source/clients/cli/go.mod; source/proto/generate.sh; source/server/Makefile; release/homebrew, release/chocolatey, release/linux and their referenced release workflows. Tests: bounded existing server/CLI baselines after verifying module-specific commands; no live desktop actions.

- [ ] Verify feature-branch HEAD, worktree status, applicable instructions, and existing execution progress through delegated Git inspection
  - [ ] Resolve the inspection failure without destructive repository repair
  - [ ] Preserve existing changes and .windows-review-evidence artifacts
  - [ ] Establish a linked implementation worktree based on the verified feature head and carry the approved planning documents into it without unrelated changes
- [ ] Map existing execution-location tasks to broker prerequisites and identify ownership of overlapping work
- [ ] Inspect the actual shared-package dependency manifests and install/update/uninstall machinery; reuse the existing model rather than infer conventions from directory names
- [ ] Establish independent component/module placement, Go/C# generation locations, and the minimal .NET consumer build layout
- [ ] Specify local authenticated transport, desktop-session discovery, credential storage, and endpoint protection for macOS and Windows
- [ ] Specify bounded operation retention, acknowledgement, broker crash recovery, client-loss behavior, control-lease expiry, and idle shutdown
- [ ] Present material unresolved choices for approval and record the resulting file/test map before implementing those boundaries
- [ ] Record targeted baseline commands and results; distinguish pre-existing failures from new work

## Phase 2 — Establish execution-location prerequisites

Objective: complete or verify the necessary portions of the existing approved execution plan, not build a competing router. Files: existing execution effort; source/proto/agent.proto; source/server/internal/conversation; source/server/internal/hostsvc; source/server/internal/worker; source/server/pkg/agentclient; source/clients/cli and the shared execution packages identified in Phase 1. Tests: existing execution-plan security, binding, routing, disconnect and regression cases in targeted packages, plus cross-interface integration tests.

- [ ] Complete the existing routing/dependency inventory gate before assuming client execution is usable
- [ ] Complete or verify approved executor authentication, pairing/revocation, local consent, and independent security review
- [ ] Complete or verify explicit bindings, capability registration, executor/process identity, and stale-ownership rejection
- [ ] Complete or verify execution identity, duplicate suppression, retained outcomes, acknowledgements, and cancellation/disconnect reconciliation
- [ ] Verify delegated and nested work inherits execution location and unsupported entry points reject explicitly
- [ ] Verify distinct host/client environments, denied consent, unavailable executors, and no host fallback
- [ ] Document which existing prerequisite tasks were completed here versus independently completed on the foundation branch

## Phase 3 — Independent contract and development compatibility

Objective: establish a client-neutral broker API without manual protocol version churn. Files: independent broker module and proto locations selected in Phase 1; source/proto/generate.sh or its shared generation entry point; source/server/Makefile where applicable; Go/C# bindings and contract tests. Tests: reproducible generation, Go and C# compilation, schema/serialization cases, handshake mismatch and capability negotiation.

- [ ] Define typed readiness, permissions, desktop/session identity, observations, captures, actions, and outcome messages
- [ ] Define authenticated attachment, conversation attribution, leases, cancellation requests, retained status and reconciliation APIs
- [ ] Separate request acceptance, input delivery, verified effect, refusal, and unknown/interrupted outcomes
- [ ] Generate Go and C# bindings reproducibly and compile both consumers
- [ ] Implement unstable development mode, deterministic contract fingerprint, build identity, and capability handshake
- [ ] Reject incompatible attachments without terminating or replacing another client's active broker
- [ ] Document semantic compatibility limitations of fingerprints and the stable-version-1 transition required before independent client releases

## Phase 4 — Broker lifecycle, authorization, and coordination

Objective: implement one independently owned broker per user/interactive desktop session. Files: new component core, local transports, session discovery, authorization, lifecycle and operation-record packages selected in Phase 1. Tests: unit and process-level integration tests using a fake backend, with no real desktop input.

- [ ] Implement private endpoint creation, authenticated attachments, session identity, discovery and single-launch coordination
- [ ] Implement attachment lifecycle independent of the first launching client and bounded idle shutdown
- [ ] Enforce executor/conversation/session ownership and separate desktop grants from workspace and operation approvals
- [ ] Implement bounded control leases covering action sequences and explicit contention responses
- [ ] Isolate observations, screenshots and results across clients/conversations
- [ ] Implement operation records, acknowledgement/reconciliation, bounded retention and duplicate suppression
- [ ] Implement honest cancellation, lease expiry, detach/disconnect, broker restart and unknown-outcome handling
- [ ] Test simultaneous startup, first-launcher exit, multiple clients, denied/stale attachments, cross-conversation substitution, lost acknowledgements and restart uncertainty
- [ ] Conduct adversarial security review before enabling a real backend

## Phase 5 — Private Cua backend and platform feasibility

Objective: implement replaceable backend semantics and prove signed/platform feasibility early, before completing UI integration. Files: broker backend interface, Cua adapter, supervisor and platform modules; package/signing manifests identified in Phase 1. Tests: adapter fixtures and fake subprocess tests first; bounded disposable desktop acceptance on authorized macOS/Windows environments afterward.

- [ ] Define backend interfaces and normalized error/outcome mappings without exposing Cua-specific client APIs
- [ ] Select and verify a pinned Cua artifact for each required platform, licenses/notices, signing provenance and optional-component exclusions
- [ ] Implement private helper supervision, session lifecycle, endpoint isolation, version checks and failure reporting
- [ ] Disable and verify independent telemetry/update behavior
- [ ] Implement reviewed observation, accessibility, pixel, keyboard and explicit foreground operations
- [ ] Preserve capture identity, coordinate frame, dimensions, scale and delivery-route evidence; reject stale/invalid targeting
- [ ] Implement platform readiness and permission checks; explicitly reject unsupported Linux environments
- [ ] Prove macOS signed launch-path permission attribution and Windows user/session/privilege boundaries in approved test environments
- [ ] Verify helper crash and refused/unverifiable outcomes without claiming a stopped or successful action
- [ ] Stop and revisit the approved premise if shipped packaging or required platform behavior is not viable

## Phase 6 — Execution routing and CLI/.NET integration

Objective: make both client technologies consume the shared broker through the existing execution-location model. Files: source/proto/agent.proto where extension is needed; execution adapters and shared packages from Phase 2; source/clients/cli; generated C# consumer and minimal .NET harness from Phase 1; existing MCP/image and vision adapters where required. Tests: fake-broker client integration, two-client process tests, selected-executor screenshot round trip and permission UI tests.

- [ ] Register broker capabilities with the selected executor; preserve authenticated bindings through host orchestration and delegated work
- [ ] Implement CLI dependency discovery, attachment, permission/readiness presentation, active-control indication and stop requests
- [ ] Implement the minimal C# consumer against the same generated contract without duplicating broker policy or Cua logic
- [ ] Verify CLI and .NET simultaneous use, lease contention, isolated approvals/results and first-client exit
- [ ] Carry screenshot bytes and capture metadata through executor transport with payload limits, sensitive-log protections and explicit vision destination
- [ ] Test remote-host/client targeting, unavailable executor, denied/revoked permissions, interruption and reconnect without duplicate side effects
- [ ] Verify that direct/external tool entry points either enforce the same ownership contract or reject unsupported use

## Phase 7 — Shared package delivery and upgrade safety

Objective: make the broker a real independently versioned dependency of both client distributions. Files: existing macOS/Windows package manifests and release workflows, independent broker release definitions, client dependency metadata, signing and onboarding documentation. Tests: clean install, repair, upgrade, mixed-client compatibility and uninstall on approved macOS/Windows test machines.

- [ ] Build the independent broker package with pinned helper and notices using existing dependency conventions
- [ ] Declare CLI and desktop-client integration dependency metadata without requiring the full Avalonia app
- [ ] Implement compatible discovery and explicit development-build override without disturbing active sessions
- [ ] Verify upgrades do not replace an active incompatible broker or silently replay work
- [ ] Verify removing either client retains a shared dependency still required by the other
- [ ] Verify permission attribution, onboarding, revocation and upgrade behavior under the exact shipped launch paths
- [ ] Document stable protocol release gate separately from component package versioning

## Phase 8 — Required-platform acceptance and handoff

Objective: establish end-to-end evidence for macOS and Windows, and identify any remaining release blockers honestly. Files: component/client integration tests, platform acceptance scripts and fixtures, this effort's verification notes, CLI/agent and installation documentation. Tests: targeted unit/integration suites plus authorized real-platform acceptance; no unrelated full suite unless shared-interface impact warrants it.

- [ ] Run reproducible contract generation and Go/C# build gates
- [ ] Run broker, execution-routing, adapter and client integration tests, including authentication and fault injection
- [ ] On both required platforms, verify accessibility actions, genuine pixel delivery, keyboard/dialog workflow and independent resulting-state/artifact readback on disposable surfaces
- [ ] Verify foreground-control indication, stop semantics, permission denial/revocation, target validation and documented human-focus limitations
- [ ] Verify screenshot/vision transport end to end and no fallback to the wrong machine
- [ ] Verify shared-package install/upgrade/uninstall and CLI/.NET coexistence using shipped artifacts
- [ ] Run independent security and implementation review; resolve findings or report blockers rather than treating unknowns as passes
- [ ] Document Linux follow-up and explicit unsupported behavior, API lifecycle, integration guidance, privacy boundaries and troubleshooting
- [ ] Record final branch/SHA, commands and platform evidence, unresolved limitations and retained artifacts
- [ ] Request completion review without pushing or landing the foundation/feature branches unless separately authorized
