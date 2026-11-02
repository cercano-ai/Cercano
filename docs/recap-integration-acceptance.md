# Recap production integration acceptance

The bounded `TestProductionRecapTaskCompletion` uses the shared ordinary production service composition, server-owned prepared task manager, registered dispatch capability, prepared dispatch scope, production recap scheduler, and real temporary SQLite task/result persistence. Providers are in-memory mocks. A channel gate holds recap inference while the test checks that neither terminal completion nor child ownership release has occurred; this is not a sequential route-resolver-only test.

Configured Dispatch goes to Primary; configured Recap explicitly overrides the new Secondary/economy default to Local. The mock cloud primary result survives in persisted `dispatch.Result`. Four cases cover auxiliary inference failure (completed, child ownership released, no recap persisted), actual SQL recap-update failure via trigger (failed, ownership retained), uncertain callback persistence failure (failed, ownership retained), and genuine primary executor failure (failed with original error). Production emits the auxiliary retry warning; the focused recap tests additionally assert `ScheduledWork.Warning`. The integration does not expose that internal work object or persist the warning on the task record.

Completion-turn notification admission is deliberately disabled: this checks the child dispatch completion closure, not subsequent parent model reply delivery. A pinned worker policy installs production dispatch scoping but no worker executable or model is launched. HOME/config/store paths are temporary; credentials are disabled and Go dependencies are offline (`GOPROXY=off GOSUMDB=off`, preexisting module/build caches).

## Verification (2026-11-02)

Passed:

```
go test ./cmd/cercano -run '^TestProductionRecapTaskCompletion$' -count=1
go test -race ./internal/recap ./internal/taskrun ./internal/hostsvc/persistence ./pkg/config -count=1
go test -race ./cmd/cercano -run 'TestProductionRecapTaskCompletion|TestProductionPreparedSchedulingOwnership|TestRecapRoutePrecedence|TestRecapRequestUsesCurrentRoute' -count=1
```

The focused non-race integration passed in 0.660s. The race package results were respectively 1.591s, 4.570s, 4.226s, 8.419s, and 4.746s for the command package selection. No races were reported. `git diff --check` passed.

Regression demonstration used an isolated disposable archive of `1eb8ff57af61e06e344763d90c9194bf60030d99`, plus the new integration test. Removing only the auxiliary-warning early-return branch from `owned_schedule.go` reproduced the old failure policy: `/inference` failed with persisted status `failed`, `recap inference unavailable`, and `retained sessions or work prevent transition`, while retaining the successful cloud text. Restoring only that corrected production file made the identical test pass (0.578s). This is a targeted policy reversion, not a full historical-tree compatibility claim. The disposable reversion also removed the now-unused `os` import; its initial compile failure before that cleanup is not regression evidence.

## Accounting boundary

Child lifecycle accounting is checked through durable ownership release (or retention for unsafe writes), not physical provider billing. `internal/usage/attempt.go` describes identity/revision deduplication and explicitly notes that final token counts may be unknown and SDK retries require instrumentation inside the retry loop. These mocks do not measure physical calls, tokens, SDK retries, or exactly-once billing; no such verification is claimed. No live provider/runtime, full-suite, launcher parity, or parent completion-turn delivery certification is implied.
