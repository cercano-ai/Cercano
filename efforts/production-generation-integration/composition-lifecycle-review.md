# Composition lifecycle review — 2026-10-27

This is partial implementation evidence, not production parity, live activation, or cutover approval.

## Process authority diagnosis

Task `task_0fe64704bb8a37a3c0f3b322393bb273` reports an offline in-memory probe of `modelpolicy.Install` with denying authorities A/B. After installing A then B, cleanup A unconditionally restored its captured predecessor: requests became unmanaged (or used an allowing context authority). Cleanup B then restored retired A. The probe asserted this defective behavior and passed; its temporary test was removed. This is a reproduced public API defect, not a demonstrated deployed A/B bypass.

The approved architecture uses separate generation-pinned Agent processes. No ordinary startup path was established to install two independently retiring full Agent authorities in one OS process. Existing two-runtime cleanup tests do not establish two policy installations.

Decision recorded in the autonomy ledger: preserve one Agent authority per process and use separate-process full-host A/B qualification. Do not change process-over-context precedence, introduce a policy dispatcher, or claim arbitrary overlapping Install support. A simple compare-and-swap cleanup would not prevent restoring a retired predecessor. Any future requirement for multiple Agent authorities in one process needs explicit security design review. The API defect remains uncorrected; tests must not conceal it through overlapping global installations.

## Catalog/accounting slice

Implementation task reports checkpoint `c66eeeb6fd3e`: actual ordinary callsites now use invocation-local catalog and accounting dependencies, real defaults, config-relative paths, and owned cleanup. Prepared admission refuses before opening these dependencies. Canonical generation activation remains blocked.

Parent verification from the execution worktree, with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

- `go test -race ./cmd/cercano ./internal/telemetry -run 'TestProductionCatalog|TestProductionAccounting|TestTelemetry' -count=1`: exit 0; cmd/cercano PASS (1.846s); telemetry had **no matching tests**, not a telemetry verification.
- `go test -race ./internal/telemetry -count=1`: exit 0, PASS (2.721s).

Source inspection of `production_catalog.go` confirms cancellation/stop captures the invocation manager. Stop does not join an in-flight fetch; it is not retirement-readiness proof. Accounting slice does not establish cross-host schema/retention qualification.

## Next unresolved lifecycle work

A prior static review identified conversation-store and retention startup before listener opening without failure cleanup, and in-process runtime construction with no-op cleanup despite owned warming/download work. Implementation task `task_faae7a930e7185d46839acd649dd8ea3` is assigned to reproduce these faults with isolated injected dependencies before fixing them. No outcome is claimed here. Shared remote modelhost global shutdown is prohibited; only invocation-owned resources may be closed.

Remaining full-production work includes lifecycle-counted producers, watchdog/provider rebuild wiring, compatible shared-authority integration, cross-host policy visibility, full acceptance, and copy-based adoption rehearsal. No native credentials, live migration, installed-launcher replacement, or user-process changes were authorized by this review.
