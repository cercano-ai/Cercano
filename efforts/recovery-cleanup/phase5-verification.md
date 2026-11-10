# Phase 5 — contained adapter and naming cleanup

Baseline: Phase 4 `9e0c20489ff85264992d11dcc26a8ee5609c1e61`. Read repository guidance, specification, plan and Phase 4 completion evidence. This phase does not supersede historical red evidence or perform Phase 6 acceptance. No new runtime bug was identified or fixed; this is interface/path consolidation and naming cleanup.

## Direct consumer and safety review

- The removed `LegacyProcessStop.Stop` contract and `developmentLegacyStop.Stop` wrapper had no remaining production recovery caller. `liveSourceRecovery` uses `WithStoppedSource`; preparation alone is now expressed as `RecoverySourcePreparation`/`CoexistenceOptions.RecoverySource`. The unrelated launcher `developmentReplaceBlockedWith` still consumes `brewrestart.StopLegacyHost`; it and platform implementations remain intact.
- `developmentContainedSource.containSource` retains record/process revalidation, fresh validation and authenticated shutdown. The redundant pre-callback absence check is consolidated into the production `WithStoppedSource` native polling check. Failed inspection still requires a separate absence proof before consent validation. Native verification is not replaced by the unit-test absence seam.
- `withSource` runs the entire callback inside `developmentRecoveryGuardWithVerifier`'s generation lock; release happens only after callback completion. The server callback validates again and runs `LifecycleRecover`, including mutation/reload, under that exclusion. Error returns also release the lock. Typed guard outcomes are unchanged.
- Tests retain orchestration absence/use-failure and lock-release assertions using a test-only `stopAndUseSource`. Added `final-native-refusal` demonstrates the production entry point cannot bypass native verification via the injected absence seam (relative store refused, deadline returned, no use). Shutdown response loss, refusal, unreadable process, bounded drain and cancellation stay fail closed. Unknown guard failures stay terminal; only classified live refusal allows discovery.
- Internal adapter/snapshot/shutdown and CLI offer/validation names now describe containment rather than obsolete legacy/original paths. `LifecycleOriginalRecovery`, `OriginalRecovery`, `ConfirmOriginalRecovery` and snapshot JSON fields remain unchanged. CLI comment documents this mapping. Wire migration is a separate follow-up; legacy/takeover offers and old confirmations remain rejected. No protobuf/generated changes. Existing native controller port/assertion logic is unchanged (only adapter names/options updated).

## Commands and results

All commands below exited 0 on Darwin. Tests use temporary fixture state, injected inspection or disposable owned helper processes, not live application hosts or user databases. Normal/race runs had nonzero selections, no skips and no race reports.

From `source/server` (each command actually executed):

```sh
go test ./internal/brewrestart -run '^TestDeadHostKernelVerification$' -count=1 -v -timeout=60s
go test -race ./internal/brewrestart -run '^TestDeadHostKernelVerification$' -count=1 -v -timeout=60s
```

Both selected exactly `TestDeadHostKernelVerification` and passed (package times 0.290s normal, 2.335s race). This explicitly closes the Phase 4 regex omission; selection is not inferred from a general Recovery regex.

```sh
go test ./cmd/cercano ./internal/brewrestart ./internal/server -run '^Test(DevelopmentLegacyProductionAdapter|LegacyAuthenticatedShutdownWait|LegacyShutdownAuthenticatedTransport|LiveRecoveryRefusesUnqualifiedWriters|DevelopmentRecovery.*|PreparedWriterAttributionBranches|PreparedGenerationInventory|DeadHostKernelVerification|Resume.*|LiveSourceRecovery.*|ExactRecovery.*|RecoveryUncertain.*)$' -count=1 -v -timeout=180s
go test -race ./cmd/cercano ./internal/brewrestart ./internal/server -run '^Test(DevelopmentLegacyProductionAdapter|LegacyAuthenticatedShutdownWait|LegacyShutdownAuthenticatedTransport|LiveRecoveryRefusesUnqualifiedWriters|DevelopmentRecovery.*|PreparedWriterAttributionBranches|PreparedGenerationInventory|DeadHostKernelVerification|Resume.*|LiveSourceRecovery.*|ExactRecovery.*|RecoveryUncertain.*)$' -count=1 -v -timeout=180s
```

Package times normal: cmd 1.101s, brewrestart 1.582s, server 3.381s. Race: cmd 2.311s, brewrestart 4.612s, server 7.833s. Actual top-level selections (same normal/race):

- cmd: TestLegacyAuthenticatedShutdownWait, TestLegacyShutdownAuthenticatedTransport, TestDevelopmentLegacyProductionAdapter, TestDevelopmentRecoveryRegistryExclusion, TestDevelopmentRecoveryGuardErrorsRemainUnclassified, TestLiveRecoveryRefusesUnqualifiedWriters.
- brewrestart: TestDeadHostKernelVerification, TestPreparedWriterAttributionBranches, TestPreparedGenerationInventory.
- server: TestExactRecoverySourceRequiresExistingReservationGap, TestExactRecoverySourceRejectionCanFenceGap, TestExactRecoveryDestination, TestExactRecoveryTimeoutAndExactRetry, TestExactRecoverySourceOwnershipChangedAfterPreflight, TestExactRecoverySourceMissingReceiptGap, TestResumeFailedReloadReproduction, TestLiveSourceRecovery, TestResumeRecoveryLookupFailure, TestRecoveryUncertainOutcomeNeverReady, TestResumeLegacyConfirmationRequiresUpgrade, TestResumeNewDestinationInterruptedTransfer, TestResumeFallbackPolicy, TestResumeReadyJournalFailure, TestResumeAcceptanceCleanupFailure, TestResumeOrdinaryBeforeContainedRecovery, TestResumeReadyRetainsCompletedOperation.

```sh
go test ./cmd/cercano ./internal/server -run '^Test(NativeControllerAssertions|LiveRecoveryConsentMutationBeforeStop)$' -count=1 -v -timeout=180s
go test -race ./cmd/cercano ./internal/server -run '^Test(NativeControllerAssertions|LiveRecoveryConsentMutationBeforeStop)$' -count=1 -v -timeout=180s
```

Selected TestNativeControllerAssertions and TestLiveRecoveryConsentMutationBeforeStop (18 consent mutation cases across confirmation/final-pre-stop); all passed. Package times normal 0.410s/0.877s, race 2.006s/2.561s.

From `source/clients/cli`:

```sh
go test ./internal/ui -run 'Test.*(Recovery|ForcedResume)' -count=1 -v -timeout=180s
go test -race ./internal/ui -run 'Test.*(Recovery|ForcedResume)' -count=1 -v -timeout=180s
```

Both selected TestConnRecovery_ReconnectedRefreshesAndAnnounces, TestConnRecovery_PendingConfirmDoesNotRestorePromptIntoComposer, TestConnRecovery_SteadyConnectedIsSilent, TestRecoveryUIRejectsInvalidBindings, TestLiveSourceRecoveryUI. All passed (0.432s normal, 1.581s race); enter/n/esc decline and explicit y acceptance remain covered.

`git diff --check` passed. A bounded source-inspection command also attempted a nonexistent `forced_resume_test.go`; grep exited 2 for that path, not a test failure. Actual UI selections above establish coverage.

## Final repeated checks

Before checkpoint, repeated the isolated dead-host commands verbatim (0.439s normal / 2.438s race), the focused three-package commands above with only `-v` omitted (normal 1.656s / 1.676s / 3.148s; race 3.445s / 4.738s / 7.643s), and the UI commands verbatim (0.423s / 1.618s). All exited 0. The isolated fixture again explicitly selected and passed TestDeadHostKernelVerification in both modes.

## Limitations

Focused normal/race fixture verification only, not full suites, historical production acceptance, independent final review, VM, deployment or cross-platform runtime qualification. Native proof remains bounded to supported prepared-generation hosts/workers, not arbitrary SQL writers. No live hosts, user data, credentials, installation changes, merge or push; historical evidence retained. Phase 6 remains pending.
