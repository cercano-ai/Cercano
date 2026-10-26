# Bounded implementation resumption — 2026-10-26

## User-authorized execution clarification

Implement normal A/B coexistence, explicit release/handover, exact-host reconnect and idle-host retirement first. Then add conservative absence-based crash recovery. Do not conduct further general process-identity research or add infrastructure. Ambiguous recovery may refuse with a clear diagnostic.

A held conversation lock blocks crash recovery. The source retaining its own conversation lock throughout normal handover readiness and durable relinquishment is expected, not a blocker.

The supported recovery path requires acquired exclusion, direct positive-PID `kill(pid, 0)` reporting `ESRCH`, matching durable owner/epoch or handover reservation, and accounting for potentially surviving work. Success, `EPERM`, other errors and unresolved external work refuse recovery. No external-effect replay. No native qualification is claimed by this statement.

## Verification priorities

- Atomic ownership/epoch checks on every owned write, with suppression of dependent stale completion effects.
- B startup does not migrate/reconcile A's active work.
- Outstanding work, callbacks and unresolved writes prevent transfer without cancellation.
- Retirement releases only the caller's participation and preserves sibling hosts and shared runtime.
- Ordinary reconnect never launches or reacquires.

## Initial implementation evidence

Delegated compatibility implementation task `task_949ccf6d36a5a85d7dfd838b341c56a9` completed at commit `c69ad0c717c9f9e657a9d1c35179e2b9617d3e92` on `feat/dev-host-generations`. It reports passing:

```
# source/server
go test ./internal/conversation -count=1
go test -race ./internal/conversation -run Coexistence -count=1
```

The parent read the new compatibility implementation. APIs prepare a new explicit fixture, perform read-only metadata preflight, and open without migrations/reconciliation. Normal Open is unchanged. At this commit the declaration is explicitly unfenced; application coexistence is not enabled. Metadata is not physical-schema attestation. Tests use synthetic temporary stores, not real user data. These are delegated test results, not native A/B acceptance.

## Concrete integration inventory follow-up

Task `task_b50a286491cb0c63d38d3656c877352c` inspected concrete broker/server/taskrun/persistence source. It reports:

- Unary and legacy streaming admission must precede persistDevMode, not only broker-backed streaming.
- Task acceptance needs a ticket before InsertTask; terminal persistence replaces context with Background, so ownership authority must be retained explicitly.
- FinishTask's affected-row boolean is discarded; successful SQL without a transition must not publish completion.
- PersistTurn and context/compaction callbacks log failures; unresolved writes need explicit readiness accounting.
- Context reads may warm caches; incremental regeneration starts background work; recap, retention, compaction and dispatch evidence bypass a main-turn-only fence.
- Passive Observe/ReplayPage, initiating streams and task subscriptions need epoch-bound attachment accounting.
- taskrun.New initializes and globally reconciles tasks: coexistence must use a prepared-store construction path with no automatic reconciliation.
- Admission tickets precede broker/manager mutexes; store checks must not call back into admission while those locks are held. Short transactions finish before notifications/callbacks.

Coverage remains incomplete for all autonomous-ledger/prompt callers, recap/retention/compactor scheduling and draining, worker persistence bridges and cross-conversation creation/rollover, and stream detach races. This note does not mark Phase 1 complete.

## Task lifecycle prerequisite

Task `task_afc0d0965c2cce7cf922ef8f2acad737` committed `6486a5f51632`. Its fake-store probe reproduced publication after `FinishTask(false, nil)` before the fix. Rejected terminal transitions now expose `ErrTerminalTransitionRejected`, retain unresolved outcome state, and suppress wakes/notifications. `NewPrepared` skips initialization and global reconciliation; legacy `New` is unchanged. `UnresolvedTerminalOutcomes` exposes blockers for later admission integration. The child reports three focused synthetic tests passed with `-race -count=20`. The parent additionally ran `go test ./internal/taskrun -count=1` successfully (2026-10-26). This is not full quiescence enforcement or native B-startup qualification.

## Additional committed foundations

- `a1d7e119ae50`: strict credential environment parsing and rejecting secrets/enterprise stores before native backend opening. Delegated hermetic opener-trap tests passed; enabled controls use fake stores.
- `ea6971209636`: startup, config, provider, OAuth and host credential service guards. Child reports focused tests and startup-package compilation passed. Disabled mode may read config YAML bytes (including legacy inline fields) but blocks guarded credential use/migration; no real config was used for verification.
- `fe9f68faa20b`: policy-only GetCredentialPolicy RPC, generated bindings, single-transport attestation and explicit reconnect reattestation. Child reports full agentclient tests and narrow handler tests passed against synthetic servers. This is not the later per-session no-respawn contract.
- `7cd8ece11cb9`: local broker admission/work/attachment/destination/retirement accounting. Child reports package and race tests passed. Parent ran `go test -race ./internal/broker ./internal/taskrun -count=1` successfully. The helper is not server integration.

A read-only review identified a copied/stale Closure handle that could remove a reopened session, plus the need for a host lifecycle ticket through post-relinquishment OS unlock/cleanup. Commit `6ce057a6c0de` reproduced and fixed the handle issue, added lifecycle-ticket coverage and passed broker race tests. These tests do not yet establish correct integrated retirement.

Commit `f06464eb15da` adds lock/epoch ownership and transaction-local row guards across the conversation package, with synthetic package/race/vet verification reported by the child. The parent ran `go test -race ./internal/conversation ./internal/broker ./internal/taskrun ./internal/modelhost ./pkg/agentclient -count=1` successfully after the initial foundations. Ownership review then identified missing relationship UPDATE guards and unauthorized-target zero-row success; focused reproduction/fixes are pending. Normal application coexistence remains disabled.

Commit `5caf9c444aab` adds explicit model-host protection options, server-side mutation refusal, reuse-only protected Start, no protected Ensure replacement/launch, and caller-only hold release. The child reports full modelhost and race tests passed and a no-hold Stop bug was reproduced before fixing. Startup must still pass these options and refuse in-process fallback; this is not native shared-backend acceptance. Task terminal failure resolution remains conservative: unresolved outcomes prevent readiness; Close is not a readiness test. Native application startup and credential inheritance instrumentation remain pending.

## Work in progress

Credential isolation, prepared-store task startup/stale completion handling, and store ownership enforcement have separate delegated file ownership. No application coexistence or crash recovery is enabled by the initial compatibility commit. No real configuration, credentials, databases, current agent/runtime, installation, push or merge is authorized.
