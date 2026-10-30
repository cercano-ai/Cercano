# Ordinary CLI closure and reopen — offline evidence (2026-10-29)

## Result and expected lifecycle

PASS for explicit closure using the existing user-facing TUI commands, and for
quiescent reopen on B with history. No production code or lifecycle architecture
changed. `/quit` is **not** session closure: it deliberately retains ownership.

Before execution, the expectation was that B `/resume <A-conversation>` would
transfer a quiescent conversation and display history, whereas A `/quit` would
leave A alive. The first PTY probe confirmed both. Source inspection then identified
the documented explicit closure sequence `/host detach` followed by `/host release`;
the second run exercised that sequence to allow automatic drained A retirement.

Exact paths:
- `source/clients/cli/internal/ui/model.go:3320-3321`: `/quit` returns `tea.Quit`.
- `source/clients/cli/internal/ui/dev_host.go:138-165`: UI help states exit retains
  ownership; detach closes attachment and retains the session; management requires detach.
- `source/clients/cli/internal/ui/dev_host.go:76-77,174-181`: UI release invokes the
  existing lifecycle release. This is an explicit management command in the ordinary
  TUI, not a newly invented close operation or a fixture-only control invocation.
- `source/clients/cli/internal/ui/model.go:4170`: ordinary resume enters host reopen.

## Commands and observations

Run from the integration worktree with
`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```sh
python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo . --evidence .ordinary-closure-evidence --ordinary-closure-probe
python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo . --evidence .ordinary-session-close-evidence --ordinary-session-close
```

Both exited 0. The first is a successful **retention probe**, not a retirement
acceptance pass: after `/quit` exited 0, A remained alive and its session was still
listed after five seconds. Its report explicitly labels ordinary quit closure blocked.

The second used actual shell launcher, publisher, compiled ordinary host, compiled
CLI PTYs and workers. After independent A work completed, B `/resume` displayed
`FIXTURE_ORIGINAL_HISTORY`, acquired the original conversation, and persisted
`FIXTURE_REOPENED_ON_B` in that same conversation. A remained alive for its other
retained session. The remaining A TUI received `/host detach`, then `/host release`,
and displayed completion. A exited automatically without an explicit retire call;
B subsequently persisted a fresh user turn and assistant reply. The only out-of-band
lifecycle helper actions in either run were read-only `list` observations: no helper
release, transfer, or retire substituted for the user-facing actions.

Focused checks (CLI module, same guards):

```sh
go test ./internal/ui ./internal/slash -run 'TestDevHostOrdinaryReopen|TestFindReopenOwner|TestRegistry_DispatchAlias' -count=1 -timeout=90s
```

PASS. An earlier invocation used `TestQuit` instead of `TestRegistry_DispatchAlias`;
UI tests passed but slash reported no matching tests, hence the corrected invocation.

## Artifacts, provenance and cleanup

Baseline branch `feat/production-generation-integration`, HEAD
`1616fead8d2b446ba67e261bbebf2fc8b8566572`. Both builds recorded source tree
`694c000f4242cdfec3c8d5cbd8c85226ff030ce5`; final isolated snapshot commit
`11a338bc034e7ce687abbf0dc44524c3dd600664`. A/B manifests, executable hashes,
provider/worker records, PTY captures and synthetic persisted rows are retained locally.

- Retention probe: `.ordinary-closure-evidence/compiled.json`, SHA-256
  `3052bd74fce16d1e17c64988d00bc54ada0cc17e88806f783aca8bba780d6c9c`;
  private fixture `/private/tmp/ca-u8q76hpu`.
- Closure acceptance: `.ordinary-session-close-evidence/compiled.json`, SHA-256
  `4c66a60fce4e7711bb4cfe25869ab3f39d0a750e08c551447788d9b5d225414e`;
  private fixture `/private/tmp/ca-yehahv0l`.
- Particularly relevant captures: `A-unstarted.pty`,
  `B-restarted.pty`, and `synthetic-reopened-rows.json` in each evidence directory.

Both cleanup reports have `remaining=[]`; cleanup targeted only recorded disposable
fixture processes. Private fixture directories and integration worktree remain for
review. Tokens were not exported (only hashes). Pre-existing dirty plan and all
unrelated untracked artifacts were preserved and excluded from the checkpoint.

Limitations: tagged synthetic provider/MCP fixtures, disabled credentials, isolated
HOME/configuration/history/runtime, CGO disabled, and offline Go dependency lookup.
Not live-provider, native-runtime, existing-store, or production-cutover acceptance.
The separate credential/runtime/store audit was not duplicated. If “ordinary closure”
is intended to mean `/quit` alone rather than the documented explicit TUI release,
that narrower requirement remains unavailable: changing retention semantics would
be a substantive product decision, not part of this task. No such change was made.
