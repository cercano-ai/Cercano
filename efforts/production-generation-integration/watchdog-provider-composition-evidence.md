# Watchdog and provider construction — bounded Phase 1 slice

2026-10-27. Continuation from e406d06ab7e2; candidate is the explicit-path checkpoint containing this evidence. Phase 1 remains in progress. No canonical activation, live launch, push, migration or native credential access was performed.

## Actual wiring and defaults

- Ordinary `startGRPCServerWithLifecycle` in `cmd/cercano/main.go` installs invocation-local cloud builder and watchdog dependencies on the actual Server before service assembly and the asynchronous `RebuildCloud`. Its legacy cloud runner factory uses the same injected builder (nil selects `cloudfactory.BuildCloudProvider`).
- `composeProductionServices` in `production_composition.go` installs the dispatch engine before `InitWatchdog`, and calls watchdog initialization only under legacy execution policy. Watchdog construction/config updates consume the server-local dependencies; nil dispatch retains `dispatch.OneShot` with `TaskWatchdog`, nil audit opener retains SQLite. Disabled watchdog opens nothing. Audit-open failure remains warning-only, without disabling supervision.
- `providers.New` returns a composable resolver; `Server.SetCloudProviderBuilder` installs its builder. `service.buildProfile` still resolves and validates credentials/token sources before invoking the builder; nil retains the real cloud factory. No credential admission semantics changed.
- The prepared development constructor passes builder/watchdog dependencies through `PreparedServerOptions`; it retains preflight, exact lifecycle store and pinned workers. Its existing cloud-only rebuild uses that resolver. Supplying watchdog collaborators does **not** initialize prepared supervision, start an audit writer, or grant producer ownership.

## Behavioral evidence (not name parity)

`TestProductionProviderPairedRebuild` constructs an ordinary Server and the prepared production development host with isolated configuration/state, memory secrets and fake providers. Both rebuild their actual provider service and execute a synthetic Chat through it. Failure in the ordinary factory clears its cloud provider while the prepared graph remains callable and its builder count unchanged. Prepared watchdog dependencies trap unexpected audit/dispatch calls, with watchdog enabled in fixture configuration.

`TestProductionProviderActivationBeforeDependencies` checks the canonical activation gate at the actual lifecycle startup entry, before incomplete runtime dependencies or injected provider/watchdog dependencies. Existing composition/preflight tests remain in the focused regression selection.

`TestCloudBuilderInvocationAndCredentialBoundary` checks independent resolvers, construction failure, usable sibling inference and missing-required-credential failure before builder invocation. Subscription fixtures use memory token sources which the fake builder does not call; this is not issuer/refresh qualification.

`TestWatchdogInvocationDependencies` checks independent server-local audit/dispatch collaborators, disabled-before-open behavior, and continued plain-English supervision after audit construction failure.

## Verification

From `source/server`, final passing command:

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./cmd/cercano ./internal/server ./internal/hostsvc/providers -run 'Test(ProductionProvider|CloudBuilderInvocation|WatchdogInvocation|BuildWatchdog|ProductionComposition|ProductionStartup|ProductionRuntime|ProductionCatalog|ProductionAccounting|RunAgentStartup|DevelopmentProduction)' -count=1
```

All three packages passed. Earlier paired-fixture probes failed because the call omitted its model, then because the reused synthetic provider intentionally rejected Chat; the fixture now provides an explicit model and successful fake Chat. The activation probe first hit missing-admission-path validation; isolated resolved paths now exercise the intended activation-disabled gate. These were test-fixture corrections, not weakened production checks.

## Outstanding gates

Real registration and synthetic provider behavior are established only at the listed seams. This does not prove full ordinary/prepared feature parity or full successful startup/shutdown acceptance. Prepared watchdog/background catalog/accounting and provider rebuild lifecycle ownership, counted work/drainage, configuration/credential invalidation across hosts, external integrations, and compiled production A/B acceptance remain outstanding. Equal tool names are not behavioral parity. Existing runtime/catalog/cleanup evidence limitations remain; this slice does not change runtime cleanup, authorization, modelpolicy or credential policy. Canonical generation activation remains blocked in `productionStartupAdmission.validateActivation`. No full-suite, native backend, real provider or live-user qualification is claimed.
