# Bounded Phase 5 telemetry schema opening

Base: `fa722ffaffea8ce30bc3db9189125cd132cf374c`. Only the legacy telemetry opener/schema checks and temporary-fixture tests are changed; accounting initialization and its `BEGIN IMMEDIATE` transaction remain unchanged. Phase 5 telemetry integration stays **in progress**, not complete. Prepared activation remains disabled; no composition, config, recap, path, retention or deletion policy change.

## Before-fix probe

Added `TestSchemaOpenRejectsView`: a temporary SQLite file containing only `CREATE VIEW events AS SELECT 1 AS id`. The old constructor uses `CREATE TABLE IF NOT EXISTS`, then ignores every nonduplicate ALTER error. Go execution was blocked by the unavailable Go executable (below).

Executed a Python stdlib SQLite reproduction of those exact create/alter statements on a `TemporaryDirectory` file, before implementing the fix. Output:

```
legacy ignored: Cannot add a column to a view
legacy ignored: Cannot add a column to a view
legacy ignored: Cannot add a column to a view
remaining columns: ['id']
```

This demonstrates the SQLite failure mechanism, **not** a passing/failing execution of the Go constructor with modernc's SQLite version.

## Implementation

Opening now reserves the writer with `BEGIN IMMEDIATE`, validates existing legacy objects and column shapes before DDL, creates missing tables and applies only the three existing supported additive upgrades. It validates again before commit. Failures roll back the transaction and return errors; failed cleanup discards the connection. ALTER errors carry column context and preserve their cause. No duplicate-error string matching is needed under the writer lock.

Checks cover table-versus-view, required columns, declared types, primary-key position, required nullability, generated required columns, and unknown required columns without defaults. Compatible optional extra columns remain untouched. This is bounded legacy structural compatibility checking, not certification of arbitrary user-defined triggers, CHECK constraints, indexes, or a new accounting representation. No data rewrite/version change is introduced.

Tests added for malformed view/table opening, schema nonmutation on refusal, injected and real query-only ALTER error propagation, simultaneous fresh/legacy opens, concurrent existing accounting initialization, supported defaults, retained event IDs/tokens/cloud/session records, current-schema reopening, and optional extension preservation.

## Validation and limitations

`gofmt -w` succeeded on all three changed Go files. With `GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1`, attempted from `source/server`:

```
go test ./internal/telemetry -run TestSchemaOpenRejectsView -count=1
go test ./internal/telemetry -count=1 -timeout=120s
go test -race ./internal/telemetry -count=1 -timeout=120s
```

Each was blocked before execution: `fork/exec /opt/homebrew/bin/go: no such file or directory`. No Go build/test/race pass is claimed. These checks still require execution in a working offline Go environment, including contention qualification for WAL initialization. No native/provider/credential access, application subprocess fixtures, real databases, backups, or live operations were used. Only a disposable Python SQLite fixture ran. Full telemetry integration, retirement drainage, cross-host production acceptance and accounting-schema qualification remain outside this slice.
