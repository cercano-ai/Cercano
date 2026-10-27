# Full production Cercano with safe generation coexistence

Specification approved 2026-10-27; this execution plan awaits separate approval. Completion means ordinary Cercano retains its existing functionality and data while gaining safe generation coexistence. A successful new-host launch, a reduced command set or a synthetic lifecycle demonstration alone is not completion.

Execution approval covers implementation, isolated regression and acceptance, and migration rehearsals on disposable working copies of the already-authorized backups. Preserve the retained backup originals. It does not authorize live migration, installed-launcher replacement, native credential access, real-provider spending, stopping existing processes, push or release publication. Test-owned children may be started and cleaned up within isolated fixtures; never target existing user processes. Live cutover tasks remain blocked on specific later authorization even if this entire plan is approved.

Use the approved specification and bounded audit as the baseline; do not repeat a broad discovery phase. Resolve routine factoring, locking, testing and coordination details during implementation. Escalate only concrete scope/security/destructive boundaries or foundational violations of approved requirements. Never weaken ownership, credential isolation or acceptance to get a pass. Commit solved units with explicit scope and record source SHA, test commands, outcomes, skips, artifacts and limitations. Keep confidential copies and credentials out of git and reports.

Phases 2–6 may proceed in bounded parallel slices after the shared composition and authority contracts in Phase 1 are established. Phase 7 depends on their integration. Phase 8 follows a stable migration format and may run alongside isolated acceptance; Phase 9 requires both acceptance and rehearsal results for the exact candidate. Phase 10 cannot begin on plan approval alone.

## Phase 1 — Establish one production composition and a safe test boundary

Objective: make generation lifecycle a policy of the existing full application, not a second feature graph. Files: cmd/cercano/main.go, development_host_production.go, development_provider*.go, internal/server/prepared*.go, worker policy/factory seams and effort evidence. Tests: paired composition tests, configured-authority resolution, fail-before-open and fake-backend trap tests. Existing integration findings are task inputs, not another audit request.

- [ ] Create a dedicated execution worktree from the approved planning commit and preserve unrelated working-tree changes
- [ ] Convert the specification's functionality list into an acceptance matrix with ordinary baseline behavior, affected implementation, automated test and explicit qualification boundary
- [ ] Factor ordinary startup dependency composition so ordinary and generation-aware hosts use the same production services and provider/tool graph
  - [ ] Inject store/lifecycle, generation-pinned worker and service authority policies without invoking legacy opening, global reconciliation or unpinned runner replacement
  - [ ] Preserve authoritative path resolution and supported overrides without creating another copied config/history authority
  - [ ] Require compatibility preflight before any schema, persistent-setting, credential-migration or task-reconciliation side effect
- [ ] Provide invocation-local test seams for credential stores, issuer/provider transports, runtime managers, MCP and enterprise fixtures without altering the production graph
- [ ] Add native-opener traps and child-environment checks before enabling any credential-dependent test scope; verify enabled fake positive controls without calling a native backend
- [ ] Establish private short runtime roots, bounded subprocess lifetimes and owned-child-only cleanup for all acceptance harnesses
- [ ] Demonstrate equivalent service registration and explicit failure handling for both composition modes; retain existing startup behavior until subsequent gates pass

## Phase 2 — Complete ownership-aware foreground, descendant and background execution

Objective: preserve ordinary execution features under exact conversation ownership and continuous work accounting. Files: internal/conversation, broker, server, taskrun, worker, hostsvc/tools, hostsvc/persistence, recap/compaction/retention producers and agent collaborators. Tests: deterministic admission barriers, stale callbacks, persistence faults, nested dispatch and two-host producer races.

- [ ] Integrate exact host/epoch authority into every ordinary request and administrative mutation, including history actions and permission callbacks
- [ ] Complete nested subagent acquisition, parent/child persistence and event/completion authority without single-level dispatch restrictions
- [ ] Carry work tickets and authority through worker calls, task acceptance, context replacement, queued outcomes, callback execution and notification acknowledgement
- [ ] Restore recap, compaction, metering, rollover and related background producers with counted timers/retries, cancellation/drain and ownership-bearing contexts
- [ ] Coordinate global maintenance and retention without broad startup reconciliation or mutation of another live owner's active records
- [ ] Ensure write failures, rejected transitions and panics suppress dependent success publication and retain unresolved readiness blockers
- [ ] Verify transfer and retirement account for queued work, descendant work, timers and persistence; no transaction spans inference, tool or network execution
- [ ] Test A's independent work and sibling conversations through B startup, unrelated transfer and terminal disconnect, including stale owner rejection and repeated concurrent SQLite writes
- [ ] Preserve conservative crash behavior without replaying external effects or claiming unsupported automatic recovery

## Phase 3 — Coordinate configuration, permissions and credential changes

Objective: retain writable ordinary settings and credential behavior at existing authorities with both serialization and cross-host visibility. Files: pkg/config, internal/hostsvc/config, agent/permissions.go, server/config_watcher.go, hostsvc/credentials, secrets, anthropicauth, chatgptauth, cloudfactory and provider rebuild paths. Tests: two-host mutations, stale-save conflicts, fake rotating issuers, delayed callbacks and invalidation acknowledgements.

- [ ] Introduce narrowly scoped cross-process mutation coordination using existing lock patterns and fresh reads; publish config and permissions atomically without stale whole-file overwrites
- [ ] Reuse ordinary watchers and apply/rebuild paths with revision/conflict handling, loop prevention and explicit durable-save versus live-application results
- [ ] Preserve supported environment resolution, hot reload and explicit restart requirements without surprise generation replacement
- [ ] Restore ordinary session and shared settings controls without treating credential disabling as a history/config freeze signal
- [ ] Serialize credential refresh/login/logout by existing credential identity and reread after admission; prevent late refresh from resurrecting logout
- [ ] Propagate provider/token invalidation across enforcing hosts and workers without secret-bearing notifications or independent writable worker credential openers
- [ ] Handle rotating-token external ambiguity conservatively, with bounded reconciliation or reauthentication rather than blind retry
- [ ] Test simultaneous conflicting changes, external file edits, failed publication, stale providers, refresh races and logout/relogin using memory stores and fake endpoints only
- [ ] Verify existing credential service/profile identities and config formats remain unchanged; do not run automatic legacy credential relocation during overlapping startup

## Phase 4 — Restore shared integrations and runtime administration

Objective: preserve ordinary MCP, enterprise and runtime/model functionality while respecting shared ownership. Files: internal/mcp_host, enterprise, modelhost, localruntime and runtime providers, production startup wiring and existing management RPCs. Tests: protocol-faithful disposable integrations, policy revocation, shared holds, download/cancel races and startup incompatibility.

- [ ] Install the ordinary MCP manager in canonical production composition and coordinate definition mutations with fresh-read conflict handling and manager reconciliation
- [ ] Track MCP connection/process ownership so restart, configuration changes and retirement do not close sibling resources; retain exclusivity for integrations that cannot safely duplicate
- [ ] Integrate enterprise RPCs, interceptors and background admission through existing authority and connection locks without creating a competing credential owner
- [ ] Deliver policy revision/revocation to each enforcing host and refuse new work when required freshness cannot be established
- [ ] Route ordinary runtime/model operations through the existing shared model host, preserving caller holds and preventing overlapping fallback managers
- [ ] Replace blanket protected-mode administration denial with coordinated operation admission, busy handling and explicit impact semantics consistent with ordinary behavior
- [ ] Coordinate download/cancel/retry and model/cache publication at configured asset paths; prevent independent writers and retain successful artifacts
- [ ] Test incompatible runtime startup without replacement, A retirement while B holds a loaded synthetic instance, retry after failed Stop, simultaneous administration and reconnect
- [ ] Test external MCP discovery/update/failure/teardown and enterprise connect/refresh/revoke/logout with fake services and no native credentials
- [ ] Record runtime variants and exclusive integration behavior covered by tests; incomplete required functionality blocks activation rather than becoming a hidden reduced mode

## Phase 5 — Restore accounting, catalog, diagnostics and project outputs

Objective: retain production maintenance and observability without stale writes or data loss. Files: internal/telemetry, catalog, watchdog, taskmodel, research, diagnostic writers and startup/shutdown composition. Tests: concurrent opens/writes, schema faults, collector drain and file conflict cases.

- [ ] Install ordinary telemetry/accounting and verify compatible schema opening, strict migration error propagation, revision-based deduplication and retirement drainage
- [ ] Test simultaneous host accounting, retry, consumer refresh and any retention interaction without losing acknowledged usage
- [ ] Restore catalog refresh, watchdog and provider rebuilding with counted lifecycle work and safe cancellation
- [ ] Preserve authoritative model/catalog paths and distinguish reconstructible caches from retained history, telemetry and downloaded assets
- [ ] Add narrow conflict checks for concurrently targeted plan/research outputs while retaining caller-selected paths and atomic publication
- [ ] Keep diagnostics attributable to host/build and persistence outcome without secrets; test errors remain visible instead of being logged and reported as success

## Phase 6 — Restore the ordinary terminal and generation lifecycle

Objective: ordinary startup, history and controls remain functional while exact-host reconnect and explicit transfer stay safe. Files: source/clients/cli/main.go, internal/ui, internal/slash, pkg/agentclient, server lifecycle RPCs, development_launch.go and launcher/publisher scripts. Tests: fake RPC reducers plus actual compiled terminal journeys.

- [ ] Restore ordinary terminal initialization for tools, permissions, runtime state and events rather than the reduced pinned-session command allowlist
- [ ] Make help/completion and execution availability consistent; surface all refused/failed operations visibly
- [ ] Browse authoritative history and explicitly acquire unowned conversations; route retained conversations to their exact live owner without silent stealing or respawn
- [ ] Preserve ordinary new conversation, search, task navigation, settings, model/profile, permission and integration controls under ownership-aware RPCs
- [ ] Fence queued messages and asynchronous results by client, conversation and monotonic lifetime generation across transfer/reconnect
- [ ] Keep terminal build identity distinct from host identity and preserve the running terminal executable across handover
- [ ] Verify source-first quiescence, extra-attachment refusal, zero destination participation on definite busy refusal and conservative exact-receipt retries for uncertain outcomes
- [ ] Preserve immutable paired publication, unchanged-source reuse, serialized host startup, strict reconnect validation and no launch attempts for stale/released sessions
- [ ] Ensure help/version/invalid flags and management-only commands do not acquire conversations or launch hosts unnecessarily
- [ ] Update operational documentation without replacing the installed launcher or executing it against real user state

## Phase 7 — Regression and isolated production-build acceptance

Objective: qualify full ordinary functionality and future A/B updates before requesting live cutover or launcher replacement. Files: integration tests, PTY harnesses, fixture services, build scripts and evidence documents. Tests use the real production composition and compiled host/worker/CLI binaries in a private environment, not a fixture-only host entrypoint.

- [ ] Run focused unit, integration, race and fault-injection checks for every changed interface and preserve failing probes before fixes
- [ ] Run full server and CLI suites and builds on an exact committed candidate; document each skip, limitation and platform qualification
- [ ] Build normal production binaries with required native features and verify manifests/signatures without accessing native credentials
- [ ] Exercise those production binaries with explicit isolated configuration, loopback protocol fixtures and credential-disabled/no-native-access boundaries wherever supported
- [ ] Use narrowly injected memory credentials and protocol-faithful fake services for credential/enterprise flows that cannot run with admission disabled; keep production composition unchanged and distinguish these artifacts from unmodified builds
- [ ] Run ordinary terminal feature-parity acceptance for existing-history resume, new work, settings/permissions, model routing, integration discovery, nested dispatch, autonomy, background work and accounting
- [ ] Run the complete future-generation A/B journey before any live activation request
  - [ ] Start A and accept independent work, then physically disconnect and reconnect the terminal to the same host without publishing or acquiring again
  - [ ] Publish distinct B from changed source while A remains active; verify unchanged-source reuse and concurrent wrapper invocations
  - [ ] Transfer unrelated Y while X executes on A; prove busy X transfer leaves destination participation exactly zero and extra attachments prevent unsafe transfer
  - [ ] Complete X with durable notification acknowledgement and no duplicate in the defined test; release/transfer remaining sessions
  - [ ] Retire A explicitly and verify B can perform fresh inference/tool work while its shared runtime resource remains held
  - [ ] Reject stale-A reconnect before any launch attempt and preserve terminal executable identity
  - [ ] Introduce a real compiler syntax error and prove build failure leaves existing state, hosts and selections usable
- [ ] Exercise lost responses, persistence failures, cancelled admission, incompatible schema, path/symlink errors and bounded Unix-socket paths
- [ ] Race-instrument child binaries where supported; explicitly identify any uninstrumented subprocess and distinguish loaded synthetic resources from real model qualification
- [ ] Obtain independent scoped review of authority, shared mutation visibility, test isolation and parity; fix reproduced findings and rerun affected gates
- [ ] Publish a candidate-specific acceptance matrix with no required feature silently deferred; native credentials/real-provider claims remain unmade until separately authorized

## Phase 8 — Implement and rehearse authoritative-store adoption and reconciliation

Objective: preserve data while enabling compatible ownership at the existing authoritative database, proven only on copies first. Files: internal/conversation migration/validation code, explicit adoption command, copy-only rehearsal harnesses and evidence. Tests: supported schema fixtures, real authorized-backup working copies, interrupted operations, conflicts, restore and rerun.

- [ ] Implement explicit exclusive adoption preflight that enumerates required writer exclusion and rejects ordinary auto-start adoption, unsupported schemas or uncertain exclusion
- [ ] Define only necessary compatibility/ownership metadata and schema version transitions; ensure old unfenced writers cannot reopen an adopted store through supported startup paths
- [ ] Preserve all existing IDs, ordering, relationships, history, task outcomes/notifications, autonomy, routes, compaction, dispatch and usage; do not replay active external work
- [ ] Implement development-record reconciliation with identical-record no-op, supported unique-history import and explicit same-ID/content conflict refusal
- [ ] Exclude development ownership, handover and active execution authority from import; do not overwrite authoritative config with the copied development snapshot
- [ ] Add resumable/idempotent migration phase tracking and conservative interruption handling, bounded disk/time requirements and pre-mutation failure checks
- [ ] Rehearse on new disposable copies of retained validated backups, leaving backup originals untouched and never invoking application startup on live paths
- [ ] Verify pre/post inventory, row fingerprints, ordering, relationships, integrity and foreign keys without displaying message or credential contents
- [ ] Test synthetic same-ID conflicts, corrupt/unsupported inputs, insufficient space, interrupted writes, repeated execution and unknown commit outcomes
- [ ] Rehearse restore to a separate disposable destination and test that no post-adoption writes are silently discarded by automatic rollback
- [ ] Run full production-build feature and A/B acceptance against the adopted/reconciled disposable store; prove legacy history and the unique development conversation remain accessible
- [ ] Record exact artifacts, resource use, migration duration, validation results and rollback boundaries; separate backup timestamps do not substitute for a final live snapshot

## Phase 9 — Exact-candidate readiness and explicit cutover proposal

Objective: provide evidence before requesting any live operation. Files: effort acceptance matrix, migration manifest, cutover/rollback runbook and operator documentation. No installed path or live store is changed in this phase.

- [ ] Reconcile specification coverage against passing Phase 7 and Phase 8 evidence; unresolved required parity or preservation failures block readiness
- [ ] Rerun full server/CLI regressions, builds, focused races, isolated compiled A/B acceptance and adoption rehearsal for the exact proposed candidate after integration fixes
- [ ] Prepare a nonexecuted launcher candidate targeting the canonical production application and existing authorities; verify syntax, paths, no legacy kill sweep and no implicit migration
- [ ] Prepare a cutover runbook listing exact writer-identification/exclusion method, scoped downtime, fresh backup limits, preservation validation, rollback boundary and abort conditions
- [ ] Identify any required native credential, real-provider or real loaded-model qualification separately; request specific authorization only after isolated acceptance and rehearsals pass
- [ ] Present evidence, remaining qualifications and exact proposed live operations for explicit authorization; do not treat plan approval or prior launcher authorizations as permission for this cutover

## Phase 10 — Separately authorized live cutover and product verification

Objective: deliver the ordinary application with preserved data and full functionality. Every task here is conditional on fresh explicit authorization for its live effects. Denial or missing authorization means a clearly reported blocked cutover, not completion of the product goal. Files/targets: only the explicitly approved authoritative paths, launcher and process identities from the runbook.

- [ ] Obtain and record authorization for each applicable live effect: migration, installed-launcher replacement, native credential access and stopping identified existing processes
- [ ] Revalidate candidate and writer identities; establish approved exclusion, take and validate fresh bounded backups and abort without migration if prerequisites fail
- [ ] Perform the rehearsed adoption and development-record reconciliation at authoritative paths with pre/post preservation checks
- [ ] Replace the installed launcher only after isolated future A/B qualification and migration rehearsal have passed, preserving the approved rollback artifact
- [ ] Run authorized ordinary-product checks for history/data, settings, permissions, integrations, routing, tasks and background functionality; limit native credential/provider activity to its explicit approval
- [ ] Confirm generation coexistence and explicit session movement without disrupting unrelated sessions, using the already-qualified lifecycle and approved runtime checks
- [ ] Report failed checks and apply only the approved rollback procedure; never silently restore an old snapshot over new writes or relaunch incompatible old binaries
- [ ] Record final product acceptance, remaining platform/native qualifications and retained sensitive artifacts; no cleanup beyond separately authorized exact targets
- [ ] Commit final evidence and documentation, report local branch/commit and worktree disposition, and do not push or publish a release without authorization
