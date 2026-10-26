# Partial admission adapter — not normal-serving enablement

This checkpoint is deliberately smaller than full foreground/task integration.
`BeginRetainedWork` requires the exact retained broker owner, serializes capability
lookup against relinquishment, and returns an ownership context plus live ticket.
The persistence outcome callback marks that ticket unresolved. Descendant bounded
writes preserve both capability and outcome; callers still own ticket lifetime.
`PersistTurn` now reports marshal/append failures through that outcome in addition
to logging. Legacy callers without an outcome retain their existing behavior.

Lifecycle-only servers explicitly refuse unary (including fallback), legacy
stream, broker turn, task completion wakes, task subscriptions, and conversation
attachments before the relevant mutations. These refusals are NOT admitted normal
execution. The dummy lifecycle server must still never be registered as a normal
handler. There is no full-server constructor, startup or proto enablement here.

## Concrete inventory and remaining prerequisites

- `server.go`: `NewServer`/`newServerWithLogs` wire the normal agent/store. Lazy
  `ensureTaskManager` still uses `taskrun.New` (initializes/reconciles). Full-server
  construction must inject the prepared store and `NewPreparedOwned` before any
  Launch. B construction preserving A tasks is not covered by this checkpoint.
- Unary and legacy-stream fallback call `persistDevMode` then `agent.Process`.
  Agent user/assistant append failures are log-only; success and scheduling can
  follow failure. They need propagated outcomes before enabling these paths.
- Broker/continuation turns must retain one accepted-work lifetime across broker
  supersession/drain, continuation loops, terminal publication and writeback.
  No exact-owner RPC bridge is installed here.
- `taskrun.NewPreparedOwned` accepts `notify func(string)`; the server's
  `notifyTaskChange(parent string)` coalesces wakes onto a host background context.
  This API loses the originating owner epoch and ticket. ID-only re-resolution
  would permit stale completion to act on a new owner. A concrete bridge must
  preserve exact identity through callback, queueing and `tryTaskCompletionWake`.
  Existing `SetBackgroundWorkAdmission` has no parent/owner argument.
- `SubscribeTasks` has parent/cursors but no established terminal attachment
  identity. A pane is not necessarily a distinct terminal. Do not mint one
  attachment per observer; the coexistence path refuses pending identity design.
- `PersistTurn` outcome is not yet checked at every runner/tool callback success
  boundary. `runner.Core` and tool history use void persistence callbacks. Other
  log-only writes (context usage, autonomy/run state, agent appends) remain to be
  converted; merely marking one PersistTurn blocker is not full write coverage.
- `workerPostTurn`, context usage recording, cloud evidence writes and runner
  cleanup use detached/background contexts. The bounded helper is available but
  those call sites still need scoped ownership and tickets covering completion.
- Recap/compaction scheduling uses conversation IDs, not owner context. Agent
  `ScheduleRecap`/`ScheduleCompaction`, persistence auto-compaction (`CompactNow`
  with background context), retention sweeper and their callbacks require tickets
  from scheduling through durable completion. These are NOT enabled here.
- Full-server cfgSvc must be frozen at construction. UpdateConfig, watcher start
  and reload must reject before any side effect; changed setter errors must be
  propagated. This checkpoint does not implement configuration freezing/guards.
- Other administrative mutation RPCs are not covered by the targeted refusals.
  Hence neither full coverage nor safe ordinary handler registration is claimed.

Tests added use temporary owned stores and synthetic failures only. Focused
server/persistence tests (including existing lifecycle/acquisition tests) pass
with `-race`. An initial compile blocker in concurrent agentclient edits cleared
on rerun; no agentclient files were changed by this checkpoint. No live provider,
native application, user config, credentials or database was used.
