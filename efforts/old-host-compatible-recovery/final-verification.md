# Final verification — cleaned candidate 15bb32cae306

## Scope and outcome

Verified on darwin/arm64 with PATH Go `/opt/homebrew/bin/go`, go1.26.3. Starting branch `feat/old-host-compatible-recovery`, HEAD `15bb32cae306320775672281a8e880ba268b991f`; initial tracked tree clean, only pre-existing untracked `.tmp-old-recovery/`. Read approved spec/plan, review-fix evidence and Phase 4 checkpoint. Earlier checkpoint limitations are historical, not a description of the protocol cleanup now present.

**Bounded verification complete; full approved plan incomplete.** Actual historical acceptance passes on the cleaned production candidate. No production code changed. One gated CLI fixture retained an obsolete confirmation expectation after deletion of the takeover discovery route; reproduced failure in normal/race, corrected only that test, reran affected checks successfully. This is not live conversation recovery.

## Executed commands and results

All Go commands used `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` (the historical script exports these itself). Commands below were executed, not just proposed. Logs are local `/tmp` artifacts, not required tracked inputs.

From repository root:

```sh
OLD_RECOVERY_PRODUCTION_FIXTURE=1 OLD_RECOVERY_EXECUTE=1 sh efforts/old-host-compatible-recovery/fixture/build-production.sh
```

PASS: `TestHistoricalProductionController`, 3/3 subtests, zero skips: owned/drain 14.29s, reserved 13.81s, blocked-timeout 13.12s (41.21s total). Log `/tmp/cercano-final-historical.log`. Script archives pinned historical production implementation `d7cbdeb21c44e0ceeefdf212203ed04a37d09150`, builds offline with `developmentfixture,cercano_development_acceptance` and loopback provider adaptation, and builds the cleaned current destination. Historical executable SHA256 `1db0365c97c1a9a53152972b2a1f0c014de25b27321e615fc4044e85faaa1add`. This is actual old host/worker execution, not current RPC stubs; it is not reproduction of an arbitrary dirty incident binary.

Owned/drain and reserved both exercised multiple failed journals, superseded consent rejection and fresh exact-owner recovery. Assertions in `source/server/cmd/cercano/native_controller_test.go:669-710` preserve the full history prefix; drain additionally preserves exact held user/assistant rows; the final turn adds exactly two rows with exact role/content, exactly one correct stream final, no stream error and one additional provider request. Blocked timeout does not call the recovery callback; subsequent disposable cleanup observes dead host/worker, contending historical launcher, and unchanged store/WAL under exclusion (`historical_runtime_test.go:25-90`). Old held stream still reports `unknown or stale ownership or ticket` during drain, explicitly distinguished from the error-free recovered turn.

From `source/server`, run each behavioral command both normally and with `-race`:

```sh
go test -json ./internal/conversation ./internal/broker ./pkg/agentclient -count=1
go test -json ./internal/server ./cmd/cercano -run 'Test.*(Recovery|Recover|Resume|Ownership|Admission|NativeControllerAssertions|DevelopmentRecoveryRegistryExclusion|ForcedTransfer|DeadOwner|Contain|Consent)' -count=1
go test -json ./internal/brewrestart -count=1
go test ./... -run '^$'
go test -c -o /tmp/cercano-final-server-probe ./cmd/cercano
go test -race -c -o /tmp/cercano-final-server-probe-race ./cmd/cercano
```

All PASS. Top-level passes **per mode**: conversation 109; broker 42; agentclient 78; server 45; cmd 16; brewrestart 47. JSON logs `/tmp/cercano-final-{core,focused,process}-{normal,race}.json`. Server compile log `/tmp/cercano-final-server-compile.log`. Compile-only intentionally runs zero behavioral tests; no full repository runtime sweep performed.

Expected skips, individually inspected: agentclient's three subprocess helper entrypoints; brewrestart's three child-only entrypoints and explicit read-only ambient inventory opt-in (not enabled); cmd's two explicit CLI bridge entrypoints. These are not skipped historical acceptance tests. Destination bridge was separately exercised below; production dead-owner coverage runs its subprocess bridge. No unexpected skip was hidden.

From `source/clients/cli`:

```sh
# Initial normal/race selection, before bridge opt-in:
go test -json ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1
# After constructing the disposable server test binaries:
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-final-server-probe go test -json ./internal/ui -run '^TestDestinationResumeAfterActualProcessExit$' -count=1
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-final-server-probe-race go test -race -json ./internal/ui -run '^TestDestinationResumeAfterActualProcessExit$' -count=1
# Final, after minimal fixture repair:
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-final-server-probe go test -json ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-final-server-probe-race go test -race -json ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1
go test ./... -run '^$'
```

Initial selection passed 112 tests per mode and skipped gated bridge. Enabling it exposed **FAIL in both modes**: `expected explicit recovery confirmation: <nil>` at old `destination_process_test.go:59`; missing continuation was a consequence. Red logs `/tmp/cercano-final-bridge-red-{normal,race}.json`. Source inspection showed candidate deletion of `offerTakeover/originalRecovery` before the existing death-proven recovery branch. The bridge deliberately waits for exact source child exit; live-host interruption consent is not needed. Test-only repair asserts successful destination binding and stable recovered epoch on reconnect instead of obsolete takeover confirmation. Server-side bridge still verifies fresh epoch versus abandoned row and preserved history plus next persisted row. No production behavior was relaxed.

Final CLI selection **113 passed, zero skipped, zero failed in each mode**, including actual-process-exit integration. Logs `/tmp/cercano-final-cli-final-{normal,race}.json`; final module compile PASS `/tmp/cercano-final-cli-compile.log`.

## Teardown and boundaries

All three historical subtests logged `owned process cleanup complete (no signals)` from inventory-based cleanup (`native_controller_test.go:267-284`). Post-run inspection found no `.old-recovery-production.*` directories, none of logged old host/worker PIDs 51547/51550/51574/51577/51613/51617, and no processes referencing the three recorded nc fixture roots or disposable bridge binaries. Bridge children were waited; final checks found no residual fixture processes. No ambient signals, live config edits, live recovery, VM exercises, push or merge were performed. `.tmp-old-recovery/` remains untracked and untouched. Temporary test binaries/logs under `/tmp` are inert local evidence.

## Plan reconciliation and exact remaining work

Updated through semantic task status tool: Phase 1 acceptance journey complete; Phase 5 focused normal/race complete; interface/platform checks in progress; finding repairs in progress pending independent review. Existing historical acceptance and checkpoint completion retained. Other unchecked tasks are deliberately not promoted based solely on package pass counts.

- **Complete done_when in this delegation:** cleaned-candidate historical owned/drain, reserved, blocked-timeout acceptance; exact history/final/cardinality assertions; nonzero focused normal/race confirmations; server+CLI native interface compilation; gated CLI process integration; fixture teardown; evidence and explicit-path checkpoint. No real production regression found in these probes.
- **Incomplete overall done_when:** independent source review is concurrent and not available in this run. Its findings must be integrated before review/repair tasks can be closed.
- Phase 2 still needs explicit scenario-to-evidence closure for unrelated-host nonmutation and the complete cancellation/PID-reuse/surviving-writer matrix. Historical relaunch exclusion and timeout are verified here, but do not by themselves satisfy every combined task.
- Phase 3 still needs consolidated mapping/review of interrupted/uncertain external effects, descendants, uncertain commit, response loss and duplicate retries/late claims, including completion notifications versus prohibited action replay. This run verifies full committed history, completed host-owned task result, lost-response fixture retry and unchanged recovered epoch; it does not prove arbitrary external effects or every crash boundary.
- Phase 4's remaining flow/status reconciliation requires reviewer closure of durable-owner routing, optional unsupported-RPC behavior, journal independence and consolidated UX. Historical evidence demonstrates the compatible route without new source RPCs and without journal uniqueness; it is not exhaustive proof for all adapters. Protocol-removal tasks already marked complete remain so.
- Native macOS is the only runtime/compile platform exercised. Linux/Windows platform checks and VM evidence are **not** supplied; macOS process containment support must not be generalized to arbitrary platforms/binaries. Any additional supported-platform matrix remains outstanding, not a failing native test.
- No authorization to deploy/recover the user's live host. Deployment requires separately qualified process/store contracts, attribution, launch exclusion and fresh consent; incident binary provenance remains a support limitation rather than evidence of live recovery.

Checkpoint scope is exactly this report, `plan.md`, and `source/clients/cli/internal/ui/destination_process_test.go`; production candidate remains unchanged.
