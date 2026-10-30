# Shutdown join follow-up — partial, not approval to activate

This change does **not** complete the five requested review blockers. The canonical activation gate is unchanged. Both shutdown-decision.md and shutdown-caller-obligations.md were read; their remaining obligations are not waived.

## Implemented

- Server task shutdown retains one shared completion channel across concurrent/repeated waits. Task execution and completion callbacks are actually joined before Shutdown can dispose runtime/store dependencies. The previous 30-second log-and-continue path is removed.
- RetireCoexistence propagates its task-drain wait error rather than proceeding to CloseParticipation/store close on deadline. The underlying join continues automatically after that waiting caller returns.
- Task managers have an atomic launch fence so obtaining the join does not wait for the manager's persistence lock merely to fence admission.
- Production producer disposal waits for actual Close completion instead of consuming its Once after a two-second deadline. This does not alter compaction implementation.

## Reproduction and checks

Before fixing the server join, TestTaskShutdownConcurrentJoinInsensitiveExecutor failed: a second close returned while an insensitive fake executor remained behind a barrier. The test now waits for both callers after releasing that barrier. A second test checks bounded completion-wait failure, retained completion join, eventual completion, and independent sibling state.

An initial race run exposed a test-only race caused by reassigning the release channel; the fixture now releases via sync.Once. The final run passed three repetitions with credentials disabled and module network access off:

```
go test -race ./internal/server ./internal/taskrun ./cmd/cercano -run 'TestTaskShutdown|TestServerTaskManager_|TestProductionIncompleteShutdownControl|TestShutdownInterruptsAndRejectsNewWork' -count=3 -timeout=120s
```

Compile-only `go test ./... -run '^$'` passed in both source/server and source/clients/cli under the same environment. These compile tests include existing mocks but are not full-suite execution. No native/provider operations or live activation were requested by these tests.

## Still unresolved / unverified

- The synchronous production fence still does not promptly fence all background producers/queued completion work while an initial rebuild is blocked. The new task-manager fence is used at actual task close, not yet at production shutdown request.
- Remote non-secret shutdown status protocol, client propagation and generated bindings/mocks have not been added. Accepted remains distinct from completion by contract, but caller propagation is still outstanding.
- CloseParticipation's discarded cleanup errors and development-host bounded handles/process retention remain outstanding. RetireCoexistence is only partially corrected here; no complete retirement/readiness qualification is claimed.
- The producer timeout failure was inspected, not reproduced with a new fake barrier before its caller change. Each of the five requested findings has NOT been independently reproduced.
- The task-close context bounds joining after synchronization; acquiring taskRunMu is not itself context-bounded. Full production bounded admission must use a nonblocking fence instead of moving this method into the synchronous stop callback.
- Server.Shutdown as a whole is not made exactly-once by this task join; production's outer handle remains responsible for exactly-once disposal.

No changes to compaction implementation, plan status, canonical activation, caller-contract approvals or external user state are included in this unit.
