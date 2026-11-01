# Combined production integration — 2026-10-31

Integrated launcher fe4afb80df22a410181bbe276c8fb9bdc1dcf06c, startup parity ffd2bd1bef6855d8d0dbf0275cd9473f5ab98ca4, and lifecycle d2040442317fa552a7c50494db26bef80f1f33f1 against main 87f4fe8d7dce096f3d29b393be295e51aa42d6bd without textual conflicts.

Review: startup hydration remains behind lifecycle acquisition; metadata is fenced by client/generation/conversation/title revision. Pinned stream registration is acknowledged only after enrollment; clients wait for the explicit header and cancellation interrupts that wait. Discovery unavailability is not proof of owner death: durable ownership/locks still gate acquisition, and malformed/authentication failures remain refusals. No crashed-owner recovery policy added.

Combined gate exposed a stale launcher-resume mock lacking the now-required response identity. Updated only the mock. Initial PTY refusal test also inherited development config authority from the environment; rerunning with development environment removed passed without production changes.

## Evidence

All final commands passed with disposable HOME/UI/config paths, credentials disabled, development environment variables removed, synthetic services and test-owned processes:

- CLI: `go test ./... -count=1`.
- Server: `go test ./internal/broker ./internal/server ./pkg/agentclient ./cmd/cercano -count=1`.
- Same server packages with `-race -run 'Test.*(Terminal|Drain|Attachment|Pinned|Session|ProductionResume|OrdinaryGeneration)' -count=1`.
- CLI: `go test -race ./... -run 'Test.*(CtrlCProgramShutdown|ReopenProduction|FindReopenOwner|DevHost|Startup|Resume|Title|Quit|Close|Release|Attachment)' -count=1`, with `CERCANO_RESUME_PROBE_BINARY` pointing to the disposable compiled server test binary.
- CLI UI: opt-in production bridge `-run 'Test(ReopenProduction|FindReopenOwner|DevHostOrdinaryReopen)' -count=3 -v` with that binary.
- Server command: `go test ./cmd/cercano -run '^TestProductionResume' -count=30` (opt-in manifest bridge skips here; executed via the CLI bridge above).
- Python launcher compatibility/progress/arguments/feedback/safety suites: 3 + 9 + 4 + 5 + 21 tests.

Local detailed logs retained in root `.combined-production-verification/` (not committed).

## Investigation disposition and acceptance limit

Carried forward only `ctrl_c_shutdown_test.go` from conversation-resume-close: synthetic gRPC + real Bubble Tea confirmed/unconfirmed Ctrl-C regression. Other broker attachment probes duplicate stronger production ownership/attachment tests; their old reports describe pre-fix behavior. Retired-host investigation is superseded by track B's production bridge. Title reducer probe deliberately asserts old title loss and does not execute commands; track A's transport/metadata tests supersede it. Investigation worktrees are retained, not blindly merged.

**Full ordinary launcher + compiled CLI A/B acceptance is NOT established.** No single harness here drives A creating a titled conversation, B starting a full second application, confirmed Ctrl-C on A, work drainage/retirement, and `-r` title/history/subsequent inference with an irrelevant stale endpoint. The bridge has ordinary production A but lifecycle-only B/minimal reload; the UI transport tests have synthetic peers; the Ctrl-C test is synthetic; launcher suites mock publication/host dependencies. Joining them into a safe synthetic-provider full application harness is additional work, not evidence supplied by these component gates. No claim of end-to-end acceptance or live model verification is made.
