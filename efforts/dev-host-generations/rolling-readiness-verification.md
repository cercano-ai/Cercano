# Rolling readiness follow-up (2026-11-03)

Status: **review branch only; not landed**. Branch `fix/rolling-readiness`, based on main `d7cbdeb21c44e0ceeefdf212203ed04a37d09150`. This is distinct from the already-landed launch-receipt implementation described in [failed-launch ownership](failed-launch-ownership-decision.md).

## Reproduction and root causes

The pre-fix terminal PTY failed waiting for `durable fixture history` after `/host reconnect`. Startup hydration reused `resumeRequestedMsg`, the history-picker's explicit reopen action. After reconnect to retained A with B still the default selection, that action dispatched a lifecycle reopen against B instead of reading the already-attached A session. The diagnostic reopen trace showed selected A and default B; the mismatch, not the fixture's persisted history, explained the missing viewport. The startup path now uses an internal hydration message and the attached client. Explicit user reopens still use destination lifecycle routing; detached owners are not hydrated. Regression tests preserve both distinctions, and the existing PTY's history and stale-owner assertions remain intact.

The pre-fix refusal PTY exited 78 for inherited config authority before reaching its asserted `injected publication failure`. A run without caller `CERCANO_*` launch authority passed, confirming fixture contamination rather than a publisher failure. Both disposable launcher harnesses now remove inherited `CERCANO_*` values before establishing their private environment. The refusal Go test deliberately supplies caller launch/config authority, so isolation is exercised even in a clean CI shell. The exit-code and injected-failure assertions are unchanged.

## Bounded integration smoke

`TestEverydayLauncherHistorySmoke` reuses `everyday-launcher-compiled.py --history-smoke` with an eight-minute subprocess deadline. It runs the ordinary installed-command shape, real publisher, tagged compiled server and CLI PTYs, with synthetic provider/worker dependencies in disposable HOME/config/runtime/store state:

1. A generates and durably stores conversation history through the CLI.
2. A source change produces distinct B; the published current target must identify B.
3. Ordinary `/quit` closes A's terminals; A must have retired before resume.
4. B's CLI `/resume` displays A's original history without a takeover prompt, and ownership is B.
5. A new prompt persists under the original conversation; recorded inference must not use A, and the conversation request must contain original history. Supervisor requests are not conversation-history requests.

This is stronger than merely observing B selected while A could still serve the resumed conversation. It does not exercise live-owner takeover or the broader task/worker acceptance journey.

## Final local verification

All commands below passed on the macOS development host in the isolated worktree. Server commands run from `source/server`; CLI commands from `source/clients/cli`.

Server:

```sh
go test ./cmd/cercano -run '^Test(DevHostTerminalPTY|EverydayLauncherEntrypointRefusalsPTY)$' -count=3
CERCANO_RUN_LAUNCHER_HISTORY_SMOKE=1 go test ./cmd/cercano -run '^TestEverydayLauncherHistorySmoke$' -count=1 -timeout=9m
go test ./... -count=1
go test -race ./internal/server -run 'LaunchReceipt|LifecycleControl|Retained' -count=1
go build -o /tmp/cercano-rolling-readiness-server ./cmd/cercano/
python3 scripts/test_launcher_compatibility.py
python3 scripts/test_launcher_progress.py
python3 scripts/test-launcher-arguments.py
python3 scripts/test-launcher-feedback.py
bash scripts/cercano-launcher_test.sh
bash scripts/install-launcher_test.sh
```

The launcher suites reported 3/9/4/5/22/2 passing tests respectively; `cercano-launcher_test.sh` invokes the safety suite, so no duplicate invocation is needed. Safety-fixture output included expected temporary-bootstrap cleanup warnings with scratch directories retained; no unknown-file cleanup was performed.

CLI:

```sh
go test ./... -count=1
go test -race ./internal/ui -run 'DevHost|LaunchDelivery|LaunchReceipt|TabVisibilityAcrossResumeEntryPoints' -count=1
go build -o /tmp/cercano-rolling-readiness-cli .
```

`git diff --check` passed. An initial CLI build invocation incorrectly used nonexistent `./cmd/cercano-cli/`; the corrected module-root build above passed.

CI now explicitly runs the bounded history smoke and launcher fixture suites in the existing server job, downloads CLI fixture dependencies before offline nested builds, and runs CLI Go tests in the existing CLI end-to-end job. The existing server full-suite step includes both PTY regressions. Workflow execution on GitHub/Linux has not been observed locally.

## Limits and review boundary

No real providers, model runtime, production process operations, signals, database/config migrations, installed cutover, or cross-platform changes. Full server/CLI Go suites ran, but not the separate broad compiled acceptance modes, npm terminal suite, or full-repository race suite. Races were scoped as shown. The new smoke intentionally covers released history, not live work transfer. No new architectural choice was needed; existing explicit ownership/reopen semantics remain unchanged. No push, fetch, merge or land. Unrelated/untracked main-worktree files, including `source/server/internal/server/probe_wake_persist_test.go`, remain outside this branch's changes. Independent review is still required.
