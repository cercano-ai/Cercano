# Phase 1 catalog/accounting composition slice

Base source: `d0df9c0a5cead5d542b91c29651203a31f602187`. This continuation is a bounded ordinary catalog and accounting extraction, not service parity or activation evidence.

## Actual integration

Ordinary `startGRPCServerWithStartup` → `startGRPCServerWithRuntime` now enters `startGRPCServerWithServices` with invocation-local catalog/accounting dependencies. Defaults perform the existing real construction: Ollama manager, best-effort cache load, background start, then HuggingFace/Ollama/DeepInfra registry; SQLite telemetry collector with accounting enabled. Config-relative cache and telemetry authority paths are unchanged. Accounting wiring remains in its original position, after producer/config setup; usage/attempt sinks and producer-stop-before-collector-drain ordering remain unchanged. Telemetry opening failures remain ordinary startup warnings, not a new strict compatibility gate.

Catalog cleanup cancels and stops only the manager created by that invocation, once, on startup failure or ordinary shutdown. This adds a bounded cleanup signal where startup previously used a background context without cleanup. Manager Stop does **not** join a fetch already in progress; cancellation is not a generation retirement proof. No shared-cache mutation coordination is claimed.

Both new composition functions reject nonlegacy/prepared admission before any factory, cache access, SQLite migration or goroutine. Ordinary entry validates dependencies before runtime/enterprise construction. Canonical activation stays blocked by the existing admission gate; these helpers cannot grant prepared producers authority.

## Verification

From `source/server`, with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
go test -race ./cmd/cercano ./internal/telemetry -run '^(TestProduction(Catalog|Accounting|Runtime|Startup)|TestAgentTelemetry|TestRunAgentStartup|TestCollectorOwnsAccountingLane|TestProducerCoverageGapSurvivesSuccessfulWrites)' -count=1
```

PASS for both packages. New tests cover refusal before dependencies (including the actual startup entry), incomplete catalog dependencies, exact config-relative paths, cache-error continuation and construction order, independent A/B catalog cancellation and idempotent stop, and two real temporary SQLite collectors: closing A does not prevent B emitting; both legacy events and accounting attempts survive collector drainage and reopening.

Catalog lifecycle/cache operations are fakes (no refresh/probe/network); server fixture uses the existing fake runner. Accounting uses only temporary SQLite stores. No live config, credentials, assets, filesystem authorities, transports or providers were accessed. Existing focused runtime tests use fake connectors. Ordinary full server startup, full suites/builds, compiled terminal journeys and native/live qualification were not executed.

## Remaining Phase 1 / ownership gaps

Phase 1 stays in progress. Watchdog and provider-rebuild construction/transport authority remain unextracted. Prepared catalog/accounting are refused, not installed with generation ownership. Counted producer work, cross-host maintenance/cache coordination, compatibility preflight for telemetry, retirement drain/readiness integration and startup-failure drainage of all ordinary producers remain outstanding. Store/lifecycle and pinned-worker parity, writable configuration/permissions, nested workers, MCP/enterprise/runtime shared authority and full terminal acceptance still require their planned integration. This slice does not qualify concurrent accounting health publication/retention or ordinary service parity from registry/tool names.

No migration of live state, launcher change, push, merge or installation performed.
