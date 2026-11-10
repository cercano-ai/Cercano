# Recovery Cleanup

Execute the approved recovery-cleanup specification. Legacy client stop confirmations are rejected with upgrade guidance; supported old-host cooperation remains. Preserve the active wire envelope. No live-host, user-state, VM, deployment, push, or evidence-deletion actions are authorized. Recheck audit findings against the execution baseline rather than treating historical observations as current proof. Use an isolated feature worktree and scoped checkpoints; preserve unrelated changes and artifacts.

## Phase 1 — Establish baseline and trustworthy test runners

Objective: identify active consumers and ensure retained fixture commands cannot report success without executing intended tests. Files: recovery source and test references; efforts/old-host-compatible-recovery/fixture/run.sh, oldmain.go.txt, build-production.sh and related documentation. Tests: missing-selection failure and successful nonzero selection. Preserve historical evidence; distinguish archived scaffolding from supported entry points.

- [x] Read repository guidance and record the execution baseline and scoped working-tree preservation requirements
- [x] Map consumers of legacy confirmations, stop interfaces, recovery envelope, reaffirmResumeWithSync, and ReserveTakeover
- [x] Identify the current historical production acceptance entry point and record exact test selections
- [x] Reproduce the orphan runner's zero-test-success problem in disposable state before modifying it
- [x] Remove or clearly archive orphan runnable scaffolding without deleting historical evidence
- [x] Make retained acceptance runners reject zero intended tests and verify both rejection and valid execution
- [x] Record baseline evidence and checkpoint only this solved unit

## Phase 2 — Remove proven unused implementation

Objective: delete dead implementation without changing runtime behavior. Files: source/server/internal/server/resume.go; source/server/internal/conversation/ownership_takeover.go and tests; direct consumers discovered in Phase 1. Tests: active ownership lock, reservation, and resume safety tests. The unused reaffirmation helper is not permission to remove active journal durability barriers.

- [x] Confirm the reaffirmation helper and takeover primitive have no production consumers at the execution baseline
- [x] Inventory unique safety assertions in deletion-target tests and retain relevant assertions against active APIs
- [x] Remove unused helpers, primitive, and obsolete tests in narrowly scoped changes
- [x] Verify references, formatting, module compilation, and focused ownership/resume tests normally and with race detection
- [x] Checkpoint deletion-only changes with explicit behavior-preservation evidence

## Phase 3 — Consolidate recovery execution and consent

Objective: retain one destination-side contained recovery continuation and reject legacy client stop confirmations before side effects. Files: server/resume.go, resume_legacy.go, live_recovery.go, relevant lifecycle interfaces; CLI internal/ui/forced_resume.go and original_recovery.go; associated tests and documentation. Preserve the existing protobuf envelope and field numbers, and preserve cooperative old-host controls.

- [x] Add characterization tests for cooperative zero-stop resume, adapter-less cooperation, exact local reservation claim, and ready retry operation identity
- [x] Add a failing regression requiring legacy confirmations to return actionable upgrade guidance with no stop or ownership mutation
- [x] Replace legacy confirmation execution with explicit safe rejection and remove parallel UI/server continuations
- [x] Keep ordinary acquisition, cooperative transfer, and exact reservation claims ahead of contained fallback
- [x] Preserve fresh default-no consent and tests for decline, expiry, replay, requester changes, source/destination identity changes, and full ownership/reservation changes before stop
- [x] Verify modern confirmation response-loss retries retain exact current operation identity and do not repeat completed shutdown
- [x] Run focused server, agentclient, and CLI integration tests normally and with race detection; compile both modules
- [x] Review the compatibility boundary and checkpoint the consolidated path

## Phase 4 — Make fallback policy explicit

Objective: replace deferred error replacement with explicit recovery eligibility and outcomes without inventing a generic orchestration framework. Files: server/resume.go, live_recovery.go, lifecycle diagnostics and focused tests. Tests must distinguish original refusal from containment discovery and prove errors never supply stop authority.

- [ ] Document existing ordinary outcomes and proposed eligible fallback cases before restructuring control flow
- [ ] Add tests for authentication/identity failure, cancellation, uncertain persistence/publication, malformed journals, missing or unreachable sources, acceptance cleanup failure, source response loss, and receipt lookup errors
- [ ] Confirm any intended behavioral correction with an isolated failing probe before applying it
- [ ] Replace deferred fallback and special suppression flags with explicit outcome handling
- [ ] Preserve original diagnostic context without logging credentials or receipt contents
- [ ] Verify current ready ownership remains necessary and historical receipts cannot authorize readiness or recovery
- [ ] Run focused normal/race tests and review fallback branches for unintended new stop offers
- [ ] Checkpoint the explicit fallback policy and its evidence

## Phase 5 — Simplify the contained adapter and names

Objective: remove redundant adapter paths after their consumers are removed, retaining continuous launch exclusion and final native verification. Files: cmd/cercano/development_legacy.go, development_legacy_shutdown.go, development_recovery.go; internal/brewrestart attribution helpers; CLI/server internal recovery helpers and docs. Tests: absence and attribution failures, shutdown response loss, bounded drain, surviving generation processes, and exclusion lifetime.

- [ ] Recheck remaining Stop and WithStoppedSource consumers and remove only obsolete interfaces/paths
- [ ] Consolidate verification sequencing without dropping final native absence checks or releasing exclusion before mutation/reload finishes
- [ ] Preserve fail-closed handling for attribution errors, ambiguous/surviving processes, response loss, and cancellation
- [ ] Rename misleading internal helpers and document their mapping to the unchanged LifecycleOriginalRecovery envelope
- [ ] Keep obsolete wire modes rejected and record wire migration as a separate follow-up
- [ ] Run focused adapter/native attribution tests normally and with race detection and inspect cleanup failure behavior
- [ ] Checkpoint the adapter and naming cleanup

## Phase 6 — Final review, acceptance, and documentation

Objective: demonstrate the simplified implementation retains the approved recovery boundary. Files: relevant recovery documentation, historical acceptance runner, efforts/old-host-compatible-recovery status documentation, and this effort's verification record. Preserve historical findings as historical rather than rewriting them into current success claims. Tests: final historical owned/drain, reserved, blocked-timeout acceptance plus focused module tests and compilation.

- [ ] Correct stale current-status statements and identify one supported verification entry point with explicit fixture limitations
- [ ] Independently review final changes for authentication, consent, ownership, uncertainty, exclusion, and compatibility regressions
- [ ] Run historical owned/drain, reserved, and blocked-timeout acceptance against the final candidate with nonzero selections and no unexplained skips
- [ ] Assert full prior history preservation, exactly two added rows, exact final response, provider-call cardinality, and owned-process teardown
- [ ] Run focused normal/race ownership, recovery, process-control, agentclient, and CLI suites and both modules' interface compilation
- [ ] Inspect final references and diff checks; verify no unintended wire changes or unrelated artifact deletion
- [ ] Record exact commands, results, limitations, residual artifacts, and any blocked checks; do not equate compilation with platform runtime qualification
- [ ] Checkpoint final documentation and present completion evidence; do not push, deploy, or perform live recovery
