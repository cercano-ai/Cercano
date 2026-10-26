# Final integration gate — 2026-10-26

Result: **FAILED / terminal acceptance blocked. Do not land on this evidence.**

Tested feature HEAD: `f8524cac4a27713ca825b5786a05b99e4d961087`.
Read-only preflight reported local main `361e38879209b84703cd56e331a0bd80100589a4`, with 0 main-only and 45 feature-only commits. No fetch, reconcile, merge, push, or installation change was performed. Existing plan.md checkbox edit remains excluded from this record's checkpoint.

## Regression execution

Delegated runner executed both full Go module suites, without package filters or -short, with isolated HOME/XDG/TMP, clean environment, CERCANO_DISABLE_CREDENTIALS=1, GOPROXY=off and GOSUMDB=off. Existing Go dependency and build caches were retained. Raw logs, exact runner/environment and results: `/tmp/cercano-regression.R7Laxs/` (temporary artifacts, not committed).

| Module | Command | Result |
| --- | --- | --- |
| source/server | go test ./... -count=1 | FAIL; 103 passing / 13 failing packages, 627.25 seconds |
| source/clients/cli | go test ./... -count=1 | FAIL; 9 passing / 1 failing packages |
| source/server | go build -o /tmp/cercano-regression.R7Laxs/bin/cercano ./cmd/cercano/ | PASS |
| source/clients/cli | go build -o /tmp/cercano-regression.R7Laxs/bin/cercano-cli . | PASS |

Observed failure classes, none waived:
- internal/mcp test mocks fail compilation: proto.AgentClient now requires CheckRetainedSession. Independent of credential isolation.
- Auth, cloud-provider, config and worker tests report credentials disabled.
- CLI UI and pkg/agentclient fixtures lack GetCredentialPolicy or the expected registered service.
- TestClosingOldLoginDoesNotCancelNewOne ignores BeginLogin failures then closes nil attempts, producing a panic.
- TestClaudeLoginCompletesFromPastedCode times out after ten minutes while receiving from a nil channel; remaining server-package tests did not complete.
- Other provider-classification, fallback-routing, metadata and tool-availability assertions failed. This run does not distinguish independent regressions from downstream credential-disable effects for every assertion.

No offline dependency-resolution failure observed. Restricted PATH caused built-in missing-tool skips for Cargo/npm/.NET validator integrations. Live-provider tests remained opt-in. See server-test.log, cli-test.log, failure-index.txt, server-failure-details.txt, and optional-validator-gates.txt in the artifact directory.

## Actual-terminal acceptance

Blocked, not passed. An offline tagged CLI build and actual PTY help invocation succeeded. Evidence is retained at `.terminal-ab-audit.5OklQb/` in the feature worktree, including commands.log and cli-help-complete.pty. No prepared hosts were launched by this terminal probe.

Concrete missing integration:
- CLI main.go uses auto-launching agentclient.Dial against localhost/config port, with no explicit authenticated pinned-host startup path.
- Existing pinned-session and lifecycle client APIs are not wired into terminal startup/commands.
- dev-host fixture gates exclude required TUI history/config, attachment, task-subscription and viewport-first resume operations; tagged streaming allows only StreamProcessRequest.

Consequently foreground A/B operation, physical terminal disconnect survival, B startup, unrelated-conversation handover, busy-transfer refusal, completion, reconnect, and idle A retirement while B remains usable were NOT demonstrated through an actual terminal. Earlier direct-RPC and after-EOF cancellation tests are not substitutes.

## Limitations and disposition

Synthetic runtime hold accounting does not establish real loaded weights/model inference acceptance or compiled-host retirement with a real loaded model. No whole-suite green claim, manual terminal acceptance, or landed-build smoke result is made.

Conditional landing criteria were not met. Main was not advanced. Feature worktree retained with pre-existing plan.md edit and terminal evidence; no force cleanup. Fixing terminal wiring and regression fixtures, rerunning this gate, and only then reconsidering reconciliation/landing remain necessary.
