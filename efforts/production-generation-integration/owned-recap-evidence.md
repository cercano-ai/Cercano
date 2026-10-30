# Phase 2 partial slice: opt-in owned recap

Implemented only internal/recap and the narrow persistence recap bridge. The Phase 2 background producer task remains **in_progress**; this is not full producer parity or activation evidence.

- Before modifying the generator, ran `TestLegacySchedulingAdmissionAndContextGap`: a fake completion observed missing caller context, while broker closure succeeded with the legacy timer queued. This characterization remains as a regression fixture for the legacy boundary.
- Added exact-owner admission before the timer. One ticket covers delay, recap inference/save, title inference/save, and completion. Reuses the generator's existing startWork/Fence/Close lifecycle and shared generation pipeline; no independent lifecycle manager or ID-based owner resolver.
- Like owned compaction, independent admissions do not debounce into each other. Legacy Schedule still coalesces. Recap has no automatic retry loop: a subsequent explicit schedule is the legacy retry boundary and must independently obtain exact admission. Failed owned work is never replayed or automatically reconciled.
- Context metadata flows through timeout/accounting wrappers, and bounded detached persistence writes retain the store capability. Errors and panics retain conservative persistence blockers. Failed recap writes prevent title inference/publication; rejected ticket transitions prevent execution. Cancellation before inference completes queued work; cancellation/errors after execution starts remain conservative.
- The real temp fenced SQLite bridge tests use fake summarizers, reject stale epochs/sibling authority, verify sibling writes on a separate connection during blocked inference, and exercise actual capability-rejected recap/title writes, panic, concurrent schedule/Fence/Close, queued cancellation, and rejected transitions. Fake tests also verify independent cancellation/metadata for same-conversation admissions.
- Reviewed production shutdown Fence ordering; no cmd harness changes made. Existing Fence rejects queued/new work and Close cancels/drains admitted workers. Prepared global scheduler installation and canonical activation remain disabled; no credentials, subprocesses, network, native backend, or user state were used by these fixtures.

Verification (credentials disabled):

- `go test ./internal/recap -run TestLegacySchedulingAdmissionAndContextGap -count=1` (before implementation): PASS.
- `go test ./internal/recap ./internal/hostsvc/persistence -count=1`: PASS.
- `go test -race ./internal/recap ./internal/hostsvc/persistence -count=1`: PASS.

Still outside this slice: canonical prepared wiring, global maintenance/retention, remaining producer families, full production parity, real providers/native qualification, and activation gates. No claim of completed Phase 2 or automatic durable reconciliation.
