# Recovery cleanup verification — Phase 1

## Baseline and scope

- Execution baseline: `76061bbe1382f2f53737e41ca39b2b45b9ed92a8`, branch `feat/recovery-cleanup`; initial `git status --short` was empty.
- Read root `AGENTS.md` (the only AGENTS.md found), approved plan and spec. Spec's proposed-status text is historical; execution authorization is the user-approved plan, Phase 1 only.
- Preserve unrelated/untracked artifacts, historical verification records and production recovery code. Explicit-path checkpoint only; no merge/push, hosts, user state, credentials, VMs, or native historical acceptance. Disposable Go fixture tests only for runner changes.

## Consumer map (execution baseline)

Reference searches across `source`, `efforts`, and `docs` found:

- Legacy confirmation: CLI `internal/ui/forced_resume.go:59` sends `ConfirmLegacyStop`; server `internal/server/resume.go:220` dispatches to `resumeConfirmedLegacy` (`resume_legacy.go:50`), which validates durable approval then calls `LegacyStop.Stop` at line 81. Characterization consumers remain in server `resume_confirmation_test.go`, `resume_test.go`, and CLI `destination_resume_test.go`.
- Stop interfaces: `resume_legacy.go:22-25` defines `LegacyProcessStop` (`Prepare`, `Stop`); `coexistence.go:36` injects it. `resume.go:442-443` prepares legacy offers. `live_recovery.go:20-21,82` adds/uses `ContainedSourceStop.WithStoppedSource`. Production adapter `cmd/cercano/development_legacy.go:98,120,183` implements both, with authenticated shutdown in `development_legacy_shutdown.go`; adapter, live recovery and historical runtime tests are active consumers. Neither interface is orphaned.
- Envelope: `source/proto/agent.proto:2811,2852` and generated `pkg/proto/agent.pb.go` retain `LifecycleOriginalRecovery`; server `live_recovery.go:138-156` creates/stores it; CLI `forced_resume.go:14` and `original_recovery.go:14-83` validate/present/confirm it. UI original/cooperative tests and historical coordinator tests exercise it. No wire change in Phase 1.
- `reaffirmResumeWithSync`: only definition/comment at `internal/server/resume.go:99-103`; no callers found. `ReserveTakeover`: implementation at `internal/conversation/ownership_takeover.go:20`, callers exclusively in `ownership_takeover_test.go` (five top-level tests). Both deliberately retained for Phase 2 safety-assertion review; active durability barriers unchanged.
- Orphan `fixture/run.sh` alone copies `oldmain.go.txt` and selects `^TestActualOldRecoveryCharacterization$`; no such test definition remains. Other references are historical verification prose, not active callers. Preserve that prose and scaffolding as explicitly inert archive evidence.


## Supported historical entry point (inspected, not executed)

`OLD_RECOVERY_PRODUCTION_FIXTURE=1 OLD_RECOVERY_EXECUTE=1 sh efforts/old-host-compatible-recovery/fixture/build-production.sh` is the current acceptance command (historical closeout corroborates it). It archives `d7cbdeb21c44e0ceeefdf212203ed04a37d09150`, builds with `developmentfixture cercano_development_acceptance`, adapts loopback inference and builds the current shipping executable. Its baseline exact selection was `go test ./cmd/cercano -run '^TestHistoricalProductionController$' -count=1 -v -timeout=180s`. `native_controller_test.go:46-53` defines subtests `drain`, `reserved`, `blocked-timeout`; missing `OLD_RECOVERY_PRODUCTION_BINARY` skips the parent. Build-only opt-in does not claim test execution. The updated runner requires successful, unskipped completion of all four exact names via JSON events.

Environment: Go 1.26.3 darwin/arm64. Historical native acceptance is intentionally not invoked for this runner slice.

## Reproduction, red/green regression and changes

Before modifying either runner, created a disposable `.runner-baseline.XXXXXX` module (`module runnerfixture`, Go 1.26.0) with only `TestDisposable`, then executed:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test . -run '^TestActualOldRecoveryCharacterization$' -count=1 -v -timeout=45s
```

Observed `testing: warning: no tests to run`, `PASS`, `[no tests to run]`, **exit 0** (0.414s). The temporary directory was trap-cleaned. This reproduces the orphan's selection/exit-status defect without building or launching its historical executable; the entire obsolete native composition command was intentionally not run.

Archived original `run.sh` as `fixture/archive/run.sh.txt` and moved `oldmain.go.txt` to `fixture/archive/oldmain.go.txt`. Compared both archive byte streams against `git show HEAD:<original-path>`: identical. `run.sh` is now only a refusal tombstone (exit 2); historical verification records are unchanged. New fixture README identifies the supported entry point and limits.

Added disposable Go fixture regressions in `fixture/test_checked_go_test.py`. First ran them against a temporary permissive implementation of `checked-go-test.py` that delegated directly to Go's exit status (same baseline contract). Exact command:

```sh
python3 -m unittest discover -s efforts/old-host-compatible-recovery/fixture -p 'test_*.py' -v
```

**RED:** 8 tests, 6 failures, 3.371s command time. Missing selection, unrelated pass, missing child, skipped parent and skipped child all incorrectly returned 0; valid-execution test also failed its required verification-message assertion. Failed-command propagation and archive refusal already passed.

Replaced that implementation with JSON run/pass validation and wired `build-production.sh` to it. Acceptance now invokes `go test ./cmd/cercano -run '^TestHistoricalProductionController$' -count=1 -json -timeout=180s`, requiring the parent and all three exact child names. Nonzero Go status remains failure; any skip/fail or malformed JSON is rejected. Package-level PASS or unrelated tests cannot satisfy intended execution. Output is captured and printed after completion (not streamed), a runner-only diagnostic tradeoff.

**GREEN:**

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s efforts/old-host-compatible-recovery/fixture -p 'test_*.py' -v
```

8/8 PASS, 3.257s suite (3.394s combined tool command). Exact Python cases: `test_missing_selection`, `test_unrelated_pass_does_not_satisfy_selection`, `test_skipped_parent`, `test_missing_child`, `test_skipped_child`, `test_failed_command`, `test_nonzero_intended_execution`, `test_archived_entrypoint_refuses`. Seven cases invoke real Go commands in disposable standard-library-only modules with `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS='' GOWORK=off`; each selects the historical parent regex except the deliberate unrelated-selection case. Valid execution confirms four intended run/pass names; rejection cases assert nonzero status (and selection diagnostic except Go failure).

Additional checks:

- `sh efforts/old-host-compatible-recovery/fixture/run.sh`: exit 2 and actionable archive guidance, before any build/host launch.
- `sh -n efforts/old-host-compatible-recovery/fixture/build-production.sh efforts/old-host-compatible-recovery/fixture/run.sh`: PASS.
- `git diff --check`: PASS.
- Scoped diff review: no `source/` edits, no wire edits, only Phase 1 statuses changed; archived originals byte-identical. Phase 2 and later remain pending.

## Limitations and checkpoint scope

No historical production acceptance, native host/worker launch, recovery behavior suite, race suite, module-wide compilation, live-host/user-state/VM action, merge or push was performed. Runner regressions establish selection integrity, not ownership/containment correctness or deployed-binary qualification. The production wrapper wiring was source-inspected and shell-syntax checked, not exercised end-to-end; final authorized native acceptance remains Phase 6. Malformed-JSON refusal is implemented but not independently fault-injected by this fixture suite. Production APIs, unused-helper deletion and safety-assertion migration remain untouched for Phase 2+. Python 3 is already a fixture dependency and is now also used for acceptance event validation.

Checkpoint scope: this verification record, Phase 1 plan statuses, fixture README, production wrapper, refusal tombstone, checked runner and its regression tests, and the two archived original text files (including old-path removal for the moved `oldmain.go.txt`). SHA is reported after the scoped checkpoint rather than self-referenced in this file.
