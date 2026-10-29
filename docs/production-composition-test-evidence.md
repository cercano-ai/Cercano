# Shared service registration evidence (2026-10-27)

Scope: test-only repair in `source/server/cmd/cercano/production_composition_test.go`.
No production credential boundary changes or credential enabling.

## Probe before repair

With `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, from
`source/server`:

```
go test ./cmd/cercano -run '^TestProductionCompositionServiceParity$' -count=1
```

Failed at the former line 158: `open read-only credentials: credentials disabled
by CERCANO_DISABLE_CREDENTIALS`. The production opener correctly refuses before
calling the injected credential factory; the former test's successful-opener
assumption was stale.

## Replacement evidence

- `TestProductionCompositionSharedServiceRegistrationParity` constructs the
  prepared graph through `server.NewPreparedServer` using a temporary prepared
  conversation store, explicit memory config/secrets and synthetic routing.
  Both graphs call the shared `composeProductionServices`. It checks nonempty
  identical listed tool names and preservation/isolation of caller stores.
  Prepared MCP factory calls remain zero.
- `TestProductionCompositionDisabledAdmissionRefusal` calls the actual production
  prepared opener with credentials disabled and requires wrapped
  `secrets.ErrDisabled`, no server/cleanup, and zero credential, runtime,
  provider and MCP factory calls.
- Existing policy-before-assembly and credential-opener zero-count assertions
  remain intact.

This is lower-level shared service registration evidence, **not** full production
opener, provider, worker execution, lifecycle, transport or runtime parity. No
native credentials, real provider/network calls or child processes were used.
The fixture writes a dummy generation executable but does not execute it.

## Focused verification

Both commands passed offline with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off
GOSUMDB=off`:

```
go test ./cmd/cercano -run '^TestProductionComposition' -count=1
go test -race ./cmd/cercano -run '^TestProductionComposition' -count=1
```

The first race attempt encountered a concurrent unrelated build edit:
`vet: cmd/cercano/main.go:492:4: undefined: cancelProducers`. A subsequent identical
race command passed (package reported 1.765s). No changes to that file were made
for this repair. No full-suite claim is made.

## Generation production scheduling slice (2026-10-28)

`newProductionDevelopmentHostWithDependencies` now calls the shared
`newProductionPreparedServer`, which installs `preparedProductionScheduling`
through `PreparedServerOptions.Scheduling`. Recap uses the existing open runner
and optional fast/light model; compaction uses configured dispatch candidates
and confirmed local runtime capacity when applicable. Invocation-local accounting
is passed to both shared services and producer attempt sinks. The prepared server
retains exact-owner admission, fencing, callback joins and constructor rollback.
No ID-only legacy scheduler is installed on this path.

Removed the production-composition comments claiming prepared background
producers are all disabled, and replaced the direct prepared constructor call
with the scheduling-enabled shared binding. The empty legacy scheduling argument
to agent composition is intentional: prepared scheduling is installed afterward
by the server's counted owner callbacks. Ordinary legacy startup is unchanged.

`TestProductionPreparedSchedulingOwnership` exercises that surviving shared
constructor with A/B servers on an isolated prepared temporary store, memory
secrets and blocking synthetic providers. Both recap and compaction reject
ambient, sibling and foreign-host contexts; pending timers block release and
then drain; active provider calls hold shutdown joins until completion. Cancelled
execution retains unresolved closure protection rather than automatically
releasing or retiring ownership. This is not full composition convergence:
foreground/child admission, MCP/watchdog restrictions, transport and runtime
parity remain caller-specific limitations. Default generation accounting remains
absent unless supplied; this change does not create an accounting collector.

Verification (from `source/server`, offline, no native opener):

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local \
  go test -race ./cmd/cercano -run 'TestProductionPreparedSchedulingOwnership|TestProductionSchedulingCommonFactory|TestDevelopmentProduction|TestProductionComposition' -count=1
```

Passed (`ok`, 5.898s package time). Existing production constructor acceptance
tests use only `enabledDevelopmentFakes`: its implementation first installs
memory secrets/runtime, synthetic providers and watchdog/MCP traps, then applies
a test-local policy override (`development_fake_dependencies_test.go:33-105`).
The new shared-constructor test keeps credentials disabled throughout. Initial
expanded-test runs exposed an incorrect expectation that cancelled active work
could be released, and duplicate turn IDs between A/B; corrected only fixture
expectations/IDs, not safety checks. No full-suite or live-runtime claim.

Additional offline boundary regression: `go test ./internal/server -run
"^TestPreparedScheduling" -count=1` passed (0.699s), with the same disabled
credentials/offline environment. `git diff --check` passed.
