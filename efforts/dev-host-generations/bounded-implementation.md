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

## Integrated synthetic lifecycle checks

- `a08a8f628efb`: reproduced and fixed relationship UPDATE and zero-row target-authorization gaps; targeted task/run IDs resolve their stored conversation inside the same transaction. Child reports conversation race tests passed.
- `3ab75f9a358a`: NewPreparedOwned integrates broker admission and scoped ownership through task acceptance, execution, bounded terminal persistence and notification; failed outcomes retain blockers. Child reports taskrun tests/race passed.
- `0da52b39c725`: lifecycle-only server methods implement retain/list/attach/release, preaccepted exact-destination reservation/claim and explicit retirement with safe participation hook. Synthetic A/B/C tests passed; this constructor must not serve ordinary RPC requests yet.
- `954e24e12b6c`: immutable hostsvc config freeze guards live mutation and persistence; actual server/startup callers still require integration.
- `a6d2e3aa7635`: isolated generation publisher and eight fixture tests. Neither installed launcher nor native application was run.

Parent verification passed:

```
go test -race ./internal/conversation ./internal/taskrun ./internal/server -run 'Test(Ownership|Coexistence|Prepared|RejectedTerminal|Owned)' -count=1
```

These checks are synthetic integration evidence, not native multi-process or manual terminal acceptance. The exact source-owned lock is expected during normal handover. Crash recovery remains unimplemented, as requested until the normal workflow is verified.

## Concrete admission follow-up

Parent source read of `Server.InvokeTool` (server.go around 2407 in this working tree) found direct registry execution with no conversation admission or request conversation field. Before startup enablement, pinned binding metadata must either provide a checked conversation ticket for this path or the path must explicitly refuse in coexistence mode. Store fencing alone cannot stop an external tool side effect. `GetContextUsage` also delegates to persistence and may require scoped cache-write context. This is an implementation coverage item, not a reason for more process-identity research.

## Prepared server and reconnect integration

- `b2324fc7c4cd`: pinned no-launch client transport and additive read-only identity/session RPCs. Initial handler integration skipped for a missing admission accessor; this was not accepted as completed verification.
- `053a9ed6d8e3`: added concrete read-only CheckRetained accessor, removed dynamic fallback and test skip. Real synthetic handler/client integration now passes under race testing.
- `cf7369f90c47`: explicit worker SpawnPolicy pins generation binary and scopes resources; zero-process tests passed. Startup must select it and skip global reaping; ambiguous orphan cleanup remains unsupported.
- `721188eab3d5`: scoped persistence failure reporting and fail-closed normal-work guards; explicitly not foreground execution enablement.
- `e7f9cc2cb639`: NewPreparedServer constructs real dependencies with the same preflighted store and frozen config; owned manager initialization avoids broad reconciliation. Synthetic two-full-server tests establish B manager startup leaves A accepted/running rows unchanged. Foreground/completion enablement remains separate.
- `f653749ac4c4`: NewPreparedOwnedCompletion carries exact owner/context through callback and preserves callback-failure blockers. A resolver-under-manager-lock deadlock was reproduced and corrected before its race tests passed.

Additional parent checks passed:

```
go test -race ./internal/server ./internal/taskrun -run 'Test(Prepared|Owned|CoexistenceIdentityPinnedIntegration)' -count=1
python3 source/server/scripts/publish-dev-generation_test.py
# credential opener trap tests only, fake enabled stores
go test -race ./internal/secrets ./internal/enterprise -run '^(TestCredentialGateRejectsBeforeBackend|TestCredentialGateEnabledFakeBackend|TestEnterpriseCredentialGateRejectsBeforeBackend|TestEnterpriseCredentialGateEnabledFakeBackend)$' -count=1
```

## Work in progress

Active delegated slices at this checkpoint:
- `task_75e886c072424fc2f4da64e557ef38bd`: terminal identity/refcount and strict pinned metadata helper; retry after task_f993dfc2fe65600acd5aec4290f34fbf failed with tool errors (no completed result).
- `task_8f02be3d7afc556c338c441e380fa90d`: exact-owner queued task-completion integration in the real prepared server.

Next dependent work: foreground/stream admission using the terminal helper, remaining administrative and background scheduler coverage, actual lifecycle RPC controls and CLI UI, explicit isolated startup/runtime/worker wiring, then native normal-workflow qualification. Only afterward implement conservative absence-based recovery. No native application startup/Keychain qualification or complete A/B journey is claimed.


Credential isolation, prepared-store task startup/stale completion handling, and store ownership enforcement have separate delegated file ownership. No application coexistence or crash recovery is enabled by the initial compatibility commit. No real configuration, credentials, databases, current agent/runtime, installation, push or merge is authorized.
