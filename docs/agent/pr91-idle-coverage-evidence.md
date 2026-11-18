# PR91 idle-shutdown coverage evidence (2026-11-17)

## Scope and isolation

Linked worktree: `/Users/bryancostanich/git_repos/cercano-ai/Cercano/.worktrees/pr91-idle-coverage`, branch `test/pr91-idle-coverage`.
Integration baseline `11951c3d16653b25af54d52394898ac74a831cf1` combines remote main
`cf406792ecb487c9492a6bd259aa36b807b7f743` with PR91 head
`458cd3e7f9b8bd48adb545b60c5e5d16114f65c8`. Both remote SHAs were rechecked
using `git ls-remote origin refs/heads/main refs/heads/fix/windows-idle-shutdown refs/pull/91/head`;
the named feature branch was not advertised, but the PR head ref was.
Only the isolated integration branch is checkpointed; no push or merge into main.
Root local main and its untracked evidence were not edited.

Read AGENTS.md, CLAUDE.md, self-dev.md and development-generations.md. Tests
use disposable fixture state, not the ordinary launcher's default user state.
The child inherits only Windows system-directory variables plus disposable
HOME/profile/config/cache/temp paths; credentials are explicitly disabled. No
VM, native credential store, installed launcher, or unrelated process is operated.
The subprocess deadline targets only its own test executable.

## Coverage

- The generation negative assertion retains SIGTERM observation and all retained
  session / independent-work assertions. It also reads the invocation-local
  shutdown control with `ShutdownAgent(StatusOnly: true)`, rejecting accepted,
  completed, or non-running shutdown after the idle grace window.
- `TestProductionDefaultIdleSubprocess` runs the existing complete production
  startup fixture in a bounded child (30s test timeout, 45s parent deadline).
  Its dedicated scenario leaves `idleShutdown` nil: startup must install the
  default `RequestSelfShutdown` callback. No explicit shutdown request occurs
  before waiting for idle disposal.
- A registered-subscriber channel synchronizes reconnect within grace; disposal
  must not happen while that stream remains connected beyond the 2s grace.
  Its last disconnect must finish production disposal. The existing fixture
  then checks stopped serving, owned-resource teardown, closed stores and policy
  restoration, before emitting the required completion marker. The parent
  requires both that marker and successful child exit.
- Timing is used for bounded deadlines and negative observation windows, not
  sleeps to synchronize subscriber registration or disposal completion.

## Old assertion probe and mutation checks

The original signal-only weakness was demonstrated before adopting the stronger
assertion, and reproduced again at final verification. Temporarily use the
integration HEAD version of production_generation_idle_test.go and add this
probe (imports omitted here: context, testing, pkg/proto):

```go
func TestPR91OldSignalOnlyProbe(t *testing.T) {
    srv := startOrdinaryGenerationTest(t)
    srv.RequestSelfShutdown()
    checkStartupLastClientShutdown(t, srv, false)
    state, err := srv.ShutdownAgent(context.Background(), &proto.ShutdownAgentRequest{StatusOnly: true})
    if err != nil || !state.Accepted || !state.Complete {
        t.Fatalf("probe did not dispose: %v %v", state, err)
    }
    t.Logf("old signal-only assertion passed despite shutdown: %v", state)
}
```

From `source/server`:

```sh
go test ./cmd/cercano -run '^TestPR91OldSignalOnlyProbe$' -count=1 -v -timeout=30s
```

**PASS**, despite `accepted:true complete:true phase:"complete"`: the old
negative assertion missed an actual control request and completed disposal.
The temporary test was removed and the strengthened file restored in `finally`.

Two independent temporary mutations in cmd/cercano/main.go, each restored in
`finally`, were then checked against the new coverage:

1. Remove `admission.generation == nil &&` from the idle enablement guard:
   `go test ./cmd/cercano -run '^TestProductionGenerationLastClientDoesNotShutdown$' -count=1 -v -timeout=60s`
   **FAIL as intended**, both retained-session and independent-work cases:
   `last subscribed client requested generation shutdown/disposal: accepted:true complete:true phase:"complete"`.
2. Replace `idleShutdown := srv.RequestSelfShutdown` with
   `idleShutdown := func() {}`:
   `go test ./cmd/cercano -run '^TestProductionDefaultIdleSubprocess$' -count=1 -v -timeout=60s`
   **FAIL as intended**, child reports
   `default idle callback did not finish production disposal`; parent rejects
   unsuccessful child exit.

`git diff --exit-code HEAD -- source/server/cmd/cercano/main.go` passed after
restoration, and the temporary probe is absent. No production changes are part
of this coverage checkpoint.

## Passing verification

Host: Go 1.26.3, darwin/arm64. Exact focused commands from `source/server`:

```sh
go test ./cmd/cercano -run '^(TestProduction(Generation|SingleHost|DefaultIdle|IncompleteShutdown|DeferredFailure|RemoteStatus)|TestOrdinaryGeneration)' -count=1 -timeout=180s
go test ./internal/server -run '^(TestDrainThenStop|TestEventHub_CloseAll|TestServer_(BeginShutdown|EnableIdleShutdown|RequestSelfShutdown|IdleShutdown|ScheduleSelfShutdown)|TestServerShutdownStopsRuntimeInstances)' -count=1 -timeout=120s
go test -race ./cmd/cercano -run '^(TestProduction(Generation|SingleHost|DefaultIdle|IncompleteShutdown|DeferredFailure|RemoteStatus)|TestOrdinaryGeneration)' -count=1 -timeout=180s
go test -race ./internal/server -run '^(TestDrainThenStop|TestEventHub_CloseAll|TestServer_(BeginShutdown|EnableIdleShutdown|RequestSelfShutdown|IdleShutdown|ScheduleSelfShutdown)|TestServerShutdownStopsRuntimeInstances)' -count=1 -timeout=120s
```

All four passed (15.961s, 0.854s, 19.443s, 1.976s package times respectively).
These include production host integration with immediate neighbors and controlled
external dependencies, not only isolated idle-callback unit tests. Neither
package requires an integration build tag for these tests.

Windows cross-compilation passed for both test binaries:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$OUTPUT/cercano.test.exe" ./cmd/cercano
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$OUTPUT/server.test.exe" ./internal/server
```

`OUTPUT` was a Python TemporaryDirectory named `pr91-windows-*`; both binaries
were removed when it closed. `git diff --check` also passed.

## Limitations / native Windows prerequisites

Cross-compilation is not native Windows execution. No VM was operated. Full
repository suite, real-model acceptance, native credentials, installed launcher
and packaged CLI end-to-end tests were not run. This test proves the production
startup/disposal fixture and test-process exit, not the packaged executable's
entire CLI entrypoint. Engines, MCP transport and external services remain the
existing fixture's controlled substitutes.

Separately, an authorized operator needs a native Windows amd64 host, the
supported Go toolchain (and Windows C compiler if running `-race`), an isolated
checkout at this checkpoint, and disposable writable state/temp directories.
Run the focused commands above natively, especially
`TestProductionDefaultIdleSubprocess` and the generation negative test; repeat
with `-race` where supported. Preserve child output, the production-disposal
marker, and successful exit as evidence. Keep real HOME/profile/credential state
out of fixtures, do not stop existing agents, and obtain separate authorization
before packaged-launcher or VM acceptance. Native Windows runtime qualification
remains required before sign-off.

## Post-merge landing verification (2026-11-18)

PR91 merged as `d03c10d36bf369b7b9f975220f0ab1ee2590da20`. Its tree
is identical to integration baseline `11951c3d16653b25af54d52394898ac74a831cf1`
(`git diff --quiet` passed). Replayed only follow-up commit
`e62343275ce31114cd29e9ea85278caa3fb72d9e` onto that current origin/main
in isolated worktree `.worktrees/pr91-idle-land`, branch
`test/pr91-idle-followup-main`; no integration merge or local-main commits
were carried over. The retained original coverage worktree was clean.

Re-ran all four exact focused commands under Passing verification above on
Go 1.26.3 darwin/arm64: all PASS (15.925s, 0.917s, 19.533s, 2.035s
respectively). Re-ran both Windows amd64 cross-compilations with CGO disabled:
both PASS; temporary output binaries removed. `git diff --cached --check`
passed after replay. Earlier mutation/probe results above are retained evidence,
not re-executed during landing. No VM actions, native Windows execution,
full-suite run, installed launcher actions, or packages-repository changes.
