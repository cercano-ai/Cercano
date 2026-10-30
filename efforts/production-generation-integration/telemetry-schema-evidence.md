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

## Resumed schema review fixes — 2026-10-28

Source parent: `f5eb62f468fb3d104d7740fcb37d19002ed0486a` on
`feat/production-generation-integration`. This entry supersedes the earlier Go
execution limitation for the checks below; Go is now available from PATH
(`/opt/homebrew/bin/go`). No plan status or activation gate was changed.

### Executed before-validation-fix constructor probes

Added disposable modernc SQLite constructor regressions, then ran:

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./internal/telemetry -run TestSchemaReviewRejects -count=1 -v
```

This failed with all seven expected subcase failures:

- Both `events` and `cloud_usage` with `WITHOUT ROWID`: constructor accepted;
  omitted-ID write failed with `NOT NULL constraint failed: <table>.id (1299)`.
- Both tables with inline `INTEGER PRIMARY KEY DESC`: constructor accepted;
  omitted-ID write returned nil but persisted one NULL ID.
- Sessions with an unknown required column defaulting to `NULL`, `(NULL)`, or
  `(nullif(1,1))`: constructor accepted; `RecordSession` returned nil with zero
  persisted rows because `INSERT OR IGNORE` suppressed the NOT NULL violation.

The invocation-local transaction fault seam was present for compilation at probe
time; the compatibility validation changes were not yet applied.

### Bounded fix and regression coverage

- Inspect SQLite's primary-key index metadata for ID tables: a separate `pk`
  index means the integer key does not alias rowid. Reject before ALTER; no DDL
  parsing, table rebuild or data rewrite. Retain AUTOINCREMENT and ordinary
  INTEGER rowid aliases, including table-level `PRIMARY KEY(id DESC)` (unlike
  inline DESC, that is a rowid alias).
- Reject unknown required ordinary columns unless their defaults are provably
  non-NULL literals or SQLite CURRENT_TIME/DATE/TIMESTAMP keywords, optionally
  parenthesized. This conservatively rejects arbitrary expression defaults,
  including expressions that happen to be non-NULL at open but can later yield
  NULL. Defaults are not evaluated during preflight. Nullable extension columns
  remain supported; all historical canonical schemas remain supported. This is
  bounded structural validation, not general certification of arbitrary CHECKs,
  triggers, generated columns or extension uniqueness constraints.
- Exercise all eight presence/absence combinations of the three additive event
  columns, preservation of non-default existing values, correct upgrade defaults,
  and repeated constructor opening. Exercise the oldest layout with none of the
  three additions and no sessions table.
- Explicitly retain historical nullable `TEXT PRIMARY KEY` sessions, including an
  existing NULL key, and test successful ordinary writes after compatible opens.
- Inject failure on the second ALTER after the first succeeds inside the real
  initialization transaction. Verify schema SQL (including newly created sessions)
  rolls back exactly, existing event data survives, the original error propagates,
  and an ordinary constructor can subsequently succeed. The private seam is
  invocation-local; transaction begin/commit/rollback cannot be intercepted.

### Verification

All commands ran from `source/server` using Go from PATH, with
`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
go test ./internal/telemetry -run TestSchema -count=1 -v                         # PASS
go test -race ./internal/telemetry -run 'TestSchema|TestWALAdmission' -count=1   # PASS
go test -race ./internal/telemetry -count=1                                   # PASS
```

`gofmt` and `git diff --check` also passed. One intermediate test run exposed a
fixture setup error (the historical fixture already lacks all three additions);
corrected the test normalization, then reran the passing checks above. No live
DB/config, native credentials, provider/network access or existing hosts were
used. No generation startup, migration redesign, plan edits, merge or push.
Package tests do not establish full production parity or authorize activation.
