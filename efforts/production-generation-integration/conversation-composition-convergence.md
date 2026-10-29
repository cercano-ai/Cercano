# Shared concrete conversation composition — 2026-10-28

First bounded convergence slice; not full production parity or activation evidence.
Baseline HEAD: `1015d119` on `feat/production-generation-integration`.

## Implemented

`composeProductionAgent` in `cmd/cercano/production_composition.go` now constructs
both ordinary and generation-host agents. It binds the caller's exact persistent
store and routing graph, creates the session conversation adapter, project loader
and per-host context meter, and installs explicitly supplied ordinary schedulers.
It opens no store, provider, credential backend or runtime. Missing store/routing
fails rather than constructing a degraded graph.

Removed the duplicate session-adapter, loader, meter and agent-option assembly
from `main.go`, including the separate recap/compaction option block. Replaced the
parallel `agent.NewAgent` and loader construction in
`development_host_production.go`. Its callback still obtains the exact store via
`admission.conversationStore`; no legacy opening or execution-mode replacement
was introduced. The prepared graph now includes the same context metering as
ordinary construction. Existing shared routing/dispatch/capability seams remain.

The registration integration test now uses the shared concrete agent constructor
for both servers, the durable prepared server constructor, and prepared pinned
worker installation (without launching a worker). It checks exact store identity,
service registration, independent meter state, loader isolation and continued
MCP suppression. Additional dependency validation rejects incomplete graphs.

## Verification

From `source/server`, all commands used
`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

- `go test ./cmd/cercano -run '^TestProductionComposition' -count=1` — PASS.
- `go test -race ./cmd/cercano -run '^TestProduction(Composition|SchedulingCommonFactory)' -count=1` — PASS (final source).
- `go test ./internal/server -run '^Test(CoexistenceLifecycle|CoexistenceAcquireTicketAndRuntimeIsolation|PreparedAdminStaleAdmission)' -count=1` — PASS.

Tests use temporary/in-memory stores, memory credentials and synthetic,
non-dialing providers. Credential policy was never enabled. No compiled subprocess
fixture, full suite, native credentials, live runtime, paid inference, migration,
installed launcher, process stopping or push was exercised. Lifecycle tests are
focused regression evidence, not a newly qualified compiled A/B journey.

## Exact retained limits and removal conditions

- `newProductionDevelopmentHostWithDependencies` still supplies empty scheduling
  to the shared agent and no `PreparedServerOptions.Scheduling`. Ordinary ID-only
  schedulers cannot be installed on this host: connect the existing prepared
  scheduling factory with authority-safe provider callbacks and counted exact
  owner/epoch work, including shutdown joins, before enabling producers. Global
  retention additionally requires cross-host maintenance authority.
- `composeProductionServices` still calls `InitWatchdog` only for legacy policy.
  Connect coordinated runtime/policy admission and host-owned shutdown before
  enabling prepared watchdog activity.
- `composeProductionMCP` still returns without manager construction for prepared
  policy. Coordinated shared-definition updates and explicit exclusive integration
  ownership/teardown must be connected before removing this gate.
- `newProductionDevelopmentHostWithDependencies` still uses snapshot config,
  guarded read-only credentials and strict session-local permissions; prepared
  server config remains frozen. Shared config/permission mutation and visibility,
  and coordinated credential refresh/login/logout/invalidation must be wired into
  this caller before relaxing those restrictions. Existing independent seams do
  not constitute caller integration.
- The generation caller still omits ordinary catalog/accounting and enterprise
  wiring and retains protected prewarmed-runtime attachment. Those service and
  authority integrations are outside this conversation-graph slice.

No gates were removed. Exact owned-store binding, explicit reopen, pinned workers,
busy refusal/no deferred transfer and lifecycle A/B contracts are unchanged.
Ordinary and prepared **server startup orchestration is not yet unified**; this
commit removes the duplicated concrete conversation graph, not the remaining
parallel startup, terminal restrictions or authority gaps. Phase 1 remains partial.
