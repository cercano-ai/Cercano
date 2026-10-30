# WAL startup admission regression (2026-10-27)

All runs below used only telemetry tests and temporary SQLite fixtures, with
`GOPROXY=off GOSUMDB=off`; no providers, native inference, or live databases.
Commands ran from `source/server` using Go 1.26.3 darwin/arm64.

Before the fix:

```
go test -race ./internal/telemetry -run TestSchemaOpenConcurrent -count=30
```

Exit 1: three `TestSchemaOpenConcurrentFresh` failures and one
`TestSchemaOpenConcurrentLegacyPreservation` failure (line 76), each reporting
`failed to enable WAL mode: database is locked (5) (SQLITE_BUSY)`.
These fixtures already use the constructor's DSN `busy_timeout(5000)`.
The synchronized eight-opener probe therefore reproduces a WAL admission race,
not a failure of the transactional schema migration admission.

The fix retries only typed SQLite BUSY errors at journal-mode admission, with
10 ms spacing and a five-second context budget. Individual SQLite calls retain
the existing connection busy timeout. Non-BUSY errors return immediately.
Schema validation and migration transaction checks are unchanged.

WAL still precedes schema validation, preserving ordinary startup behavior.
Incompatible-schema rejection preserves schema definitions and rows, not
necessarily journal mode, database bytes, or sidecar files. The code comment
and test name now state that narrower guarantee explicitly.

After the fix (both exit 0):

```
go test -race ./internal/telemetry -count=2
# ok ... 5.426s
go test -race ./internal/telemetry -run 'TestSchemaOpenConcurrent|TestWALAdmission' -count=50
# ok ... 24.935s
```

Coverage includes synchronized fresh/legacy opens, legacy row preservation,
invalid-schema rejection without schema/data changes, real SQLite BUSY retry,
bounded admission expiry, immediate non-BUSY propagation, and visible
constructor I/O errors. Parent should independently rerun the regression.
