# Phase 1 bounded audit resumption — 2026-10-26

Source inspection only. No implementation, tests, native probes, user-data access, or process manipulation. The user requested continuation after the dispatch-capacity pause. Three bounded tasks completed; their scope does not establish exhaustive coverage.

## Conversation store

Task `task_387d24404bf75d290dcdda2161c37094` completed eight inspection calls in `source/server/internal/conversation`.

Owner-controlled candidate mutations include conversation/subagent creation, rollover, Append, Rename, SetGeneratedTitle, MarkSubagentDismissed, UpdateRecap, SetDevWorkDir, compaction/context usage, SetChatRoute, AppendDispatchEvent, task insertion/start/finish/reconciliation, wake claims/acknowledgements, autonomous runs, Delete/DeleteTurns/PruneRawBodies/CollapseConversation. See store.go:515–685,861–986,1104–1191; chat_route.go:24–41; dispatch_event.go:54–88; tasks.go:132–203; autonomy.go:126–189.

Transactions exist for Append, CreateRolledOver and CollapseConversation, but many other operations use direct ExecContext. There is no inspected common mutation wrapper. Owner/epoch checks must cover both forms atomically; fencing only Append is insufficient. Startup migrations and task reconciliation must remain behind compatibility/recovery admission. Task records have no inspected foreign-key cascade from their parent; deletion semantics require explicit review rather than assuming all state disappears.

## Callers, admission and dependent effects

Task `task_822ee7f59daa99b22a7b53daa95b14d4` completed ten inspection calls.

- `internal/hostsvc/persistence/persistence.go:512–540`: PersistTurn returns no error and logs Append failures. This body was additionally read directly in the parent turn. Existing behavior cannot propagate stale-write rejection to the turn loop. Caller/control-flow changes and unresolved-persistence accounting are required before claiming safe readiness.
- `internal/server/task_completion.go:70–141`: background policy admission precedes broker admission and wake claims; the turn lease covers notification acknowledgement. Claims and acknowledgements are separate writes. Response completion is not itself proof of persistence. Claim errors skip tasks; acknowledgement errors are logged.
- `internal/taskrun/manager.go:65–156`: task acceptance and terminal persistence happen under manager mutex; notification runs after unlock. Persistence errors suppress notification and remain available to Wait. The affected-row boolean from FinishTask still needs explicit tracing; nil error must not imply a successful fenced write.
- `internal/taskmodel/forest.go:124–259`: mutate, persist, then emit; rollback of in-memory changes suppresses callbacks on persistence failure. Actual plan writer and owning-store locks remain untraced.
- `internal/research/sidecar.go:46–58`: direct research_state.json overwrite, without a transaction/admission seam. The separate web sidecar and callers remain untraced. These tool-produced files require admission/quiescence classification; they must not casually be described as SQL-fenced writes.

## Shared configuration and runtime

Task `task_0b5b3057c03de7a7d2ab6a554a57142c` returned a partial inventory. Some findings were explicitly carried from its supplied history and searches were truncated; do not treat it as exhaustive fresh verification.

- Config service Persist and direct Save callers coexist with live-memory mutations. A Save-only guard cannot prevent preceding runtime/live-config effects. UpdateConfig, watcher replay and direct startup/setup writes all need coverage.
- `internal/modelhost/server.go:582–850`: dispatch directly exposes download/ensure/cancel/delete/enroll operations; restart affects all holders. Stop is caller-hold release, and invalidated acquisition cleanup stops only an unheld instance. Guards must preserve legitimate release/cleanup instead of blanket-blocking every Stop.
- `internal/modelhost/ensure.go:171–240`: older-host replacement can request shutdown; reconnect also uses Ensure. Refusal must precede shutdown/replacement and fallback, not merely reject the resulting connection.
- Session-local main-chat routing must remain distinct from frozen shared configuration and history persistence.

## Remaining gates

The ownership inventory remains incomplete: indirect compaction callbacks, all persistence callers, full admission/attachment ordering, plan writer, both research paths and other file effects need bounded follow-up. Shared-config callbacks, complete UpdateConfig sequencing, shutdown/idle/release behavior, fallback and direct tool exposure also remain incomplete.

The prior native identity audit found no existing process-birth/death evidence primitive sufficient for the approved recovery contract in modelhost, worker and enterprise. PID, endpoint loss, timestamps and released flock alone are insufficient. No claim is made that a safe macOS mechanism is impossible. A concrete process-instance evidence design and uncertainty contract require review before recovery implementation; no mechanism has been selected or implemented.

The plan's foundational stop remains in force. Recommendation for human review: authorize a bounded source/API feasibility investigation of a macOS process-instance identity mechanism, preserving fail-closed recovery and prohibiting live process/user-data access. This is not permission to weaken the contract or begin native application acceptance.
