# Launcher fixture and protobuf-copy CI repair verification

Base: `0328b9344fa60edf29621c5d590972c9be5fed97` (`main`).
Branch: `fix/ci-launcher-fixture-protobuf`.
Environment: macOS/Darwin arm64, Go 1.26.3, Node 25.9.0, npm 11.12.1.
These are macOS results, **not native Linux verification**. Native Linux is
unavailable in this run; CI uses Ubuntu and the enterprise matrix also uses macOS.
The local Node version differs from CI's pinned 22.13.0.

## Changes and reproduction

Only test code changed: place the feedback test state under a private temporary
parent; use deep protobuf clones for independently mutated control requests and
startup identity. Production validation and existing feedback/security assertions
are unchanged.

Before editing, from the worktree root:

```sh
TMPDIR=/tmp python3 source/server/scripts/test-launcher-feedback.py LauncherFeedback.test_database_check_has_flushed_feedback_before_inspection
```

Exit 1 reproduced `ValueError: state parent must be owned and not writable by
others` at launcher line 514. Before editing, `go vet ./...` in `source/server`
exited 1 and reproduced all four requested protobuf-copy diagnostics, as well
as the unrelated diagnostics listed below.

## Remaining blockers (not changed)

Complete server `go vet ./...` still exits 1, solely on existing unrelated issues:

- `internal/compactiongen/owned_regression_test.go:179,236`: cancel not used on all paths.
- `pkg/agentclient/attachment_test.go:143`: GetCredentialPolicy copies atomic/noCopy receiver.
- `pkg/agentclient/cloud_reauthentication_test.go:135`: same receiver-copy issue.
- `pkg/agentclient/tasks_test.go:85`: same receiver-copy issue.

The exact enterprise-client workflow vet command now passes, and full server
tests pass with their normal default vet behavior. The explicit complete vet
command is stricter. No assertions or production safeguards were weakened to
clear unrelated diagnostics.

Terminal E2E `npm test` exits 1: 11 passed, 23 failed, 1 todo. Captured screens
show `cercano: development launcher requires nonempty --dev-host; legacy fallback
refused`; the existing E2E helper starts the CLI without a development selection.
This requires a broader fixture update and is outside this repair. Build and
`npm ci` passed. Traces/screens remain under `test/e2e/artifacts/` (ignored).

## Commands and results

Inspected `.github/workflows/ci.yml`, `enterprise-client.yml`,
`enterprise-catalog.yml`, and `enterprise-contract.yml` for exact commands.
All six CI launcher fixture commands ran with `TMPDIR=/tmp`, including the shell
wrappers (22 safety tests and 2 install tests). Targeted changed-test race checks
were explicitly uncached (`-count=1`); the enterprise race command covers the full
workflow package list. Full server tests were additionally run with vet disabled
to isolate runtime results; the ordinary full suite also passed.

Commands below run relative to the indicated working directory. Exit 0 = pass.
Full local logs and the runner are retained in `.verification/` (untracked).

| Check | Working directory | Command | Exit |
| --- | --- | --- | --- |
| affected | `source/server` | `go test ./internal/server ./cmd/cercano -run TestForcedCutover&#124;TestDevelopmentProduction&#124;TestDevelopmentValidatedStartupBinding -count=1` | 0 |
| vet | `source/server` | `go vet ./...` | 1 |
| enterprise-vet | `source/server` | `go vet ./internal/enterprise ./internal/modelpolicy ./internal/managedsettings/... ./internal/managedrouting ./internal/cloudfactory ./internal/hostsvc/providers ./internal/hostsvc/persistence ./internal/contextedit ./internal/dispatch ./internal/loopcompact ./internal/loop/... ./internal/usage ./internal/capabilities/builtins ./internal/hostsvc/tools ./internal/runner ./internal/server ./internal/mcp ./internal/agent ./internal/llm/... ./internal/engine/... ./internal/localruntime/... ./internal/worker ./cmd/cercano` | 0 |
| launcher-compatibility | `source/server` | `env TMPDIR=/tmp python3 scripts/test_launcher_compatibility.py` | 0 |
| launcher-progress | `source/server` | `env TMPDIR=/tmp python3 scripts/test_launcher_progress.py` | 0 |
| launcher-arguments | `source/server` | `env TMPDIR=/tmp python3 scripts/test-launcher-arguments.py` | 0 |
| launcher-feedback | `source/server` | `env TMPDIR=/tmp python3 scripts/test-launcher-feedback.py` | 0 |
| launcher-shell | `source/server` | `env TMPDIR=/tmp bash scripts/cercano-launcher_test.sh` | 0 |
| install-shell | `source/server` | `env TMPDIR=/tmp bash scripts/install-launcher_test.sh` | 0 |
| enterprise-demo | `source/server` | `go test -tags=enterprise_demo ./internal/server -run ^$` | 0 |
| broker-race | `source/server` | `go test -race ./internal/broker` | 0 |
| enterprise-race | `source/server` | `go test -race ./internal/enterprise ./internal/modelpolicy ./internal/managedsettings/... ./internal/managedrouting ./internal/cloudfactory ./internal/hostsvc/providers ./internal/hostsvc/persistence ./internal/contextedit ./internal/dispatch ./internal/loopcompact ./internal/loop/... ./internal/usage ./internal/capabilities/builtins ./internal/hostsvc/tools ./internal/runner ./internal/server ./internal/mcp ./internal/agent ./internal/llm/... ./internal/engine/... ./internal/localruntime/... ./internal/worker ./internal/inference/... ./cmd/cercano` | 0 |
| server-full | `source/server` | `go test ./... -count=1` | 0 |
| server-full-no-vet | `source/server` | `go test -vet=off ./... -count=1` | 0 |
| server-build | `source/server` | `go build -ldflags -X main.version=ci-local -o bin/cercano ./cmd/cercano` | 0 |
| history-smoke | `source/server` | `env CERCANO_RUN_LAUNCHER_HISTORY_SMOKE=1 go test ./cmd/cercano -run ^TestEverydayLauncherHistorySmoke$ -count=1 -timeout=9m` | 0 |
| cli-download | `source/clients/cli` | `go mod download` | 0 |
| cli-full | `source/clients/cli` | `go test ./... -count=1` | 0 |
| cli-build | `source/clients/cli` | `go build -o ../../../.verification/cercano-cli .` | 0 |
| catalog-race | `source/server` | `go test -race ./internal/catalogexport ./pkg/config` | 0 |
| catalog-check | `source/server` | `go run ./cmd/export-enterprise-catalog --check` | 0 |
| contract-race | `source/enterpriseapi` | `go test -race ./...` | 0 |
| contract-vet | `source/enterpriseapi` | `go vet ./...` | 0 |
| e2e-build | `test/e2e` | `./scripts/build.sh` | 0 |
| e2e-install | `test/e2e` | `npm ci` | 0 |
| e2e-tests | `test/e2e` | `npm test` | 1 |
| affected-race | `source/server` | `go test -race ./internal/server ./cmd/cercano -run 'TestForcedCutover&#124;TestDevelopmentProduction&#124;TestDevelopmentValidatedStartupBinding' -count=1` | 0 |

Final rerun from worktree root:
`TMPDIR=/tmp python3 source/server/scripts/test-launcher-feedback.py` passed all
5 tests after the explanatory comment was added. `git diff --check` passed.

Worktree retained at `wt-ci-fixture-protobuf-fix` below the original repository
for independent review. No landing, pushing, deployment, or service restarts.
Unrelated original-worktree files were not modified.
