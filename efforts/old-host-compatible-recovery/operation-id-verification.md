# Contained recovery operation identity follow-up

Bounded must-fix following review of afb4b8fe5637. No migrations.

## Reproduction and repair

Strengthened `TestLiveSourceRecovery` to compare the operation ID on ordinary resume after discarding the successful contained response. Ran the test before the implementation: owned and reserved cases both failed because the ready response omitted OperationId.

Added a read-only recovery-receipt lookup against existing conversation_handovers joined to current conversation_ownership, requiring exact conversation, destination, epoch and owned state. RecoveryID already hashes abandoned source owner/epoch and destination into the recovery-prefixed receipt. LifecycleResume uses this only after successful acquisition/readiness and only when cooperative journal metadata did not supply an operation ID. Receipt lookup grants no authority.

The regression now covers owned/reserved response loss, repeated ready resume, unchanged epoch and one total stop. After release/reacquisition it rejects both a new-epoch lookup and an old-epoch lookup against historical recovery evidence. Intermediate test-fixture attempts to reacquire via the same admission registry failed; the final historical-mismatch probe deliberately uses a fresh store capability instead of invalidly retaining stale admission state.

## Executed checks

On darwin/arm64, offline (`GOPROXY=off`, final checks also `GOTOOLCHAIN=local GOSUMDB=off`):

- PASS normal and race: `go test [ -race ] ./internal/server ./internal/conversation -run 'Test.*(LiveSourceRecovery|LiveRecovery|Resume|DeadOwner|Handover)' -count=1`.
- After adding the additional repeated-ready assertion: PASS normal and race `go test [ -race ] ./internal/server -run '^TestLiveSourceRecovery$' -count=1`.
- PASS compile only: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/cercano` (output moved to `/tmp/cercano-opid-linux`).
- PASS compile only: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/cercano-opid-windows.exe ./cmd/cercano`.

No non-Darwin runtime claim. No tooling installs, live host/config/VM mutation, push or merge. `.tmp-old-recovery` untouched. Historical production integration was not rerun for this metadata-only fix; earlier results remain historical evidence, not a new run.

## Remaining evidence and plan status

Read final-verification.md and plan.md. No composite task promoted merely because focused suites pass. This bounded unit closes the reproduced contained-response-loss operation-ID defect, not the full Phase 2/3 matrix.

Still outstanding in this run: explicit native attribution denial/fallback/disappearance branch evidence; consolidated unrelated-host, cancellation, PID-reuse and surviving-writer mapping; declined/stale consent and source-identity scenario closure (existing LiveRecovery tests ran, but no complete matrix audit); uncertain commit/retry injection and completion-notification versus prohibited-action-replay evidence. Full historical acceptance was not rerun. Phase 3 timeout identity and complete retry/late-claim matrix remain unchecked. Independent review and broader Phase 4 status reconciliation remain open. Cross-compilation adds platform compile evidence only; Phase 5 platform/integration task stays in progress.
