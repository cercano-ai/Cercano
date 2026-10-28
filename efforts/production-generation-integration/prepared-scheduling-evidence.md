# Bounded prepared recap/compaction scheduling integration

Scope: explicit `PreparedServerOptions.Scheduling` only. Canonical/default startup does not install these producers. Shared configuration remains frozen; no telemetry/config files, launcher, credentials, providers, runtime models or subprocess fixtures were changed. Phase 2 remains incomplete.

Source baseline: `533f12c29fef28641209973fed0aa1619874b010`; the checkpoint containing this evidence records the explicit integration files.

## Integration

- Retain `TestPreparedSchedulingGap`, the pre-integration gap probe: admitted assistant persistence leaves no producer ticket without explicit scheduling injection.
- Prepared construction clears legacy and preexisting owned schedulers, then optionally constructs host-owned generators on its prepared store.
- Persistence assistant append and history assembly carry their incoming context into the optional agent scheduling bridge. Agent foreground turn persistence does likewise. Legacy scheduling and legacy append-error continuation remain unchanged when injection is absent.
- Scheduler callbacks derive exact authority from incoming prepared capability/epoch, not current-owner lookup by conversation ID; `BeginRetainedWork` admits before timer creation through existing scoped persistence bridges.
- Background fence closes producer admission; join and shutdown wait for producer Close, including final callbacks. Cancellation alone is not completion.

## Focused offline verification

Run from `source/server`, 2026-10-28:

```sh
CERCANO_CREDENTIALS_DISABLED=1 GOPROXY=off GOSUMDB=off go test -race \
  ./internal/server ./internal/agent ./internal/hostsvc/persistence \
  ./internal/compactiongen ./internal/recap \
  -run 'TestPreparedScheduling|TestPreparedOwnedCompletion|Test.*Owned|Test.*Legacy|Test.*Retained|TestAgent_storeConversationTurn' -count=1
```

PASS all five packages (server 4.358s, agent 1.562s, persistence 3.426s, compactiongen 3.024s, recap 2.088s). `git diff --check` passed before evidence addition.

Integration tests cover disabled/default gap, pending recap/compaction/both timers, forged epoch/ambient/sibling refusal, release and accepted-destination handover busy refusal without source reservation, fence/join drain, final usage callback held across shutdown and handover, history assembly while shared config stays frozen, and scoped callback write failure retaining a conservative readiness blocker after join. Agent tests cover incoming foreground context preservation and legacy scheduling parity. Existing generator/bridge owned tests cover retry and persistence boundaries; they are included in the focused command.

Limitations: no full suite/build acceptance, native credentials, real providers/models, subprocess acceptance or canonical activation. Prepared ordinary request admission safeguards remain in place. This does not establish complete foreground/descendant integration, broader producer parity, full cross-host production acceptance or completion of any whole plan phase. Unresolved producer persistence failures intentionally retain blockers and require explicit reconciliation.
