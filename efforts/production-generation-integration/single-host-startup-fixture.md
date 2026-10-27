# Single-host startup/shutdown fixture

Successor to `stopped-startup-fixture-recovery.md` / `014aeb2cfe6f`.

`TestProductionSingleHostStartupShutdown` calls the actual
`startGRPCServerWithLifecycle`, not a substitute graph. Sequential `serves` and
`late-mcp-failure` cases each own one full Agent/policy authority. Credentials
remain disabled and cloud configuration remains empty throughout.

## Exercised boundary

- Successful return and actual Agent/Enterprise gRPC registration over bufconn
  (no TCP listener). Agent credential policy, configuration, tools and history
  RPCs work. Enterprise control correctly rejects the non-loopback bufconn peer.
- Temporary config, permissions, conversation/task DB, telemetry DB, catalog,
  enterprise directory and diagnostic paths. Real local SQLite and filesystem
  watchers; fake runtime/catalog/provider/MCP lifecycle and watchdog audit.
- Ollama engine/embedder factory and llama/mistral HTTP clients are invocation
  local; nil retains production defaults. Transport/model/native-opener traps
  cannot enter a real backend. Enterprise uses a standalone host with trapped
  credential/browser/HTTP dependencies, not a managed enrollment.
- Fake memory credentials are usable directly, but both writable and read-only
  admission reject before either the memory or native opener is called. No
  credential enabling, HTTP test server, model call or paid endpoint is needed.
- Concurrent repeated cleanup closes the listener/store/runtime/catalog/MCP and
  watchdog audit exactly once; telemetry rejects writes after closure. Catalog
  and MCP cancellation barriers drain before return. Startup now also joins
  config/permission watchers, initial cloud rebuilding and serving, closes
  diagnostic/audit handles, and restores the previous process policy authority.
- Late MCP factory failure happens after watchers, accounting, cloud rebuilding,
  watchdog and provider wiring, and verifies the same owned-resource cleanup.
- Startup-only `in_process` scope: the actual wired server has no worker runner,
  orphan reaping is fake, runtime calls match an exact metadata-only list, and
  no turn is requested. This is **not worker acceptance or model qualification**.

## Validation

From `source/server`, passed on 2026-10-27:

```sh
GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1 go test -race ./cmd/cercano ./internal/server ./internal/hostsvc/permissions ./internal/hostsvc/providers -run '^Test(ProductionSingleHostStartupShutdown|ProductionCredentialRefusesBeforeOpen|ProductionListenerFailureClosesOwnedStore|ProductionFailureStages|ProductionStartupGenerationRejectsLegacyBeforeEffects|ProductionProviderActivationBeforeDependencies|SelectExecutionMode_InProcessLeavesWorkerNil|StartConfigWatcher.*|WaitWithNotify.*|Broker.*|BuilderCredentialAdmission|CloudBuilderInvocationAndCredentialBoundary)$' -count=5 -timeout=120s
```

All four packages passed. `git diff --check` passed. This is targeted offline
race validation, not a full-suite run. No multi-authority fixture, enabled
production startup, compiled acceptance, live qualification, launcher change,
native credential access, worker backend, push or merge was performed.
Canonical prepared activation remains gated; no broad plan completion is claimed.
