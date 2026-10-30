# Ordinary per-host MCP composition — 2026-10-29

Supersedes the prepared zero-open MCP restriction described in the earlier MCP composition evidence. Per the user's explicit direction, both execution policies now use the existing ordinary manager factory, registration, Start and once-only cancel/Stop lifecycle. No cross-host forwarding, sibling holds, owner service or endpoint transport was added. The prepared host's existing `closeMCP` ownership/cleanup integration is reused unchanged.

## Reproduction and implementation

Before removing the policy early return, the new isolated stdio test failed with `prepared composition silently omitted its manager`:

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./cmd/cercano -run '^TestProductionMCPPreparedStdioOwnership$' -count=1 -timeout=60s
```

After removal, two real manager instances initialize/list/call tools via distinct SDK command-transport sessions to a fake MCP stdio executable (the test binary in helper mode). Closing A removes its manager handles and registered tools; B still calls its tool. Repeated A close is harmless. A missing configured executable yields ordinary nonfatal startup with a listed `failed` server and a nonempty manager error, rather than omission; B still works.

`TestProductionMCPOwnedPreparedCleanup` additionally constructs two actual prepared production graphs with invocation-local fake external dependencies and real MCP command transports. It verifies the manager receives the host's configuration-relative authority, and exercises the existing exact-owned prepared cleanup before calling B again. The first fixture attempt correctly hit the credential admission guard; the test now uses the existing `enabledDevelopmentFakes` helper after installing every external boundary fake, not a production admission bypass. This briefly enables **only the test parent with injected memory stores**. Every MCP child explicitly receives `CERCANO_DISABLE_CREDENTIALS=1` and asserts that value before serving. Its HOME and configuration are temporary; helper lifetime is bounded to 15 seconds. No native credentials, providers, installed/live integrations or unrelated processes are used.

The shared graph-only fake dependency helper now supplies an ordinary manager with suppressed Start when no explicit MCP fixture is provided. Explicit MCP dependencies are preserved for real stdio lifecycle tests. The paired composition test now requires prepared manager construction rather than enforcing omission.

## Verification

From `source/server`, both commands passed with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
go test ./cmd/cercano ./internal/mcp_host -run 'ProductionMCP|ProductionComposition|DevelopmentProduction|ProductionDevelopment|TestManager|TestStop|TestRestart' -count=1 -timeout=90s
go test -race ./cmd/cercano ./internal/mcp_host -run 'ProductionMCP|ProductionComposition|DevelopmentProduction|ProductionDevelopment|TestManager|TestStop|TestRestart' -count=1 -timeout=120s
```

The race run includes the helper subprocess built by `go test -race`; no separate uninstrumented helper binary is used.

## Retained limits

This is not completion of Phase 4: shared-definition coordinated mutation/fresh-read conflicts, watcher reconciliation, arbitrary configured integration exclusivity, and concrete enterprise authority remain separate requirements. Their genuine guards were not removed or claimed resolved. The fixtures establish ordinary per-host stdio manager ownership, not tolerance of arbitrary external servers for duplicate starts. No canonical production activation gate, ownership/admission policy, migration, installed launcher, or enterprise lock was changed. No full suites, native qualification, live operation, merge or push occurred.
