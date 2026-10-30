# Bounded production-generation audit

Status: planning only, 2026-10-27. No implementation, migration, cutover, shutdown, replacement, backup execution or cleanup authorization is implied. Existing authoritative paths and the full production application are the baseline. Do not maintain a reduced-feature everyday application.

## Stopping rule

Inventory application-owned durable state by subsystem, not every emitted file. For each category record path resolution, writers, cross-host mutation and visibility behavior, and an acceptance obligation. Classify project/external outputs separately and never make relocating them an application migration requirement. Classify reconstructible caches separately from history/accounting. Stop a category after tracing its production constructor, persistence boundary and reload/consumer boundary; explicitly mark unresolved boundaries. Do not recursively scan user directories. No more raw live database copies. Remaining source work is limited to MCP manager configuration/process lifecycle, telemetry writer/open compatibility, model download/cache authority, and configuration/credential/enterprise notification boundaries. Unknowns remain blockers for the relevant design claims, not reasons to repeat broad audits.

## Inventory and acceptance obligations

These are source findings and planned tests, not executed acceptance.

| Classification/category | Authority and writers | Coexistence/visibility | Required acceptance |
|---|---|---|---|
| Application: conversations, tasks, autonomy, dispatch and usage records | CERCANO_CONVERSATIONS_DB or ~/.config/cercano/conversations.db; conversation store and host persistence/task/background producers | Prepared ownership fencing exists; legacy writers do not participate. Initial adoption requires separate writer exclusion. | Two-host fencing, sibling preservation, transfer/drain, producer and notification coverage; rehearsal on copies |
| Application: configuration | config.DefaultPath(): ~/.config/cercano/config.yaml; config.Save, service mutations and startup callbacks | Prepared path freezes a copied snapshot; ordinary startup watches shared config. Watcher presence does not prevent stale saves or prove ordered cross-host visibility. | Concurrent and stale saves; acknowledged revision visible in both hosts; provider reconstruction; external edit/error handling |
| Application: permissions | Beside config: permissions.yaml; agent.PermissionStore setters | Process-local mutex and direct WriteFile; Mode rereads file, server also watches. Full cached mode/MCP allowlist save can race other hosts. | Conflicting mode/allowlist updates preserve both intended changes or explicitly reject stale update; both hosts enforce acknowledged policy |
| Application: credentials | OS backend, Cercano service/profile identifiers; auth flows, refresh and credential service | Prepared guard is read-only, not full refresh/login parity; native store separate from file paths | Memory-backed concurrent refresh/login/logout, invalidation and token visibility; no fallback to native stores in isolated acceptance |
| Application: enterprise | ~/.cercano/enterprise; host/manager active bundle, managed marker, connection.lock, credential backend | Existing exclusive connection lock and bundle rename; cross-generation policy distribution still requires trace | Policy change/revocation reaches all enforcing hosts; admission and credential refresh coordinated; no lost managed state |
| Application: telemetry/accounting | telemetry.db beside config; startup collector | Not classified as disposable merely because diagnostic; multi-process open/schema and retention semantics remain unverified | Simultaneous writers, accounting completeness, compatible opens and retention |
| Application/integration: MCP definitions and lifecycle | Production MCP-manager construction still to trace; do not confuse internal/mcp tool server with external MCP manager | Prepared factory omits manager installation; process sharing and settings visibility unresolved | Real protocol fake servers: configuration changes, tool discovery, policy, disconnect/restart and ownership-safe teardown |
| External/project: effort plans | Caller-selected plan.md, commonly project efforts; taskmodel.PlanStore | Atomic temp+rename; process-local serialization, no watcher, documented stale in-memory tree | Competing edits must not silently overwrite; reopen/refresh behavior explicit. No relocation requirement |
| External/project: research outputs | Caller/base-relative output directory, default scratch/research/topic; pipeline, report/progress and sidecar writers | File ownership/concurrent job collisions need bounded output coordination, not DB migration | Separate-job isolation and explicit same-path conflict handling; preserve outputs in place |
| Disposable: catalog cache | catalog-cache.json beside config; catalog manager periodic refresher | Load failure tolerated; refresh producer needs lifecycle/atomic publication checks | Missing/corrupt cache rebuild and concurrent refresh safety; no required migration |
| Diagnostics | crash.log beside config; development routing/failure logs under hosts/host | Host-local log destinations possible; retention and active writers must be respected | No log clobber or unsafe cleanup; no requirement to merge logs into history |
| Runtime/model assets | Still to trace manager defaults and overrides | Model files/downloads are not automatically disposable; shared runtime holds and mutations already have protection | Shared downloads/model usability and A retirement without disrupting B |

## Sources inspected this turn

- cmd/cercano/main.go:435–535: catalog cache, permission store/watcher, config persistence/watcher, telemetry paths and wiring.
- internal/agent/permissions.go:148–185: disk reread, local mutex, whole-file persistence.
- internal/taskmodel/plan_store.go:1–130: caller-selected path, atomic publication, no watcher and explicit staleness caveat.
- cmd/cercano/enterprise.go:30–35: authoritative enterprise directory.
- Earlier retained startup audit and database inventory were inspected; neither establishes complete inventory or native acceptance.

## Supported consistent backup assessment

SQLite Online Backup API: https://www.sqlite.org/backup.html (fetched this turn). Incremental backup takes read locks during steps, allows other connections to write between steps, and may restart on external writes. Persistent writes can prevent completion. Completion gives a consistent snapshot, not exclusive access, and not a snapshot of multiple databases at one common instant.

Recommend a bounded Online Backup API attempt, not raw copying and not VACUUM INTO: the latter rebuilds/compacts and is unnecessary for an identity-preserving inspection copy. Source must be opened read-only through SQLite with normal locking (not immutable=1); SQLite/WAL shared-memory participation can occur, so this is a new permission scope compared with ordinary file reads. Do not call application store Open, migrations, reconciliation, VACUUM, journal-mode changes, explicit checkpoint, or source SQL writes.

Proposed authorization: one incremental attempt per known legacy/development database into a new private directory; 10-minute overall deadline, bounded busy waits, incremental steps; stop rather than escalate if busy, space-limited or deadline exceeded. Validate completed destinations only and compare IDs/content hashes without displaying message bodies. Check free capacity first; initially require at least 30 GiB free for the approximately 19.24 GB legacy copy plus growth/headroom, monitor a 10 GiB free-space floor, and stop before destination exceeds 24 GiB. These are proposed conservative limits, not measured free-space availability or guaranteed completion. Avoid holding a long source read transaction to force progress because WAL growth/checkpoint interference must remain bounded. Interrupted destinations remain marked incomplete pending cleanup authorization.

## Retained sensitive snapshots: metadata-only current inventory

Root T = /var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T

- T/cercano-db-evidence-20gdwqsu/legacy/conversations.db: 19,236,786,176 bytes.
- T/cercano-db-evidence-20gdwqsu/legacy/conversations.db-wal: 15,656,032 bytes.
- T/cercano-db-evidence-20gdwqsu/development/conversations.db: 135,168 bytes.
- T/cercano-db-evidence-20gdwqsu/development/conversations.db-wal: 65,952 bytes.
- T/cercano-db-evidence-20gdwqsu/manifest.json: 3,335 bytes. Combined capture rejected as unstable; not valid legacy evidence.
- T/cercano-dev-evidence-p7if48qy/conversations.db: 135,168 bytes.
- T/cercano-dev-evidence-p7if48qy/conversations.db-wal: 65,952 bytes.
- T/cercano-dev-evidence-p7if48qy/query/conversations.db: 135,168 bytes.
- T/cercano-dev-evidence-p7if48qy/manifest.json: 1,040 bytes.
- T/cercano-dev-evidence-p7if48qy/safe-record-inventory.json: 1,824 bytes.
- T/cercano-dev-evidence-p7if48qy/schema-inventory.json: 13,749 bytes.

Sizes are logical file sizes from directory metadata, not allocated disk usage. No contents read in this inventory. Proposed cleanup for separate approval: delete only the rejected combined-capture directory; retain the accepted development evidence until consistent comparison is complete, then request its deletion. Ordinary unlink is not guaranteed secure erasure on snapshots/backups/SSD storage. No cleanup performed.

## Subsequent document gates

After the bounded audit and any real coordination choices are reviewed, write spec for human sign-off, then phased plan. Distinct phases: full production integration and isolated acceptance; minimal ownership/schema adoption and bounded development-record reconciliation; separately authorized one-time legacy cutover; future non-disruptive generations. Coordination must include mutation serialization/conflict handling AND visibility/invalidation across hosts. Prefer existing stores, watchers, enterprise lock/policy mechanisms and runtime holds; justify new durable coordination only where these cannot enforce the contract. Planning approval must not silently authorize live cutover.
