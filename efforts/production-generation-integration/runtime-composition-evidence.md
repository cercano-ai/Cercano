# Phase 1 bounded runtime composition slice

Base: `2b5275d779272c16d42ebd2808ddf301715fa12b` (2026-10-27).

Ordinary startup now supplies invocation-local probe, mode resolution, remote
connector, local-manager builder and warmer dependencies. Real defaults call the
existing complete implementations, including supported runtime providers,
callbacks, orphan handling and download resumption. The existing engine and
provider wiring is unchanged. Probe/engine/connect ordering is unchanged.
Ordinary remote failure still falls back; caller cancellation still fails without
fallback. Remote cleanup is idempotent per attachment and remains after server
shutdown. No new global override is used.

The canonical generation activation gate precedes dependency validation and
opening. Prepared development construction is unchanged: it still borrows the
exact lifecycle store and requires its explicit runtime authority connector;
this legacy fallback policy is not offered to prepared hosts.

Offline verification (from `source/server`, with
`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`):

- `go test ./cmd/cercano -run '^TestProductionRuntime' -count=1` — PASS.
- `go test -race ./cmd/cercano -run '^(TestProductionRuntime|TestProductionStartup|TestRunAgentStartup)' -count=1` — PASS.

New tests use fake runtime managers/connectors only. They cover probe warnings,
remote endpoint-before-warm ordering, independent A/B attachment cleanup,
continued B use after A cleanup, in-process mode, remote-error fallback,
cancellation without fallback, and rejected/malformed admissions before runtime
opening. Existing selected startup tests cover disposable authority/store
fixtures and exact prepared-store identity. No live config/database, native
credential/runtime probe, native process or installation was used. The whole
package suite and live/native runtime qualification were not run.

Phase 1 remains **in progress**. This is the explicitly bounded runtime slice,
not full shared-service parity. Catalog/telemetry/accounting construction,
watchdog/provider-rebuild dependencies and production producer integration still
require extraction, invocation-local store/transport fixtures and lifecycle-safe
coordination/drainage tests. Prepared fallback remains prohibited and canonical
generation activation remains disabled. No schema migration, cross-host
coordinator, launcher change, push or merge is included.
