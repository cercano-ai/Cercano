# Nested host dispatch — bounded solved unit (2026-10-29)

Baseline: `3c29066b653b1a1104f9f06b6588b2114d49bb62`. This completes host-side scope composition, not Phase 2 or worker nesting.

## Reproduction and mechanism

The minimal parent → child → grandchild regression fails against baseline `prepared_worker.go`:

```
go test ./internal/server -run '^TestPreparedNestedDispatchThreeLevels$' -count=1
prepared_nested_test.go:29: grandchild acquisition: nested or invalid prepared dispatch
```

The inherited dispatch-key guard refuses the grandchild before acquisition. The baseline refusal was repeated by temporarily substituting the exact HEAD version of this one source file, running the fake regression, and restoring the implementation in a `finally` block. The implementation then passes the same regression.

## Change

Reuse the existing prepared scope, exact ownership and retained persistence work tickets. Each scope records its exact owner. Nested admission checks that the supplied immediate-parent authority matches that scope, and reserves a completion join under the same mutex used to fence closure and producer registration. Acquisition and waiting occur outside that mutex and outside the lifecycle mutex. Failed admission resolves its join reservation; uncertain acquisition retains the existing work blocker. Successful admission resolves its reservation only after descendant scopes/producers join and existing final persistence, ownership release and work-ticket completion have run. Descendant failures propagate upward and retain blockers. No depth policy, ordinary dispatch wiring, schema, worker protocol or credential behavior changes.

## Verified fake-only tests

All commands ran from `source/server` with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
go test ./internal/server -run '^TestPrepared' -count=1
go test -race ./internal/server -run '^TestPrepared' -count=1
```

Both passed; the final race run passed after all source/test changes. New regressions cover three levels, persisted child/grandchild relationships, grandchild transcript and evidence ownership, wrong-parent/scope rejection, bottom-up completion with a delayed producer callback, fencing late admission, independent sibling progress, descendant write and durable-release faults, retained root blockers, and registration racing scope closure. Existing prepared scheduling/worker tests remain in the passing selection. The delayed producer test registers the existing wait contract directly; this is not a compiled-worker or provider end-to-end test. No native credentials, external inference, live state, installs, activation, merge or push were used.

## Remaining worker integration boundary

Worker descendant guards deliberately remain in place. `ConfigurePreparedWorker` still refuses Ensure descendants; `preparedWorkerAuthority.Context` still authorizes only the original conversation. `workerRunner.ensureDispatchChild` validates stream ancestry, but that map is not ownership authority. Worker subagent transcript writes request `TurnAuthority.Context`; dispatch events and Ensure callbacks currently receive the turn context instead of a separately resolved child context.

The concrete unresolved implementation contract is how one worker-turn authority retains each admitted descendant's exact context/completion callback, supplies the immediate-parent context to Ensure and the target child context to evidence writes, then fences and finalizes those scopes bottom-up after worker producers and evidence writes are drained. The existing Close callback supplies a turn error, not per-descendant completion proof; error paths must remain conservative. This unit does not infer completion from ancestry, a terminal event, or an error, and does not introduce a new authority or protocol to bridge that gap. Worker nesting and compiled production acceptance remain incomplete; the plan item stays partial.
