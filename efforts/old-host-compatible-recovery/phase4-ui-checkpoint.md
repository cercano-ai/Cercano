# Phase 4 UI checkpoint — partial cleanup

The contained-source recovery UI now presents one host-interruption offer and confirms it through `LifecycleResume`. It refuses legacy original-operation and cooperative-takeover envelopes rather than translating them into permission to stop a host. The selected destination identity, endpoint and credential-file binding must match. Enter, Escape and No decline; stale UI subscription consent cannot execute. Success requires exact current ready ownership with a changed epoch, not a historical handover result.

## Recovery scope

The approved journey is destination-coordinated: read durable ownership, identify the exact source and selected destination, disclose whole-source-host/sibling interruption, obtain fresh consent, revalidate, hold launch exclusion and establish source writer containment, then recover ownership and reload committed history. Journals are evidence, not authority. Interrupted tasks/actions are not automatically replayed. Earlier external effects may remain uncertain and are not rolled back.

The containment implementation is scoped to supported macOS prepared hosts and their established process/store contracts, not arbitrary historical executables or arbitrary SQLite connections. A stuck drain, uncertain attribution, surviving writer or uncertain containment must refuse reassignment. Ownership rows and advisory locks do not fence arbitrary SQL writers. This checkpoint is fixture verification, not a claim of live recovery or authorization to perform it.

## Changes and retained work

Removed the CLI original-retry/legacy/takeover prompt branches and `LifecycleTakeover` invocation from `original_recovery.go`. Replaced the takeover-prompt test with `TestRecoveryUIRejectsInvalidBindings`; default-No and stale-callback behavior remain covered by `TestLiveSourceRecoveryUI` and `TestForcedResumeConfirmationAndStaleCallbacks`.

No public RPC definitions or client wrappers were removed in this checkpoint. The original wire envelope, server retry/takeover/original-recovery orchestration, legacy forced-resume UI, mocks and generated bindings still exist. Therefore Phase 4 is **not complete**. Protobuf reservations/regeneration, broader consumer removal, useful server regression migration, global-descriptor-scan usage audit/removal, and current-documentation cleanup remain outstanding. Historical verification records are unchanged.

## Executed verification

All commands passed on the current macOS worktree using fixtures; no live host/configuration/VM mutation:

- CLI: `go test ./internal/ui -run 'Test(LiveSourceRecoveryUI|RecoveryUIRejectsInvalidBindings|ForcedResume)' -count=1 -v`, and the same with `-race`. Three selected top-level tests, with binding and confirmation subtests.
- Server: `go test ./internal/server ./pkg/agentclient -run 'Test(LiveSourceRecovery|ForcedTransfer)' -count=1 -v`, and the same with `-race`. Server selected `TestLiveSourceRecovery` (recover/reserved/stale/uncertain); agentclient selected three ForcedTransfer tests (historical missing method, no silent fallback, lost destination response).
- Compile-only: `go test ./... -run '^$'` in both server and CLI modules. These compile checks intentionally select zero tests and are not counted as behavioral coverage.

The historical old-process acceptance was not rerun here. Independent review, remaining Phase 2/3 safety coverage, and broader Phase 5 ownership/admission/process-control verification remain open. No push or merge; `.tmp-old-recovery/` is excluded from the checkpoint.
