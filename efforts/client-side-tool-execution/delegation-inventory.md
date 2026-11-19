# Delegation inventory — partial, directly inspected

Status: Phase 1 remains incomplete. No implementation or tests. Paths below are relative to this worktree. These findings do not certify the full routing inventory.

## Confirmed call chain

- `source/server/internal/capabilities/builtins/dispatch_cap.go:dispatchCap.Execute` requires host-supplied `Call.ConversationID`; a model-supplied conversation ID is only a consistency check. It chooses `cwd`, then `path`, then `Call.WorkDir`. Nested grants are narrowed against `agenttools.DelegationFrom(ctx)` before `call.Svc.Tasks.Launch`. The launch spec carries directory and parent conversation, but no executor identity. Binding must derive from the parent, never from these directory arguments.
- `source/server/internal/worker/tasks.go:streamTasks.Launch` sends a `TaskControlRequest` with task, tools, directory, routing task, tier and read-only status through `p.call`. The host receiver remains to be traced. The protocol boundary must carry or authoritatively recover the inherited execution binding, not infer it from `WorkDir`.
- `source/server/internal/server/tasks.go:serverTasks.Launch` forwards to the startup-owned task manager; absence is an error, not synchronous fallback.
- `source/server/internal/taskrun/manager.go:Manager.Launch` creates a durable `TaskRecord` containing parent, child and directory. Optional admission validates conversation ownership using conversation/host/epoch. It inserts before acknowledging, then preserves context values with `context.WithoutCancel`, supplies manager-owned cancellation, copies tool grants, assigns child conversation ID, clears the launching turn's progress callback and invokes `m.run` asynchronously. These host ownership fields are not client executor authentication.
- `Manager.run` invokes `m.execute(ctx, spec)` and persists the original result. Explicit stop/completion arbitration is serialized under the manager mutex; host shutdown records interrupted status and a no-replay warning. Binding must survive detached execution and persistence, not live only in a turn callback.
- `source/server/internal/worker/worker_dispatch.go` installs the service runner through `svc.SetEngine(engine)` and supplies `runner.BuildSystemPrompt` with `workerCtxHistory`. This is host-side orchestration, not a unit to move wholesale to clients.
- `source/server/internal/hostsvc/tools/tools.go:Service.SetEngine` installs `RunAgenticDispatch`. That runner builds a registry-scoped delegation context, then invokes `agent.RunToolLoop` with `spec.WorkDir`, child conversation ID, granted tools as `PreauthorizedTools`, and a nil permission requester. Forced confirmation still denies execution according to the inline contract. Client execution authorization must remain distinct from tool-grant preauthorization.

## Additional receiver and composition evidence

A bounded six-call delegated trace (`task_a4fa064b6d1ab1f5506287b2fd3e3388`) reports: `worker/host.go:1082–1087` passes the active turn's `req.ConversationID` to `handleTaskControl`, rather than accepting a payload-selected parent. `worker/tasks.go:handleTaskControl` validates parent/service availability, routing class and read-only grants, then reconstructs the launch spec. `streamTasks.call` rejects a parent unequal to its bound parent. Upstream worker-stream authentication was not examined by this trace; this is not proof of remote executor authentication.

Direct follow-up inspection closes the executor injection gap: `internal/server/server.go:Server.ensureTaskManager` obtains `s.toolSvc.Engine()` and passes `engine.Dispatch` to both ordinary and prepared-owned task manager constructors (lines 662–671). Searches locate task service injection through `control.SetTasks(serverTasks{s})` at line 1635 and prepared worker setup at `internal/server/prepared_worker.go:108`. `internal/dispatch/engine.go:Engine.Dispatch` invokes `e.agenticRunner(ctx, spec, sel, model)` at line 278; its separate one-shot path calls `e.ctxLoader.PrependContext(spec.WorkDir, prompt)` at line 302. Thus routing only explicit child tool calls would miss one-shot context loading.

## Required follow-up evidence

Trace `streamTasks.call` and host task-control receiver, task manager executor wiring, `dispatch.Spec` and conversation/task persistence schemas, registry construction in `RunAgenticDispatch`, and tool-loop context propagation through adapters. Enumerate automatic reads and their callers separately. Existing `nested_dispatch_integration_test.go`, `dispatch_cap_test.go`, and task lifecycle tests were located but have not been validated for the new contract or executed.

## Audit delivery limitation

Delegated tasks `task_905f6f074489a3bef4ad116c52891015` and `task_9ab567e6111bda3d919a058301443583` both failed on dispatch token-volume limits before producing usable inventories. Their inspection activity is not treated as completed evidence. This document records only subsequent direct inspection.
