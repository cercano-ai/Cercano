# Bounded prepared recap/compaction scheduling integration

Scope: explicit `PreparedServerOptions.Scheduling` only. Canonical/default startup does not install these producers. Shared configuration remains frozen; no telemetry/config files, launcher, credentials, providers, runtime models or subprocess fixtures were changed. Phase 2 remains incomplete.

Source baseline: `533f12c29fef28641209973fed0aa1619874b010`; the checkpoint containing this evidence records the explicit integration files.

## Integration

- Retain `TestPreparedSchedulingGap`, the pre-integration gap probe: admitted assistant persistence leaves no producer ticket without explicit scheduling injection.
- Prepared construction clears legacy and preexisting owned schedulers, then optionally constructs host-owned generators on its prepared store.
- Persistence assistant append and history assembly carry their incoming context into the optional agent scheduling bridge. Agent foreground turn persistence does likewise. Legacy scheduling and legacy append-error continuation remain unchanged when injection is absent.
- Scheduler callbacks derive exact authority from incoming prepared capability/epoch, not current-owner lookup by conversation ID; `BeginRetainedWork` admits before timer creation through existing scoped persistence bridges.
- Background fence closes producer admission; join and shutdown wait for producer Close, including final callbacks. Cancellation alone is not completion.

## Focused verification

The original child-reported command below uses `CERCANO_CREDENTIALS_DISABLED`, which is **not** the credential-policy variable. It is retained verbatim for provenance and must not be treated as proof that credential admission was disabled in that run. Read-only review `task_04d7df9bc8326ee429adf74f11ae7924` found no reachable native opener, real-provider/external-network operation, or subprocess route in the selected fixtures; server package initialization uses mocked in-memory bufconn. This is source assessment, not observed execution or a sandbox guarantee. No claim about actual native access is inferred from the typo alone.

Originally reported from `source/server`, 2026-10-28:

```sh
CERCANO_CREDENTIALS_DISABLED=1 GOPROXY=off GOSUMDB=off go test -race \
  ./internal/server ./internal/agent ./internal/hostsvc/persistence \
  ./internal/compactiongen ./internal/recap \
  -run 'TestPreparedScheduling|TestPreparedOwnedCompletion|Test.*Owned|Test.*Legacy|Test.*Retained|TestAgent_storeConversationTurn' -count=1
```

PASS all five packages (server 4.358s, agent 1.562s, persistence 3.426s, compactiongen 3.024s, recap 2.088s). `git diff --check` passed before evidence addition.

Parent verification used the **correct** explicit environment `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, the same package/test selection, and `-count=2`. It passed with exit 0: server 6.737s, agent 2.175s, persistence 5.645s, compactiongen 1.736s, recap 2.888s (2026-10-28). This is a new correctly gated run, not retrospective proof of isolation for the original run.

Integration tests cover disabled/default gap, pending recap/compaction/both timers, forged epoch/ambient/sibling refusal, release and accepted-destination handover busy refusal without source reservation, fence/join drain, final usage callback held across shutdown and handover, history assembly while shared config stays frozen, and scoped callback write failure retaining a conservative readiness blocker after join. Agent tests cover incoming foreground context preservation and legacy scheduling parity. Existing generator/bridge owned tests cover retry and persistence boundaries; they are included in the focused command.

Limitations: no full suite/build acceptance, native credentials, real providers/models, subprocess acceptance or canonical activation. Prepared ordinary request admission safeguards remain in place. This does not establish complete foreground/descendant integration, broader producer parity, full cross-host production acceptance or completion of any whole plan phase. Unresolved producer persistence failures intentionally retain blockers and require explicit reconciliation.

## Bounded ordinary production constructor extraction (2026-10-28)

Source baseline: `009c72ba5999f05d9f6e7cb2d935c2867b6efc58`; this checkpoint contains only `cmd/cercano/main.go`, `production_scheduling.go`, `production_scheduling_test.go`, and this evidence file. No plan checkboxes changed.

`composeProductionScheduling` now owns the ordinary recap and store-backed compaction constructor sequence, including recap model override, BeginWork callback, existing loopcompact summarizer/config/fresh inline factory, diagnostics/runtime/routing callbacks, debounce/max-turn options, enabled/tool-elision flags and optional attempt sink. Dependencies and store are invocation-local. Ordinary startup passes its existing provider, enterprise Begin callback, live dispatch candidates, runtime context accessor and stderr diagnostics; it explicitly retains ID-only scheduler installation. Its existing later collector setup, generator registration, fence/join and retention code remain in place.

`preparedProductionScheduling` adapts that same constructor to `PreparedServerOptions.Scheduling`, capturing no store: only the store supplied by prepared construction is used. It neither schedules nor installs ID-only producers. The existing prepared server installer remains responsible for exact incoming ownership validation, retained-work admission and owned callbacks, and for fence/join.

### Authority choice and exact missing caller

**No production-development or canonical activation.** The missing caller is `newProductionDevelopmentHostWithDependencies` in `cmd/cercano/development_host_production.go`, specifically its `server.NewPreparedDurableServer(... PreparedServerOptions{...})` call (currently around line 248). It does not supply `Scheduling`. This bounded slice deliberately leaves it unchanged. Before passing `preparedProductionScheduling` there, that caller must assemble the ordinary authority-safe producer callbacks: enterprise/settings Begin admission that preserves the retained incoming context; the live dispatch candidate graph and recap tier/provider; the runtime context accessor and diagnostics; and the accounting collector/sink with a lifecycle that outlives all final producer callbacks. Ordinary inline dispatch compaction installation must also be composed consistently. A local `openRunner` alone is not proof of that wiring. Supplying a no-op Begin or a partial provider graph here would hide the outstanding full-production integration and is not enabled.

### Current-slice verification

Tests use only fake inference and temporary SQLite stores. `TestProductionSchedulingCommonFactory` exercises both ordinary legacy and prepared factory construction, recap and compaction, successful persistence and shutdown while a final usage callback is held (eight cases). It verifies prepared incoming-context propagation versus legacy background-context policy, stale admission rejection, the exact supplied store despite a separate same-ID store, an independent SQLite connection committing while inference is blocked, retained ownership blockers, fence allowing active work to finish, and Close joining final callbacks. Cancelled prepared persistence conservatively retains its blocker. These are constructor/bridge tests, not proof of a fully composed production-development host. The existing server tests cover the actual owned scheduling installer and disabled/default gap.

Initial fixture runs failed because compaction inherited the default Secondary destination and the fake response used Markdown headings instead of the compaction parser's `GOAL:`/`STATE:` format. Only fixture routing/response were corrected; production routing/parser and owned generators were not changed to obtain a pass.

Commands run from `source/server`, with the **correct credential-policy variable** throughout this slice:

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race \
  ./cmd/cercano -run 'TestProductionSchedulingCommonFactory|TestProductionComposition' -count=2
# PASS: cmd/cercano 5.501s

CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race \
  ./internal/server ./internal/agent ./internal/hostsvc/persistence \
  ./internal/compactiongen ./internal/recap \
  -run 'TestPreparedScheduling|TestPreparedOwnedCompletion|Test.*Owned|Test.*Legacy|Test.*Retained|TestAgent_storeConversationTurn' -count=2
# PASS: server 5.054s, agent 2.239s, persistence 5.596s,
#       compactiongen 2.962s, recap 3.254s
```

`git diff --check` passed. No full-suite/build acceptance, native credentials, real inference/providers/models, subprocess acceptance or live user-state qualification is claimed. No retention/global-maintenance, shared-config thaw, launcher, canonical activation, schema or owned-generator work was performed. Phase 1 composition and Phase 2 producer progress remain partial: the common producer factory is tested, but the exact production-development caller wiring above and broader production authority/lifecycle parity remain outstanding.
