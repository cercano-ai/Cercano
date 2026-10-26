# CLI documentation audit

Scope: `docs/features/cli/**` and `source/clients/cli/**/*.md`, plus this requested report. No implementation files or shared audit ledger were edited.

## Disposition and coverage

The group contains 38 Markdown documents after this update: three current guides,
34 nested historical records, and the preserved original roadmap. A filesystem
search found **no Markdown files under `source/clients/cli`**.

- Replaced `docs/features/cli/README.md`'s mixed roadmap/status checklist with a
  current build/startup, command, configuration and feature guide. Preserved the
  previous contents in `original-plan.md` with an explicit historical warning.
- Updated `session-model.md` for Premium shortcuts, the session tier picker,
  direct-control versus tool-confirmation semantics, managed routing validation,
  and per-request override precedence.
- Retained the source-backed picker contract in `model-catalog-picker.md` and
  explicitly attributed its earlier PTY verification to historical feature work.
- Marked all 34 nested records historical at their entry point. This includes
  their old command examples, package paths, completion labels, proposals and
  test claims; they are not current operational instructions. Each directs the
  reader to the current guide. No historical checkbox was silently turned into
  a present-day completion claim.

### Feature-group disposition

| Group | Files covered | Current evidence / disposition |
|---|---|---|
| buffer-scrollbar | design, plan | Historical; `internal/ui/scrollbar.go`, `chat_virtual_scroll.go` |
| chat-view | design, plan, step2-plan, step3-design/plan, step4-design/plan | Historical migration stages; current `chat_view.go`, `chat_events.go`, `main_agent_driver.go`, `context_manager_driver.go` |
| dev-mode | design, plan | Historical; `internal/slash/dev.go`, `internal/ui/model.go:3081` |
| history-view | design, plan | Historical; `internal/slash/history.go`, `internal/ui/history_view.go:111` |
| image-input | design, plan | Historical; `image_paste.go`, `clipboard_image_darwin.go`, `clipboard_image_other.go` |
| living-recap | design, plan | Historical; recap data is consumed by `history_view.go:135`; no blanket original-plan acceptance claim |
| llm-provider-wireup | design, plan | Historical architecture; inference/capabilities/runner now live under server, not CLI |
| mcp-host | design, plan | Historical; `internal/slash/mcp.go`, managed MCP config tab |
| model-catalog-online | design | Historical; `cloud_provider_catalog.go`, `cloud_catalog_picker.go` |
| native-tool-calling | design, followups, tasks | Historical implementation/follow-up snapshots; server runner/capabilities, UI main-agent driver |
| settings | design, plan | Historical; `config_surface.go`, `config_tabs.go`, `settings_page.go` |
| streaming-markdown | design, plan | Historical; `internal/render/{glamour,mdstream,table,diff}.go` |
| textarea | spec | Historical; `prompt_input.go`, `prompt_image.go` |
| theme-system | design, plan | Historical; `internal/theme`, `internal/uiconfig/uiconfig.go:16` |
| tui-charm-v2 | design, plan | Historical; current `go.mod` declares Charm v2 and Go 1.26.0 |
| top-level guides | README, session-model, model-catalog-picker | Current semantic reference, updated as above |
| original roadmap | original-plan | Preserved history, superseded as an operational reference |

Paths in the evidence column are relative to `source/clients/cli` unless marked
server. The README includes a current implementation map. Historical documents
were classified and labeled, not revalidated line-by-line as current requirements.

## Operational evidence and corrections

- **Startup/build:** `source/clients/cli/main.go:27–66`, `go.mod:3`,
  `setup_start.go:10`; `source/server/pkg/agentclient/client.go:202` and
  `source/server/pkg/config/config.go:759`. Corrected single-binary/Go 1.21
  guidance, automatic latest-resume assumptions and obsolete bypass flags.
  Documented sibling/PATH server discovery and resumable setup.
- **Complete registered command families:** registration in
  `source/clients/cli/internal/ui/model.go:436–471` and handlers under
  `internal/slash`. Included search, exports, setup, runtime/model management,
  permission/locus/profile controls, context operations, dev/restart and all
  model controls. `registry.go:149` requires exact names/aliases at dispatch;
  prefixes are completion/suggestion behavior, not implicit invocation.
- **Effects rather than stale help alone:** `model.go:3246` starts a fresh
  conversation on clear; `model.go:3329` handles permission modes;
  `model.go:3368` queues known W/X direct-tool confirmations. `/mcp` opens its
  config tab. Old `/bypass on|off|status` and unimplemented `/font`, `/diff`,
  `/undo`, `/project` are not advertised as supported commands.
- **Config:** `internal/slash/config.go`, `internal/ui/config_tabs.go`, and
  `source/server/pkg/config/models.go:113`. Documented all scalar setter keys,
  nine tabs and the actual `models.<runtime>.<tier>` sparse patch shape instead
  of contradictory legacy helper text. `internal/uiconfig/uiconfig.go:16–58`
  supplies UI paths, environment overrides and fallback theme independently of
  the server config path.
- **Session model:** `internal/slash/model.go`, `internal/ui/session_model.go`,
  `session_tier_picker.go:21–167`; server `internal/server/session_model.go`,
  `internal/chatroute/route.go`, `internal/runner/core.go:318–350,447`,
  `internal/agent/permissions.go:54`, `internal/worker/session_model.go`.
  Persistence/isolation/rollover behavior is additionally specified by
  `internal/conversation/chat_route_test.go:10–58`. Added shortcuts and chip
  picker without implying that changing models transfers a running loop.
- **Catalog picker:** `internal/overlay/catalog.go`,
  `internal/ui/cloud_catalog_picker.go` and associated tests back draft-only
  selection, credential/save gating, async refresh and generation/scope checks.
  Runtime and conversation model pickers remain separate surfaces.
- **Feature/platform boundaries:** image loader and platform-specific clipboard
  files establish the 20 MiB limit and macOS-only clipboard acquisition. Render,
  scrollbar, history, prompt and chat-view sources establish the implemented
  terminal features without promoting old roadmap acceptance checklists.

## Verification

- Ran `go test ./... -count=1` in `source/clients/cli` after current-guide edits:
  **PASS** (root, banner, form, overlay, render, slash, theme, UI, uiconfig,
  wizard; buildversion has no tests).
- Checked current-guide local Markdown links and implementation-map paths;
  checked that every nested historical document has its banner and that the
  source CLI Markdown inventory is empty.
- No live provider requests, paid inference, live MCP-server acceptance run,
  terminal PTY run, installer/Homebrew validation or full server test suite
  was performed. Earlier feature test reports are explicitly historical.

## Remaining limits / implementation discrepancies

- Historical plans retain obsolete links and source paths intentionally;
  they are evidence of prior designs, not runnable instructions.
- Slash help for `/clear`, direct `/tool` invocation and model-tier key shapes
  does not fully match downstream handlers. The current guide describes the
  handlers; implementation/help-string fixes are outside this docs-only scope.
- Permission-mode updates currently ignore RPC errors and optimistically update
  the local chip (`model.go:3329`); the chip alone is not proof of server state.
- Catalog discovery errors are not uniformly typed, so the documented generic
  error fallback remains. Provider availability and vision compatibility are
  not established by local unit tests.
- This audit establishes source-backed current CLI documentation, not completion
  of every acceptance criterion in the preserved historical plans.
