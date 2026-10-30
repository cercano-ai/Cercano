# Generation accounting continuation — unverified draft

2026-10-28. This is not acceptance evidence or a completed unit.

Draft changes connect `composeProductionAccounting` to the prepared production host, using the admitted config-relative telemetry path. Server usage, receipt-backed worker accounting, service engine sinks and prepared scheduling attempt sinks receive the collector. Prepared accounting opening errors fail construction. Resource teardown drains accounting after server/constructor producer shutdown; partial constructor rollback now precedes closing its dependencies. The test fake dependency bundle supplies an in-memory telemetry store before any credential-enabled scope.

The generic accounting helper now permits a generation admission only after the existing conversation compatibility preflight. Canonical application activation remains refused. This is not a telemetry read-only schema preflight guarantee: existing SQLite constructor/WAL and compatibility limitations remain relevant.

Added, but NOT executed: generation accounting A/B drain/reopen probe, accounting authority path and canonical gate checks, actual injected host retirement and accounting-open-failure checks. The host fixture uses memory credentials/runtime and synthetic providers with MCP/watchdog traps; no turns or workers are executed. The A/B persistence probe uses disposable SQLite files. These do not establish producer observations reaching persistence through full application execution or complete retirement health qualification.

Validation was blocked before execution:

- `gofmt -w ...` failed: `fork/exec /opt/homebrew/bin/gofmt: no such file or directory`.
- `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./cmd/cercano -run '^TestProduction(GenerationAccounting|Accounting|CatalogAccounting|Composition)' -count=1` failed: `fork/exec /opt/homebrew/bin/go: no such file or directory`.

No formatting, compilation, tests or acceptance are claimed. No checkpoint: the unit is not verified. No toolchain installation was attempted. Further wiring was not attempted with compilation unavailable.

No obsolete entrypoint was removed in this draft. The host's empty accounting sink dependency is replaced by the ordinary accounting opener. Restricted prepared snapshot configuration, read-only credentials, session-local permissions, guarded MCP/watchdog/catalog and protected runtime paths remain: their coordinated mutation, policy freshness, counted work and ownership/drain replacements are not wired in this slice. Canonical startup remains gated until full feature parity. Existing lifecycle/worker/transfer code was not changed, but regressions have not been rerun.

Remaining qualification includes formatting and focused compilation/race tests, producer-through-host accounting observations, constructor rollback faults, retirement failure/readiness health, shared SQLite consumer/retention behavior, ordinary service and terminal parity, full build/suites and separately authorized native/provider/live qualification. No installation, live migration, native credential access, paid inference, existing-process termination, merge or push was performed.

Correction to the opening progress statement: the stated clean baseline/HEAD was taken from supplied historical context, not freshly established before edits in this run. Current `git status --short` was inspected after edits and contained only the five edited/added Go files at that point. No unrelated files were reverted.
