# Phase 1 dispatch/capability extraction — 2026-10-27

Continuation of routing extraction `bf865c0ed007`; working HEAD at verification: `8c63af38` (parallel permission work retained). This bounded implementation is authorized by the current Phase 1 task; it is not completion of production parity or activation approval.

## Implemented

Both ordinary startup and the prepared production host now use `bindProductionAgent` and `composeProductionServices`: context/locus/permission binding, live dispatch provider/model callbacks, both agent/server dispatch installations, and the real capability registry installation. Ordinary accounting sinks remain invocation-local. The explicit execution policy installs legacy selection only for ordinary startup and the existing generation-pinned `ConfigurePreparedWorker` for prepared startup. Worker installation stays after ordinary MCP setup. Only ordinary policy initializes the existing watchdog.

Prepared worker-policy validation now precedes credential/runtime dependencies and graph construction. Composition does not open/replace the supplied agent, conversation store, server or prepared authority. Existing prepared guards, static permissions, read-only credentials and background restrictions remain unchanged. No config-persistence or permission-package internals changed in this unit.

## Verification

All commands run from `source/server`, with outer `CERCANO_DISABLE_CREDENTIALS=1`; no credential-enabled scopes used.

- `go test ./cmd/cercano -run TestProductionComposition -count=1` — passed after correcting a test fake's pointer receiver compilation error.
- `go test -race ./cmd/cercano -run 'TestProductionComposition|TestDevelopmentProduction(PreflightBeforeOpener|RejectsUnsupportedSnapshotBeforeDependencies|OpenerFailsOnceWithoutFallback|RuntimeErrorWithoutFallback)$' -count=1` — passed on final formatted sources.
- `gofmt` run on the four changed Go files.

New paired test compares real ordinary/prepared tool registrations, supplies memory secrets, an in-memory ordinary conversation store, a disposable prepared database, a fake runtime manager and synthetic inference provider (no network transport), and verifies ordinary store identity. Existing injected embedding tests use fake transports. Invalid execution policies fail before graph dependency inspection; invalid prepared generation policy fails before secret opening. Existing preflight and startup-failure tests also passed under race instrumentation.

## Remaining gaps / qualification

Phase 1 remains in progress. This extracts shared graph installation, not full startup: store/lifecycle construction, provider/runtime acquisition and authoritative path resolution remain host-specific. Prepared MCP/enterprise, telemetry/accounting producers, catalog/watchdog/maintenance scheduling, writable shared settings, nested workers and ordinary terminal parity still need their ownership-aware integration. Ordinary background/MCP startup was not executed in these tests; worker installation is exercised by prepared construction but no child is launched. Full suites, compiled-host acceptance, live credentials/inference and live migration were not run. No installed launcher, user-state path or existing process was accessed; no push/merge.

The shared plan's parity/failure-handling task is marked in progress, not complete. The plan contains concurrent edits and is deliberately excluded from this code/evidence checkpoint.
