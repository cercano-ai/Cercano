# Phase 1 MCP invocation-local composition slice

Ordinary startup now calls `composeProductionMCP` with the admitted configuration directory. The default factory remains `mcphost.New`, with the same registry and ten-second call wait. Ordering remains capability installation → manager construction → SetMcpManager → Start → worker installation. Configuration/dial failures remain manager-owned log/status failures, not fatal startup errors. Cleanup remains after gRPC drainage and before server shutdown, cancelling then stopping only the captured manager instance.

Prepared composition calls the same policy-aware factory boundary but deliberately opens **zero** managers. Existing shared-definition writes remain blocked by prepared policy. This does not claim external integration registration parity or lifecycle completion: exclusive integration ownership and cross-host definition mutation/reconciliation are still pending. No startup admission branches or permissions package were changed. No activation flag was added.

The manager's narrow invocation-local transport constructor allows the real initialize/list/call/discovery path to run over SDK in-memory transports. Tests use disposable configuration files, independent per-host manager/session instances and opener counters. They verify ordinary RPC registration, discovered tool invocation, idempotent owned cleanup, sibling usability after cleanup, nonfatal malformed-config/connect failure, invalid-policy fail-before-open, and prepared zero-open policy through the production prepared host fixture. The paired composition test still establishes built-in service parity only, not external MCP parity.

Validation (2026-10-27), from `source/server`, on this checkpoint's code:

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./cmd/cercano ./internal/mcp_host -run 'TestProduction(MCP|Composition)|TestManager|TestConn|TestEndToEndRegisterAndCall' -count=1
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./cmd/cercano ./internal/mcp_host -run 'TestProduction(MCP|Composition)|TestManager|TestConn|TestEndToEndRegisterAndCall' -count=1
```

Both passed. An initial fixture run exposed a missing capability-registry initialization in the test helper; the helper now installs the production capabilities before composing MCP. No production lifecycle workaround was introduced. No real MCP subprocess, network service, native credential access, live configuration, install, merge or push was used. Full suites/native qualification were not run. Phase 1 seam and composition tasks remain in progress; enterprise seams and prepared MCP lifetime/coordination are not completed by this slice.
