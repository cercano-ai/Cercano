# Full production Cercano with safe generation coexistence

Specification approved by the user — 2026-10-27. The user authorized preparation of the phased execution plan for separate execution approval. Isolated production acceptance, future A/B generation qualification and copy-based adoption/reconciliation rehearsals must precede any request to replace the everyday launcher or perform live cutover. Specification approval does not authorize implementation, live migration, installed-launcher replacement, native credential access or stopping existing processes. Previously authorized bounded backups and exact rejected-snapshot cleanup are separate completed operations.

## Problem and product contract

The everyday launcher was changed to a prepared development application that has separate history, a copied and frozen configuration, guarded credentials, omitted production services and restricted terminal commands. Although its isolated generation journey passed synthetic acceptance, that did not establish an upgrade of the user's existing Cercano. The required product is the existing full production application, with its existing history, settings, credentials, integrations and everyday functionality, plus safe same-machine generation coexistence.

Generation support is a lifecycle capability of the production application, not a second reduced-feature application. Existing ownership, prepared-store validation, immutable publication, pinned worker launch, authenticated identity and transfer mechanisms are reusable foundations, not sufficient evidence of product parity.

## Approved direction and scope

Preserve authoritative path resolution, including supported environment/configuration overrides. The ordinary conversation authority is CERCANO_CONVERSATIONS_DB when set, otherwise ~/.config/cercano/conversations.db. Configuration remains at config.DefaultPath(), ordinarily ~/.config/cercano/config.yaml. Permissions, external Model Context Protocol (MCP) definitions, catalog cache and telemetry retain their existing configuration-relative locations. Enterprise retains ~/.cercano/enterprise and existing override semantics. Credentials remain in existing OS-backend services/profile identities; they are not exported, duplicated into generation directories or relocated. Configured model and download directories remain authoritative.

The development conversation database is a bounded reconciliation source, not the future primary database. Preserve its useful conversation content without importing stale host ownership, handover reservations or live task execution authority. Config copies do not become a competing source of truth. Detect any meaningful divergent configuration and report a non-secret conflict rather than silently overwriting existing settings.

Project plans, research output and tool-created files remain project/caller-owned outputs. No relocation or general file migration is required. Catalog caches may be reconstructed; history, telemetry/accounting and downloaded model assets must not be discarded under that classification.

The existing application must remain usable through history browsing/resume, new conversations, settings, permission controls, model/profile selection, login/logout and token refresh, external MCP, enterprise policy where configured, ordinary dispatch and nested subagents, independent tasks, autonomy, search, compaction/recap/retention, metering/accounting, runtime/model management and diagnostics. Existing opt-ins and security restrictions remain valid; new blanket coexistence restrictions do not count as parity. Where integration is incomplete, do not activate the new default path.

Initial native qualification targets macOS. Other platform behavior must not regress, but native coexistence claims require platform-specific evidence. Real-provider, native credential, and real loaded-model qualification are separately authorized checks; synthetic tests cannot substitute for or imply them.

## Production integration boundary

Use the ordinary production dependency graph as the canonical composition path. Factor lifecycle/store/worker policy inputs into that composition rather than copying selected wiring into another factory. Both compatible generation startup and any retained single-host startup must use the same feature wiring and tests. Prepared ownership must remain effective throughout ordinary services; calling legacy Open or SelectExecutionMode to bypass missing integration is forbidden.

Startup validates build identity, protocol and persistent representation compatibility before migrations, task reconciliation or other persistent changes. It must not unconditionally reap other generations' workers, replace a shared runtime, migrate inline credentials, or reconcile another live host's tasks. All workers are pinned to their host generation and launched with explicit prepared authority. Restore normal terminal initialization, command discovery and controls, not just an allowlist of generation commands. Unsupported commands must never be advertised as functional silent no-ops.

## Conversation ownership and everyday resume

Each conversation has one durable owning host instance and acquisition epoch. Store mutation checks and owner validation occur within the same transaction. Admission covers foreground requests, worker callbacks, child creation, task acceptance/execution/completion, notification claims/acknowledgements, passive attachments, scheduler work and administrative writes. Child operations retain both required parent and child authority; nested dispatch is not permanently disabled.

History browsing includes existing authoritative history. Resuming an unowned conversation performs explicit authorized acquisition. Resuming one retained by a live host routes to that exact host; it must not silently steal ownership. Moving a conversation is an explicit, quiescent transfer. The terminal executable remains unchanged and displays terminal and connected-host identities separately. Multiple terminal attachments prevent unsafe release or transfer. Disconnect detaches a terminal without releasing the session or cancelling accepted independent work. Foreground transport-bound work keeps its documented cancellation semantics; this effort does not silently promise foreground survival.

Close source admission and verify quiescence before destination participation. A definite busy refusal creates no destination reservation. Destination acceptance is bound to conversation, source epoch, handover identifier and exact destination instance under its retirement fence. Pending or ambiguous outcomes retain exact retry information and conservative blockers. Success is reported only after durable destination acquisition with a fresh epoch. Stale reconnect never launches, reacquires, migrates or respawns a retired host.

Retirement is explicit and refuses retained sessions, unresolved transfers, callbacks, worker/task activity and unfinished persistence. It closes only that host's participation. A different generation and its runtime holds remain usable. Cancellation/Shutdown is not a readiness test. Crash recovery remains fail-closed where owner death or continuing effects cannot be established; external effects are not replayed to manufacture completion.

## Shared mutation and cross-host visibility

Serialization and visibility are separate requirements. A mutex in one host, a native credential backend or a filesystem watcher alone provides neither complete contract.

Configuration, permissions and MCP definitions retain their existing stores. Mutations must read current state under cross-process coordination, validate and apply the requested change, atomically publish, and reject incompatible stale whole-state updates. Reuse existing watcher/apply paths for convergence without watcher-induced save loops. Return success with a precise distinction between durable save and live application. A host may not acknowledge a new revision while continuing to admit work using invalidated policy. Non-hot-reloadable changes preserve ordinary explicit restart guidance; they must not trigger surprise host replacement.

Credential refresh/login/logout use the existing credential services. Serialize by existing credential identity across participating hosts, reread after obtaining authority, and prevent a late refresh from resurrecting a logged-out credential. OAuth token rotation and its ambiguous external failures are not made atomic by a local lock: unknown outcomes require bounded reconciliation or reauthentication, not blind refresh replay. Other hosts invalidate stale credential/provider state before subsequent authorized use. No secret material appears in revision notifications, coordination records, logs or fixtures. Existing workers use host-mediated authority instead of independently opening writable credentials.

Enterprise connection locking and policy admission remain intact. Do not bypass the existing lock to start a second credential/policy owner. Compatible hosts must obtain the current enforced policy through existing enterprise/host interfaces, with revision and revocation propagation and fail-closed admission when policy freshness cannot be established. Extend existing interfaces only as necessary; a new enterprise service or general distributed coordinator is not presumed.

Reuse the shared model host as runtime mutation authority. Preserve caller holds, operation admission and reconnect semantics. Restore downloads, cancellation, model selection and ordinary runtime administration with explicit impact reporting and safe handling of sibling-held resources; blanket denial is not the completed design. No fallback manager may race the shared authority over the same assets. Destructive operations must drain or explicitly refuse busy resources, rather than unload another host's active model silently.

External MCP definitions are shared, while configured subprocesses/connections have explicit ownership. Reuse the current manager, discovery, defunct-handle and teardown mechanisms. Retirement cannot close another host's connection or process. Integrations that require exclusive process ownership must retain that exclusivity through existing host interfaces; do not assume arbitrary user-configured servers tolerate duplicate starts. Preserve ordinary tool and policy behavior across reconnect and definition changes.

Reuse telemetry SQLite coordination and transactional accounting. Ensure identity/revision-based deduplication, compatible schema opening, collector drainage and acknowledged accounting preservation across retirement. Do not create a telemetry service merely to serialize writes already handled by SQLite.

## Background work and external outputs

Recap, compaction, retention, metering, catalog refresh, watchdog and accounting producers must be wired through the full application. Conversation-specific scheduling carries owner/epoch and counted work through writes and callbacks, including timers, retries and descendants. Persistence errors suppress dependent success publication and block readiness until resolved. Global maintenance operates under bounded cross-host maintenance authority and cannot delete another owner's active records. No long database transaction spans model/tool/network execution.

Atomic file replacement does not prevent stale project-plan edits. Preserve existing output paths and add narrowly scoped conflict checking where concurrent generation work can target the same file; do not build a new project-files database. Research jobs use explicit output ownership or report collisions. Diagnostic files may be host-scoped, but changing diagnostic naming cannot hide failed persistence or erase retained evidence.

## Initial adoption and development-record reconciliation

Initial adoption is separate from ordinary future startup. Legacy writers do not become ownership-aware because tables were added. A cutover must establish exclusive control of every writer to the authoritative store, including background processes and external clients; unavailable endpoint or free conversation lock alone is not sufficient evidence. No live adoption runs during planning or normal auto-launch.

Build and rehearse an idempotent, explicit migration on disposable copies. Preserve existing conversation IDs, turn ordering and content, task outcomes/notifications, autonomy records, routes, compaction, dispatch and usage data. Add only ownership/compatibility metadata required for participating generations, and version actual semantic/schema changes. Never stamp an unsupported representation as compatible. Bound lock duration, temporary disk use and cancellation; stop before mutation if prerequisites fail.

Reconcile development records by identity and content. Identical records are no-ops; unique supported history is imported with relationships intact; conflicting IDs/content stop for explicit resolution without overwriting. Ownership and handover rows are never imported as active authority. Existing application data wins no automatic destructive conflict policy. Rehearse reruns, interrupted phases, corruption, foreign keys and rollback boundaries. Config/credential migration is not implied.

The authorized backups completed on 2026-10-27. Evidence reports legacy 3,885 conversations and 315,418 turns, development one conversation and one turn, and no shared IDs in compared tables. These are separate point-in-time copies, not proof of current writer quiescence or a final cutover snapshot. Revalidate at cutover. A validated rollback copy and tested restore procedure are required. After new-schema writes begin, an old binary must not reopen the upgraded store; restoring an old snapshot would lose subsequent writes and requires explicit acknowledgement rather than automatic rollback.

## Acceptance and activation gates

Acceptance must run through the ordinary production composition and terminal entrypoint. Dependency injection is confined to disposable credential stores, protocol-faithful provider/MCP/enterprise fixtures and runtime test managers, not substitutes for the application graph. Tests must prove existing history resume, configuration and permission mutation/visibility, credential refresh/logout races, integration discovery/policy changes, nested dispatch, independent task survival, background scheduling and accounting. Credential-enabled test scopes require injected stores and transports in every child; HOME isolation is not native Keychain isolation.

The multi-generation journey includes new A work, a physically disconnected terminal with accepted independent work, publishing B while A remains active, unchanged-generation concurrent reuse, unrelated conversation transfer while A's task continues, busy transfer with zero new destination reservations, exact reconnect, exactly one acknowledged completion within the defined non-crash test, and explicit A retirement with B still usable. Exercise real compiler failure without disrupting running state. Separately qualify real loaded-resource hold behavior when authorized. Compile-time synthetic providers must be clearly identified in results.

Run focused race and fault-injection tests for ownership/admission, callbacks, concurrent mutation, SQLite contention and cancellation. Run full server and CLI suites and builds on the exact activation candidate. Record skips and fixture boundaries; passing package tests is not production credential or native inference qualification. Verify negative paths: stale host/epoch, malformed metadata, unsupported schema, incomplete publication, failed persistence, lost responses, process restart, symlink/path confusion and overly long Unix socket paths.

Activation requires product-parity evidence, copy-based adoption rehearsal and explicit live-cutover authorization identifying writers, backup/rollback artifacts and downtime window. Launcher replacement, stopping/restarting processes, live migration and native credential access remain separately authorized operations. No push or release publication is authorized by this specification.

## Infrastructure discipline and decisions

No new product-scope choice is needed: existing authoritative paths, full production integration and non-disruptive generations were specified by the user. A reduced-feature prepared application, a permanent second history/configuration store, and concurrent unfenced legacy writers do not satisfy those requirements.

Routine lock layout, watcher debouncing, callback factoring and test organization are execution details. Reuse SQLite transactions, the existing OS-lock pattern, host identity/authentication, model-host holds, config watchers, MCP manager and enterprise admission. Add only narrowly scoped coordination metadata or interface extensions supported by concrete race tests. If implementation demonstrates that a new long-lived authority, changed credential trust boundary or incompatible persisted representation is unavoidable, surface that consequential decision before code depending on it; do not present speculative infrastructure options as prerequisites to planning.

## Delivery order and review boundary

First integrate and prove full production composition, ownership-aware work and shared mutation/visibility in isolated fixtures. Then implement and rehearse minimal authoritative-store adoption and development-record reconciliation on copies. Then conduct a separately authorized one-time live cutover with fresh backup, writer exclusion and rollback gates. Subsequent compatible generations use ordinary startup without repeating migration or interrupting existing hosts. Representation-incompatible updates require a separately reviewed compatibility/cutover procedure.

The phased checkbox execution plan follows human sign-off of this specification under the required planning protocol. Review does not authorize implementation. The audit is bounded and closed for planning purposes: unresolved external-server behavior, runtime variants and consumer refresh paths become concrete tests and integration tasks, not another broad inventory loop.
