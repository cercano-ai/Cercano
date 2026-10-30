# Generation permissions continuation (not acceptance)

2026-10-28. Resumed the existing accounting draft without reimplementing it. Parent reports `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./cmd/cercano -run 'TestProduction.*Accounting|TestDevelopment.*Accounting' -count=1` PASS before these edits. Exact new accounting tests are `TestProductionGenerationAccountingDrain` and `TestProductionGenerationAccountingHostLifecycle` (retirement/open-failure). This report is parent evidence, not an execution in this child.

Continued actual host integration: prepared production host now loads an explicitly supplied config-relative permissions.yaml using the ordinary coordinated PermissionStore, binds it to the actual server/agent, and starts the ordinary permission watcher with cancellation/join during cleanup and retirement. Invalid supplied policy fails before credential/runtime opening. Removed the unconditional static-permission assembly for supplied policy. Missing policy deliberately retains strict session-local defaults, not ordinary permissive defaults. The ordinary store rereads validated external policy at gates and retains last valid policy on malformed subsequent edits. Permission mutation RPC coexistence guard remains unchanged. No constructor activation policy was enabled.

Added `TestProductionGenerationPermissionsHost` with missing-strict/shared-policy subtests: actual injected host, external coordinated policy change observed through server RPC, malformed edit retains validated policy, mutation guard retained, retirement joins. Memory credentials/runtime/provider and accounting fixture; no turns, worker subprocesses, network or native credentials. This test is unexecuted.

`git diff --check` PASS. Attempted `/opt/homebrew/bin/go test ./cmd/cercano -run 'TestProduction.*Accounting|TestDevelopment.*Accounting' -count=1` with required disabled/offline environment; route reports binary not found. No installation attempted. Source integration continued. No checkpoint of the combined unit pending parent formatting and verification of the changed constructor.

Parent commands, from source/server:

```sh
/opt/homebrew/bin/gofmt -w cmd/cercano/development_host_production.go cmd/cercano/development_fake_dependencies_test.go cmd/cercano/production_startup.go cmd/cercano/telemetry_startup.go cmd/cercano/production_generation_accounting_test.go cmd/cercano/production_generation_permissions_test.go
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go test ./cmd/cercano -run 'TestProduction.*Accounting|TestDevelopment.*Accounting|TestProductionGenerationPermissionsHost' -count=1
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go test -race ./cmd/cercano -run 'TestProductionGenerationAccounting|TestProductionGenerationPermissionsHost' -count=1
```

Still reachable restrictions: prepared config snapshot plus frozen configuration service in internal/server/prepared.go; guarded read-only credentials; permission mutation RPC refusal; missing-policy static strict fallback; guarded MCP/watchdog/catalog; protected runtime administration; canonical production activation gate. Settings and other service convergence remains unfinished, not a newly discovered design blocker. No legacy entrypoint was deleted. Full host feature parity and accounting producer-through-host persistence acceptance remain unclaimed. No live changes, migration, native access, process termination, paid inference, merge or push.
