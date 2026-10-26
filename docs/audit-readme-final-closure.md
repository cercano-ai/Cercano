# README final bounded source closure — 2026-10-23

## Scope and disposition

Owned files: `docs/agent/README.md` and this report, including their existing
uncommitted changes. Routing files belong to another writer and are not staged.
This matrix consolidates the saved increments and supersedes their repeated
partial/open-family lists and promises of a later matrix. Saved citations are
retained evidence, not claims of fresh inspection in this finalization.
Only `source/server/internal/agent/toolloop.go:1205–1275` was freshly read to
resolve the remaining concrete watchdog qualification. No broad source tracing.

Source closure is bounded to the exact assertions below. The saved
[control closure](audit-readme-control-closure.md) now supplies cross-report
evidence for autonomy, later Permission gating, subscription-auth recovery and
conversation-scoped main-chat selection. The reconciliation below supersedes the
former whole-family gaps, not implementation completeness or runtime acceptance.

## Exact-section closure matrix

Source paths in this table are relative to `source/`; abbreviated filenames in
a cell share the preceding directory unless otherwise stated.

| README section / assertion | Disposition and retained source evidence |
|---|---|
| Prerequisites; architecture dependencies | Go 1.26/separate modules match `server/go.mod:3`, `clients/cli/go.mod:3`; Anthropic SDK 1.51, Ollama and pure-Go SQLite match `server/go.mod:8,16,25`; Bubble Tea matches `clients/cli/go.mod:8`. |
| Setup 4 / Cloud; Cloud setup | Provider creation, keys/subscriptions, account actions, quality/model selection, custom endpoints and Mistral combined catalog: `clients/cli/internal/ui/cloud_section.go:125–148,191,205–295`; account credential/catalog selection: `server/internal/server/mistral_catalog.go:18–90`. `/cloud` lists/selects/keys existing profiles (`clients/cli/internal/slash/config.go:73–141`); creation instructions corrected to Cloud settings, including Meridian. |
| Cloud flavors, subscription prerequisites and account traversal | Four flavors and subscription token-source routes: `server/internal/cloudfactory/factory.go:19–24,68–122`. Built-in Claude Max/Pro sign-in does not require Meridian. Generation-checked account state, quota advancement and finite exhaustion: `server/internal/inference/profilechain/cycle.go:106–165,229–246`. ChatGPT token save/refresh: `server/internal/chatgptauth/source.go:14–107`. Fixed model allowlist replaced with account-scoped catalog wording. |
| Setup 4 / Local | Managed-runtime setup/readiness gates: `clients/cli/internal/ui/settings_page.go:562–579`; hard local-only governor: `server/internal/inference/router.go:67–103`. Selecting a model alone is not an offline guarantee. |
| Setup 5; What you get / native tools | Submission and slash separation: `clients/cli/internal/ui/model.go:2979–3008`. Tool resolution, profile fences and R-tier goroutines versus W/X partition: `server/internal/agent/toolloop.go:925–975,1010–1040`. Source behavior is not a promise of correct model choices; completed permission controls are inherited, not reopened. |
| Slash commands / parser and UI dispatch | `clients/cli/internal/slash/basic.go:10–42`, `color.go:13–74`, `config.go:16–141`, `settings.go:6–14`, `theme.go:5–12`, `tools.go:14–46`, `context.go:13–95`, `contextview.go:5–12`, `mcp.go:12–74`; `clients/cli/internal/ui/model.go:3225–3414`. `/help` generates registered command help, not a keymap. Ctrl+C clears selected/composed input first; double-empty-press exit: `model.go:3620–3649`. |
| Slash commands / downstream context, MCP and config | Elision preserves raw turns with a process-local send-view floor: `server/internal/hostsvc/persistence/persistence.go:1164–1201`; clear/regenerate/incremental dispatch: `:1263–1316` (incremental compaction starts asynchronously). MCP RPC dispatch: `server/internal/server/server.go:463–520`; lifecycle/persistence: `server/internal/mcp_host/manager.go:262–328` (add may retain failed connection status). Config commit/persist/provider reconfiguration: `server/internal/server/server.go:2094–2125`. These close asserted operations, not universal success. |
| Architecture / provider interface and internal blocks | Actual interface is Name/Capabilities/Chat/StreamChat: `server/internal/inference/provider.go:52–57`, not the stale Infer/Stream introductory comment. Streaming calls: `server/internal/agent/toolloop.go:745,1312`. Internal text/tool/image/reasoning schema: `server/internal/llm/messages.go:7–13,23–51`; reasoning was added to the inventory. |
| Cloud resilience; Meridian streaming | Retry classes: `server/internal/llm/error.go:150–184`; one narrated live restart and guarded pre-content retry/failover: `server/internal/inference/resilience/resilience.go:841–889,954–990`. Corrected busy-only and never-replay absolutes. Request decoration: `server/internal/llm/anthropic/auth.go:1–64`; removed unsupported proxy-header/turn-cap promises. Tool-loop StreamChat use remains, without an external proxy-cap rationale. |
| Tool loop / three-error guard | **Corrected in this finalization:** `server/internal/agent/toolloop.go:1209–1216,1228–1243` computes all-error results, then prioritizes `watchdogIntervened` over incrementing the counter and testing `>= 3`. Watchdog challenge/block/escalation iterations neither advance nor reset the counter. README now qualifies the counted iterations; no application edit. |
| Built-in tools / aliases, command execution and limits | Registration/aliases: `server/internal/capabilities/builtins/builtins.go:27–111`; reads/globs: `fs_read.go:19–32,263–310`; atomic writes/edits: `fs_write.go:59–110,163–198`; destructive operations: `fs_destructive.go:44–65`; git writes: `git_write.go:160–175`; command execution: `run.go:79–139,196–209`. Corrected canonical RunCommand (Bash is legacy), direct argv rather than implicit shell, and 16 KiB cap per output stream. Inventory is explicitly selected, not exhaustive. |
| Persistence / stored turns, resume and rename | Block JSON and durable title upsert before first turn: `server/internal/conversation/store.go:629–655,830–858,926–947`; automatic titles exclude user titles: `:950–958`. Host forwarding: `server/internal/hostsvc/persistence/persistence.go:649–677,862–869`; load/meter/title forwarding and in-memory replay: `server/internal/agent/agent.go:288–328,424–428`. Internal-block reconstruction/text fallback/pair repair: `server/internal/agent/history.go:16–46`; summary plus live tail: `server/internal/compactor/compactor.go:51–64`. Corrected provider-native/no-translation/lossless absolutes: schema includes display metadata, excludes ProviderExtras (`server/internal/llm/messages.go:35–51`). |
| Known limitations / folding and restored transcript | Tool navigation/toggles: `clients/cli/internal/ui/model.go:1586–1606`; folding/lazy fetch: `clients/cli/internal/ui/chat_view.go:928–963,1831–1848`. Resume skips blank prose turns and inserts timestamp-bound recap divider: `clients/cli/internal/ui/model.go:3959–4010`. Removed obsolete no-keybind claim; restored text is not a replay of every live tool card. |

## Cross-report reconciliation — 2026-10-23

Saved reports only; no new source investigation. The
[control closure](audit-readme-control-closure.md) records:

- **Autonomous completion / later Permission gating:** Increment 1 compares the
  one-confirmation completion action, legacy `review_pending`, retained review
  fields/decisions, completion-before-profile-change and attempted rollback with
  reported rollback failure (`autonomous.go:213–282`), plus permission fences and
  stream-origin decisions. This closes the former broad autonomy gap, not an
  atomic transaction or guaranteed model compliance.
- **Subscription authentication:** Increment 2 compares structured login-required
  gates, safe replay/fallback, credential ownership and cancellation. The corrected
  preservation claim allows normalized saved-account identity updates while
  retaining routing/active selection. Live OAuth and delivery remain excluded.
- **Session model selection:** Increment 3 compares conversation-only storage,
  exact profile/model validation, next-message/resume semantics, pinned main-chat
  routing and worker conversation checks. This closes the former broad selection
  gap, not exhaustive delegated/background isolation or crash durability.

### Final disposition of the two carried-forward gaps

Superseded by the fresh bounded evidence below: autonomous completion decline
is source-supported; a separate session-model transition/rollback guarantee is
not asserted by the README and is out of scope. No remaining actionable source
evidence gap is identified for these two maintained-claim checks.

## Inherited work and runtime exclusions

Previously completed startup/headless, setup 2/3/6, Permission modes and later
permission callbacks, direct tools, attachments, rename/resume parsing,
history/clear local handlers, development mode and subscription-auth recovery
are inherited from the earlier control/self-development work, not re-audited.
The control report was read for the reconciliation above; other inherited reports
were not freshly revalidated.

Excluded from source closure: live sign-in, keychain/OAuth success, account/model
eligibility and third-party ToS; external Meridian behavior/turn limits;
terminal delivery; correct model tool choice; exhaustive MCP/config/error
branches; full viewport/client reconstruction and live-card replay; raw-body
retention/pruning and crash durability. These are not implementation backlog
unless an actual README assertion is identified. No byte-for-byte replay or
universal connection-success promise remains.

Failed guessed UI/MCP/config paths in earlier continuations supplied no evidence.
No shared ledger or routing guide audit, application changes, tests, builds,
installation, network, restart, push or merge were performed in this finalization.

## Scoped publication checks

Finalization checks are recorded after execution below. Only the two owned
Markdown paths are checkpoint targets; unrelated routing changes stay intact.

- `git diff --check -- docs/agent/README.md docs/audit-readme-final-closure.md` passed.
- Both files passed line-by-line trailing-whitespace checks.
- Offline relative filesystem-link check: 16 targets, zero missing; fenced blocks,
  external URLs and fragment anchors excluded. No fetching or anchor certification.
- Checkpoint restricted to the two owned files, including pre-existing owned edits.

## Reconciliation publication checks — 2026-10-23

Saved-report reconciliation only; no guide/source edits or runtime checks.
Scoped whitespace and offline local-link results are recorded in the shared
[remaining ledger](audit-core-remaining.md). Historical checks above belong to
that earlier finalization and were not rerun against the README here.

## Final two-gap source closure — 2026-10-23

Fresh scope: only the two carried-forward questions, with README assertions read
before source comparison. This continuation owns this report and the three
shared ledgers, not the README or application files. Six inspection calls total
(including one failed command); no further source inspection.

- **Autonomous completion decline — maintained claim closed.**
  `docs/agent/README.md:325–338` says declining completion leaves the run active.
  `source/server/internal/agent/permissions.go:54–78` preserves human-handoff
  gating in bypass and lists `request_autonomous_exit`; the action is X-tier
  (`source/server/internal/capabilities/builtins/autonomous.go:213–222`).
  `source/server/internal/agent/toolloop.go:1110–1157` shows the actual gate
  (including preauthorization/watchdog exceptions): a denied permission records
  an error result and returns before Execute at line 1163. Thus that declined
  action does not write completion or change profile. The mutations are inside
  Execute, at `autonomous.go:245–278`, after active-run validation. This closes
  decline behavior, not universal prompting, UI delivery or model compliance.
- **Session-model transition/rollback — no separate maintained guarantee.**
  `docs/agent/README.md:401–409` asserts inspect/set/clear of a durable main-chat
  override and next-turn behavior; it does not assert transactional switching or
  rollback. The rollback text at lines 334–338 concerns autonomous completion,
  not session models. A separate session-model rollback requirement is therefore
  **out of scope**, not a missing feature or evidence gap.
- **Exact set/clear handler error path compared.**
  `source/server/internal/server/session_model.go:70–99` trims set arguments,
  requires a resolver, runs Mistral-selection validation and route resolution
  before `SetChatRoute`; each validation/storage error returns immediately.
  Clear rejects profile/model arguments and calls `SetChatRoute` with nil;
  its storage error also returns immediately. Both then read back `ChatRoute`;
  a read-back failure returns an error after the write, with no compensating
  rollback in this handler. Lines 101–103 state next-message/resume semantics,
  not provider availability. RPC lines 129–140 propagate conversation creation,
  handler and serialization errors. No claim of storage atomicity or rollback
  on every error follows from this comparison.

**README disposition:** unchanged; no concrete assertion/source mismatch found.
All prior runtime/external exclusions remain. No broad tracing, application
edits, runtime/network checks, tests or builds were performed.

**Publication checks:** scoped `git diff --check`, trailing-whitespace and offline
relative filesystem-link checks cover only the four owned audit documents;
external URLs and fragment validation excluded. Results recorded by the save
command below; checkpoint explicitly restricted to those four files.

- Final checks passed: scoped diff whitespace, four-file trailing whitespace,
  and 41 offline relative filesystem-link targets; zero missing targets.
