# Closeout verification — 3ca80f4e049e

Production code tip: `3ca80f4e049ede828a161bdfa8a9ccf655572f3e`, branch `feat/old-host-compatible-recovery`. This closeout changes documentation/status only. Host: darwin/arm64, Go 1.26.3. All Go checks below used `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`. No broad runtime suites repeated: parent reports full conversation/server normal/race already passed at this tip; that report is not a new execution here.

## Executed final checks

From repository root:

```sh
OLD_RECOVERY_PRODUCTION_FIXTURE=1 OLD_RECOVERY_EXECUTE=1 sh efforts/old-host-compatible-recovery/fixture/build-production.sh
```

PASS, 3/3 historical subtests, zero skips: owned/drain 14.52s, reserved 14.11s, blocked-timeout 13.21s (41.84s total). Log: `/tmp/cercano-closeout-historical.log`. Built pinned historical `d7cbdeb21c44e0ceeefdf212203ed04a37d09150` offline with `developmentfixture,cercano_development_acceptance` and loopback backend adaptation; executable SHA256 `72c741f6e6c22a206ed5cbde9e6b09385bdb39e418af1621644a19695f53b5f2`. Current destination built from the code tip above. This is actual historical host/worker execution, not a current RPC stub and not the dirty deployed incident binary.

Both successful cases exercised multiple failed journals, rejected superseded consent, and recovered with fresh exact-owner consent. `native_controller_test.go:669-710` requires preserved complete prefix, exact drained rows where applicable, exactly two new persisted rows with exact roles/content, exactly one correct final and no recovered stream errors, and exactly one additional provider request. Blocked drain refuses without calling recovery; subsequent disposable cleanup verifies dead host/worker, competing historical launcher exclusion and unchanged store/WAL (`historical_runtime_test.go:25-90`). The old held stream reports `unknown or stale ownership or ticket`; this is not presented as a clean old-client stream.

From `source/server`:

```sh
go test ./... -run '^$'
go test -c -o /tmp/cercano-closeout-probe ./cmd/cercano
go test -race -c -o /tmp/cercano-closeout-probe-race ./cmd/cercano
go test ./internal/server -run 'Test(ResumeOrdinaryBeforeContainedRecovery|ResumeReadyRetainsCompletedOperation|ResumeRecoveryLookupFailure|LiveSourceRecovery)$' -count=1
go test -race ./internal/server -run 'Test(ResumeOrdinaryBeforeContainedRecovery|ResumeReadyRetainsCompletedOperation|ResumeRecoveryLookupFailure|LiveSourceRecovery)$' -count=1
```

All PASS. Four selected top-level behavioral tests per mode; compilation intentionally selects no behavioral tests. Native interface log: `/tmp/cercano-closeout-server-compile.log`.

From `source/clients/cli`:

```sh
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-closeout-probe go test -json ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-closeout-probe-race go test -race -json ./internal/ui -run 'Test.*(Recovery|Recover|Resume|Forced|Confirm)' -count=1
go test ./... -run '^$'
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
```

All PASS. JSON counted **113 top-level passes, zero failures/skips in each mode**, including actual-process-exit bridge. Logs `/tmp/cercano-closeout-cli-{normal,race}.json`, `/tmp/cercano-closeout-cli-{compile,linux,windows}.log`. Linux/Windows CLI cross-builds are compilation only. Earlier server command cross-builds at `9a879b345ee8` are documented in `operation-id-verification.md`; no non-Darwin runtime validation is claimed.

## Phase 4 reconciliation and addressed findings

- Ordinary acquisition, exact reservation claim and supported cooperation precede contained recovery (`source/server/internal/server/resume.go:188-218`). Confirmation continues only the bound attempt. The four focused tests above verify ordering/no unnecessary stops, ready operation metadata, response-loss identity and fail-closed receipt lookup.
- Optional discovery RPC families are removed from the service; retained lifecycle controls support cooperation. Failure flows into explicit qualified containment rather than requiring new source RPCs. `live_recovery.go:25-33` reads durable ownership directly; `:42-103` revalidates full consent and contains before reassignment. The actual historical acceptance verifies the compatible route. No journal-count discovery prerequisite remains on this route; journals still serve cooperative evidence. This does **not** relax ambiguity checks on current recovery receipts: tip `3ca80f4e` rejects ambiguous/unavailable receipt lookup without ready result, fallback or mutation (`resume.go:243-254`, `TestResumeRecoveryLookupFailure`).
- Normal contained reopen uses `source/clients/cli/internal/ui/original_recovery.go:28-83`: one whole-host confirmation, single-use challenge, cancellation default, explicit uncertain external effects/stuck-drain disclosure and exact ready-result validation. Legacy compatibility handling remains, not a second discovery/repair journey. UI normal/race coverage passes.
- Previously reproduced ordering/ready-metadata findings and fixes are recorded in `review-fix-evidence.md`; response-loss identity in `operation-id-verification.md`; attribution, uncertain commit and notification/non-replay boundaries in `bounded-gap-verification.md`. This run verifies the current composed candidate, not a new independent review.

Phase 4 remaining tasks are now done on this code/test evidence. Phase 5 test/build items are done with the platform limits above. Independent final reviewer is running separately: its outcome is not known here. Review, final finding-repair closure and overall report tasks remain for the parent; no invented approval.

## Teardown, prerequisites and limits

All three historical cases logged `owned process cleanup complete (no signals)`. Post-run `ps` found none of owned PIDs 57878/57882, 57940/57944, 57964/57968, no process referencing fixture roots `/private/tmp/nc-1957707324`, `nc-982111437`, `nc-2727742918`, and no bridge probe processes. No `.old-recovery-production.*` directory remained. Tests wait bridge children; local `/tmp` logs/binaries are inert evidence.

Runtime support is bounded to same-user Darwin prepared hosts with qualified attribution, writer contract and retained launch exclusion. Non-Darwin/native-without-cgo recovery remains unsupported and refuses (`source/server/internal/brewrestart/writers_unsupported.go:1-11`, `dead_host_unsupported.go:1-11`), regardless of cross-build success. Stuck drain/uncertain containment fail closed. Interrupted external actions are not replayed; uncertain external effects are not undone. Later authorized completion notification is not interrupted-action replay.

No live hosts/configuration/VMs changed, no live recovery, no push or merge. Deployment requires separately qualified source provenance/process/store contracts and fresh consent. Worktree retained; pre-existing untracked `.tmp-old-recovery/` preserved. Checkpoint scope is exactly this report and `plan.md`; final docs commit SHA and tracked-clean status are reported after checkpoint rather than self-referenced here.
