# Restricted SUBPROCESS acceptance — toolchain resolved; partial execution evidence

2026-10-26. Baseline: branch `feat/dev-host-generations`, commit
`9e646825537a20fc947696bba501db273c37dd4f`. Retained worktree:
`/Users/bryancostanich/git_repos/cercano-ai/Cercano-dev-host-generations`.
The initial status contained an existing modification to `efforts/dev-host-generations/plan.md`; it was not edited or staged by this attempt.

## Inspection (not execution evidence)

Read `AGENTS.md`, prior native A/B evidence and execution notes, and current
fixture/worker source. The previous native evidence explicitly uses seeded
completed/notified task records, not independently running tasks.

- `cmd/cercano/dev_host.go:217-220` constructs the policy-pinned worker runner
  with nil provider/child callbacks; `:226-228` constructs an agent with nil
  provider/router. The execution-stream interceptor at `:245-247` still refuses
  execution. Opening that interceptor alone is not acceptance.
- `internal/worker/worker.go:70-89` offers provider and tools factories for tests,
  but `cmd/cercano/main.go:2126` registers `worker.New()`, not the injected server.
  Existing injection is not automatically present in the launched CLI worker.
- `internal/worker/worker.go:514-544` honors an injected tool factory, otherwise
  builds the full capability stack. Registration alone is not reachability:
  `internal/agent/toolloop.go:930-954` independently enforces the active profile
  before dispatch/confirmation. No unrestricted-tool fallback was introduced.
- `internal/worker/spawn.go:108-151` resolves the spawn policy and starts a real
  child, whose lifecycle is intentionally not bound to the initiating turn's
  context. This path was inspected, not exercised in this attempt.

## Actual command and result

Attempted from `source/server` via the granted RunCommand tool:

```
go test ./cmd/cercano -run '^TestDevHost' -count=1 -v
```

The tool returned:

```
run_command: fork/exec /opt/homebrew/bin/go: no such file or directory
```

This is a tool/executable launch failure, not a Go test failure or a reproduced
application bug. Compilation and tests did not start. No alternative toolchain
was installed or searched for outside the authorized worktree.

## Outcome and limitations

No implementation or test code changed; execution gates remain closed. No
fixture host, worker, provider, credential store, or model runtime was started
by this attempt. No foreground A/B execution, live sibling task survival,
busy-operation refusal, generation mismatch rejection, or exactly-once durable
completion was verified. No acceptance is claimed, and no modelhost-hold
acceptance was changed. The prerequisite instruction to verify before enabling
fixture execution gates could not be satisfied in this environment. The only
checkpoint from this attempt is this evidence file; there are no newly tested
implementation files.


## Continuation: toolchain resolved (2026-10-26)

The executable-launch failure above is historical, **not a current blocker**.
Plain `go` with inherited PATH and merged `GOPROXY=off`, `GOSUMDB=off`,
`CERCANO_DISABLE_CREDENTIALS=1` now runs successfully.

A test-executable-only worker entry now injects a deterministic provider and
synthetic tool registry. The production CLI cannot select this entry. The real
spawn-policy path launches copied executables from temporary A and B generation
directories, in sequence, with temporary HOME/config/tmp and Unix sockets. No
production provider or native credential store is constructed by the fixture.
The allowed read tool executes; the provider also deliberately requests the
registered forbidden `run_command` sentinel, whose Execute panics if reached.
Passing requires the actual `blocked by plan profile` error event, not merely
a filtered advertised catalog or registration.

### Reproduced failure and fix

Before the profile transport fix, the subprocess test failed (exit 1):
`worker: panic in RunTurn: profile fence bypass: synthetic sentinel executed`.
The host's configured PlanProfile did not reach the initial runner profile:
worker deps carried only CurrentProfile (used by live profile-control gates),
not Profiles (used for the initial capability fence). Minimal host/worker
plumbing now transports the initial profile through private worker gRPC
metadata and installs it in runner deps; malformed/duplicate metadata fails
closed. Existing live profile-control RPCs remain unchanged.

Spawn policy also records binary/manifest content digests at construction and
rejects changed content before a later spawn. The generation replacement
regression covers both members. A baseline-policy regression probe (`git show
HEAD:source/server/internal/worker/policy.go`, restored in `finally`) failed
for both members with `changed generation accepted`, exit 1. This probe was
repeated after the fix; it is not claimed as a new before-edit run.
This is not an atomic defense against a
concurrent filesystem attacker, nor proof of durable ownership epochs.

### Actual final commands/results

From `source/server`, with the environment above:

```
go test ./internal/worker ./cmd/cercano -run 'TestDevHost|TestPinned|TestOwnedCleanup' -count=1
# PASS: worker 1.694s; cmd/cercano 8.726s
go test -race ./internal/worker -run 'TestDevHostWorkerGenerationReplacementRejected|TestPinnedFixtureSubprocessGenerations' -count=1
# PASS: worker 3.223s
```

The race test copies its race-instrumented test executable into both worker
generations. Cleanup kills only fixture-owned workers. The existing plan.md
change is not included. Modelhost acceptance was not modified.

### Explicit remaining gaps

This is **partial**, not completed restricted dev-host subprocess acceptance.
The A/B foreground turns above run through real pinned workers but not through
the compiled dev-host execution RPC (which remains closed). No live sibling
task survival across B startup/other-conversation handover, busy lifecycle
refusals tied to that task, or exactly-once durable completion is established
by these new tests. Existing compiled dev-host lifecycle tests still use seeded
completed tasks; their passing results must not be substituted for live-task
acceptance. No claim of those missing behaviors is made.
