# Separate loaded-instance HOLD acceptance — 2026-10-26

## Baseline and ownership

Inspected git first: branch `feat/dev-host-generations`, HEAD
`9e646825537a20fc947696bba501db273c37dd4f` (Integrate prepared durable dev-host A/B lifecycle harness).
Only pre-existing dirty path was `efforts/dev-host-generations/plan.md`; left untouched.
Read root AGENTS.md (no nested AGENTS under source/efforts), modelhost protection
and engine-hold tests, and `native-ab-integration-evidence.md`.
This unit owns only the new `internal/modelhost/loaded_hold_acceptance_test.go`
and this note. No command, worker, production code or shared plan edits.

Existing protection coverage checks protected reuse/remote close, but does not
combine post-A synthetic engine usability, explicit manager-boundary Stop
counting, and final cleanup in one protected A/B test. Existing engine-hold tests
exercise synthetic completions with an unprotected host. Native compiled evidence
explicitly has no registered provider and proves attachments, not loaded holds.

## Precomputed oracle and implemented check

Expected counts were stated before writing/running the test:

| Stage | Holds on original instance | Manager Stop calls |
|---|---:|---:|
| Instance seeded before server/clients | 0 | 0 |
| Connected A and B each reuse/own | 2 | 0 |
| B synthetic completion | 2 | 0 |
| A closes (or own Stop then closes) | 1 | 0 |
| B synthetic completion after A closes | 1 | 0 |
| B final own Stop | 0 | 1 |
| B closes | 0 | 1 |

`TestProtectedLoadedInstanceABHoldAcceptance` covers both A-close variants.
Each subtest owns a temporary registry, protected actual modelhost Server,
loopback gRPC listener, two protected actual Remote clients, InMemoryManager,
FakeProvider, and loopback HTTP canned-completion endpoint. Manager Stop is
counted with an atomic wrapper (not inferred only from provider calls).
A/B Start must reuse the original seeded ID; provider Starts must stay exactly
one throughout. Assertions inspect actual synchronized server hold ownership
and stream attachment counts. After A detaches, B remains attached and owns the
sole hold, the host is not draining, and the same cached engine completes again
through the synthetic endpoint. Exactly two HTTP completions are observed.
B sees the original non-stopped instance and endpoint. Final B Stop must cause
exactly one manager and provider Stop and the original instance must become
stopped; subsequent B close does not add a Stop.

## Exact commands and concrete results

From repository root:

```
git status --short --branch
git log -1 --format='%H %s'
gofmt -w source/server/internal/modelhost/loaded_hold_acceptance_test.go
```

From `source/server` (offline module resolution explicitly enforced):

```
GOPROXY=off GOSUMDB=off go test ./internal/modelhost -run '^TestProtectedLoadedInstanceABHoldAcceptance$' -count=1 -v
# PASS, package 0.475s (initial test before adding explicit final stopped-state assertion)
GOPROXY=off GOSUMDB=off go test -race ./internal/modelhost -run '^TestProtectedLoadedInstanceABHoldAcceptance$' -count=20 -v
# PASS, package 1.778s, both variants on all 20 repetitions, final test
GOPROXY=off GOSUMDB=off go test -race ./internal/modelhost -count=1
# PASS, package 6.982s, final test and existing package coverage
```

No failures observed; no negative mutation/regression probe was run. Race
instrumentation covers the in-process fixture, server, clients and manager;
there are no separately compiled children in this acceptance check.

## Scope and limitations

This proves shared loaded-instance hold accounting independently of synthetic
worker/subprocess acceptance. “Loaded” here means the synthetic provider's
running instance recorded in the manager, NOT weights loaded by an actual model
runtime. HTTP replies are canned; no real model inference qualification is
claimed. A participation retirement is exercised as Remote Stop/Close, not a
compiled dev-host retirement RPC, retained-session transfer, or live task.
The native compiled fixture's loaded-runtime gap remains for that journey.
No downloads, real runtimes/models, credentials, external network, install,
existing live-process changes, push or merge were used. Listeners are isolated
loopback resources created and closed by tests. No whole-repository validation.

Worktree retained at
`/Users/bryancostanich/git_repos/cercano-ai/Cercano-dev-host-generations` on
`feat/dev-host-generations`. Checkpoint stages explicitly only the two owned
files; commit SHA is reported with the handoff rather than self-embedded here.
