# Task-only tab close regression: existing dismissal, no schema change

## Historical trace

At `4b1d66d2c287c2d129b581a9038e15066b479906`, `chat_tabs.go` removed a tab locally before `dismissSubAgentTab` launched a fire-and-forget RPC. The comment explicitly warned that failed persistence could make the tab return after restart. Server dismissal updated `conversations.dismissed`; a missing row was a zero-row update in the legacy store. Pinned dismissal also used the child ID against parent-bound terminal metadata. Thus local closing could appear successful without durable dismissal.

The acknowledged-close changes stopped concealing those errors. Their ancestor check rejected a legitimate task whose routing failed before dispatch created the child conversation. This explains the reproduced `PermissionDenied: unrelated child`; it does not identify the user's exact live task, which was not inspected.

## Correction

For a missing child row, validate the durable task's exact pinned parent and require a terminal task. Admit the requesting terminal normally. Reuse `beginPreparedDispatch` to acquire a previously unused child identity, create the ordinary empty subagent conversation metadata, and release the temporary child scope. Then use the existing ancestor-authorized dismissal/reopen path.

No transcript turns or task results are invented or changed. No new table, column, contract version, migration, task JSON field, RPC, or ownership-fence exception is introduced. `TaskSnapshotPage` and `ListChildren` already consult the existing dismissal flag. The speculative schema-3 branch is not included.

A still-starting task is not repaired: it returns an explicit retry error. A child identity with existing ownership is not adopted, even if released; interrupted-execution recovery is outside this correction. Existing conversation-backed tabs retain their current behavior.

## Verification performed directly

- New regression initially failed with `PermissionDenied: unrelated child`.
- Strengthened regression uses real task manager + dispatch engine with no configured route, then real gRPC close/reopen. Bypassing only the new repair path reproduces that exact failure.
- Corrected test verifies close/reopen/close visibility, a fresh SQLite connection, unchanged task record, and zero transcript turns.
- Negative tests reject stale epoch, conflicting terminal, active task and already-owned child without creating metadata. Existing parent-child visibility tests also reject foreign targets and revoked terminals.
- Focused server/conversation race selection passed three repetitions; full CLI suite passed.
- Full `go test -race ./internal/server ./internal/conversation ./internal/taskrun -count=1` passed after restoring the correction from the baseline probe.
- Tests use disposable state with credentials disabled and dependency fetching disabled; no real inference, live configuration/database writes, or process restart.

This is source/integration verification, not a live terminal deployment or proof of crash recovery. The existing schema declaration remains unchanged.
