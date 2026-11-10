# Recovery cleanup verification — Phase 3

## Scope and authorization

Executed Phase 3 only under the explicit user request; the spec's proposed-status text is historical. Read root AGENTS.md/CLAUDE.md, spec, plan and prior verification. Baseline HEAD: `6fdadbb42a7ab64e4606097b1d2c5cf34231b588`. Semantic task statuses used. No Phase 4 fallback restructuring, adapter consolidation, wire migration, live host/data/VM operation, merge or push. Tests use temporary stores and loopback fixtures, not production processes.

## Removed and retained boundaries

- `LifecycleResume` rejects `ConfirmLegacyStop` after authenticated control validation and before acquisition, journals or deferred fallback. Mixed modern/legacy flags also refuse with FailedPrecondition and actionable client upgrade guidance.
- Removed `validateLegacyApproval`, `resumeConfirmedLegacy`, legacy offer construction and the parallel UI stop-confirmation continuation. Old-server legacy offers now produce destination-server upgrade guidance; the client never translates weaker consent into modern authorization.
- Retained `LegacyProcessStop` interface and adapter implementation for separately scoped Phase 5 review; no `LegacyStop.Stop` execution consumer remains in server recovery. Retained the small UI forwarding wrapper to modern consent; naming cleanup is deferred.
- Preserved `LifecycleOriginalRecovery`, protobuf schema/generated bindings and field numbers unchanged (empty scoped wire diff). Modern contained coordinator and production exclusion implementation are unchanged. Ordinary acquisition, cooperative old-host controls, exact reservation claims, guarded dead-source recovery, terminal attachment takeover, journal durability and uncertain-outcome refusals remain.
- Removed tests specific to successful legacy execution, legacy snapshot refresh/requery and legacy UI force confirmation. Retained the fake process boundary for refusal regression. Modern tests retain default-no disclosure/selection validation, exact consent, uncertain stop refusal and completed retry identity. Added expiry, decline, replay, refreshed challenge, selected identity, owner and state mutations; existing mutations cover epoch, reservation destination/handover, endpoint/token and source at confirmation and final pre-stop. Retirement tests now expect cooperative refusal rather than a legacy offer but retain participation/restart/retirement assertions.
- Strengthened cooperative/adapterless/exact-reservation characterization to assert fresh destination epoch, exact reservation operation ID, stable retry epoch/ID, and zero stops/shutdowns; reserved claim does not contact source. Modern retry test retries the same completed confirmation and verifies no repeated stop and identical operation/epoch; this models a caller retry after losing a successful response, not packet-level response dropping.

## Red regressions and intermediate failures

The execution history records the refusal regressions failing before replacement. Independently reproduced the red baseline in this run by temporarily restoring only the four changed production files from HEAD, running the following, and restoring candidate bytes in a finally block:

```
# source/server
 go test ./internal/server -run '^TestResumeLegacyConfirmationRequiresUpgrade$' -count=1
# source/clients/cli
 go test ./internal/ui -run '^TestResumeLegacyOfferRequiresUpgrade$' -count=1
```

Both exited 1. All six server modes failed: owned/contained invoked callbacks, unowned/reserved changed ownership, local returned ready, mixed flags lacked upgrade guidance. CLI returned the obsolete interruption-confirmation offer. Candidate regression verifies exact ownership row and journal equality, zero stop/recovery/source callbacks, and upgrade refusal across owned/unowned/local/reserved/contained/mixed modes. CLI covers old offers with/without adapter snapshot and no confirmation request.

An intermediate focused server run failed three retirement subcases still expecting legacy offers; assertions were updated to refusal without removing the later safety assertions. A subsequent normal/race run failed the new state mutation because `changed` violates SQLite's state CHECK; the fixture now changes reserved to valid owned. No production behavior was changed to address these fixture failures. CLI and agentclient passed those intermediate runs.

## Final verification

Each command below exited 0. Both normal and race commands used `-count=1 -json -timeout=180s`; counts are actual test pass events, excluding package completion events. No skip or fail events.

From `source/server`, normal and with `-race`:

```
go test ./internal/server ./pkg/agentclient -run 'Test(DestinationResume|Resume|LiveSourceRecovery|LiveRecovery|ExactRecovery|RecoveryUncertain|Lifecycle|Takeover|Attachment|ConversationAttachment|ForegroundAttachment|LocalAttachment|DialExisting)' -count=1 -json -timeout=180s
```

Each run: server **32 top-level / 109 total test events**; agentclient **13 top-level / 18 total test events**. Includes ordinary/legacy compatibility, consent/refusal, recovery uncertainty/lookup failures, interrupted destination retries, retirement, attachment takeover and authenticated lifecycle transport controls.

From `source/clients/cli`, normal and with `-race`:

```
go test ./internal/ui -run 'Test(Resume|LiveSourceRecoveryUI|RecoveryUI|Takeover|Enrollment|TaskObservation|InheritedForeground|ConnRecovery)' -count=1 -json -timeout=180s
```

Each run: **27 top-level / 52 total test events**. Includes loopback destination RPC integration, legacy-offer refusal, modern default-no UI consent and terminal attachment/reconnect paths.

Both module compile gates: `go test ./... -run '^$' -count=1` from `source/server` and `source/clients/cli`, each exit 0. These intentionally run zero tests and are compile/interface gates only, not full-suite execution.

`git diff --check` passed. Source search found no `resumeConfirmedLegacy`, `validateLegacyApproval` or `LegacyStop.Stop` calls. The sole literal `ConfirmLegacyStop: true` is the refusal regression. No schema/generated wire changes. Focused tests and compilation do not qualify native platform containment or the exact historical incident binary; no historical acceptance/VM run was performed in Phase 3. No architectural surprise required a scope change; Phase 4 remains pending.
