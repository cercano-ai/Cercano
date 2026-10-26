# README control closure — 2026-10-23

Source comparison only, not live certification. Scope: autonomous completion and
later Permission gating, subscription authentication recovery, conversation-scoped
main-chat selection in `docs/agent/README.md`. Read the prior
`docs/audit-readme-final-closure.md`; excluded every other section and shared ledger.

## Increment 1 — autonomous completion / later Permission gating

Disposition: maintained claims match the compared implementation, with bounded
exceptions below. On final source review, qualified the README rollback claim:
rollback is attempted and its own failure is reported, not guaranteed.

- `source/server/internal/runner/core.go:1122` supplies the completion/decision
  protocol as model instructions, not an automatic proof that a brief is satisfied.
- `source/server/internal/capabilities/builtins/autonomous.go:213–282` defines the
  one-confirmation completion action, accepts running/legacy review_pending,
  preserves existing review fields and decisions, writes completed before changing
  profile, and attempts record rollback if profile change fails. Rollback itself
  can fail and is returned as an error; this is not an atomic transaction promise.
  `builtins.go:67` registers this action rather than the retired review tool.
- `source/server/internal/agent/permissions.go:54–78` adds session-model mutation
  and explicit human-handoff gates. `toolloop.go:1043–1155` rechecks profile fences,
  handles watchdog escalation approval without a duplicate prompt, permits delegated
  preauthorization to skip ordinary gates, and forces WIP-stash confirmation.
- `source/clients/cli/internal/ui/model.go:5027–5080` forwards stream-origin
  allow/deny/persist decisions. Previously audited direct `/tool` behavior is
  deliberately not re-audited here.
- Continuation comparison only: `server/autonomy_continuation.go:160–235` requires
  a current, uncanceled stream and running run, stops for explicit blockers, and
  bounds turns without durable progress/real work. This does not certify model
  compliance with the completion instructions.

Unresolved: runtime UI delivery and model compliance; atomic/crash rollback is
not claimed or certified. Previously completed direct-tool claims are inherited,
not newly verified. No tests, builds, network, or application changes.

## Remaining at this save

Subscription authentication recovery and conversation-scoped main-chat selection.


## Increment 2 — subscription authentication recovery

Disposition: source-backed within the boundaries below; corrected the overly
absolute configuration-preservation/credential-only wording. Login preserves the
saved routing profile and active selection but updates normalized account identity.

- `source/server/internal/llm/auth_recovery.go:45–78` requires structured
  subscription `ErrLoginRequired`, a named profile, and an installed requester;
  API-key/auth/permission prose does not trigger this gate. Unsafe replay removes
  the fallback option. `internal/inference/resilience/auth_recovery.go:13–60`
  bounds recovery by credential identity and acts on explicit login/fallback.
- `internal/inference/resilience/resilience.go:911–944` gates subscription failure
  before ordinary retry/failover; after emitted content, login repairs credentials
  but returns an explicit-fresh-request error rather than restarting the stream.
  `internal/runner/authentication_fallback.go:159–184` switches inference streams
  only before emission on explicit selection; it does not rerun completed tools.
  `internal/runner/core.go:589–601` installs that fallback only when permitted.
- `internal/hostsvc/config/config.go:64–97`,
  `internal/hostsvc/providers/providers.go:414–417`, and
  `internal/worker/host.go:1054–1108` share the configuration-owned credential
  service. `internal/hostsvc/credentials/service.go:220–240` fences refresh writes
  by flight/generation; `credentials/login.go:22–100` owns single-use attempts and
  provider/profile login notifications. Cancellation is checked before commit.
- `internal/hostsvc/config/cloud_login.go:29–87` clones the existing matching
  profile, checks it has not changed before commit, updates account identity,
  and avoids active-profile changes during reauthentication. This is the exact
  exception missing from the README's former configuration-preservation claim.
- `internal/server/cloud_reauthentication.go:20–41` keeps recovery distinct from
  onboarding. `internal/server/authentication_gate.go:25–101` owns conversation/
  request waiters, watches successful login, checks canceled contexts and routing,
  and atomically consumes explicit fallback/cancel decisions. It does not accept
  a client assertion that login succeeded.
- `internal/server/server.go:3787` installs recovery only for opted-in clients;
  `internal/worker/host.go:758–760,816–836` fails closed for unsupported worker
  protocol and relays challenges to the host. `source/clients/cli/internal/ui/
  authentication.go:63–139` renders login/cancel/explicit safe fallback, not a
  tool approval. Compatibility is source behavior, not tested interoperability.

Unresolved: actual OAuth/provider acceptance, browser/terminal delivery, durable
keychain behavior, and deployed mixed-version interoperability. No live login,
request replay experiment, tests, builds, or network calls were performed.

## Remaining at this save (supersedes the earlier list)

Conversation-scoped main-chat model selection and scoped publication checks.


## Increment 3 — conversation-scoped main-chat model selection

Disposition: compared storage, controls and main-chat routing match; no concrete
stale claim found in this section. Isolation is bounded to the compared paths.

- `source/server/internal/capabilities/builtins/session_model.go:14–55` exposes
  status/models as read-tier and set/clear as mutation; its description requires
  user intent and says next-message/resume semantics. `internal/agent/
  permissions.go:54–62` always gates non-read model selection at that boundary.
- `internal/server/session_model.go:14–155` resolves controls on the server,
  validates an exact saved profile/model before storing, clears only the
  conversation route, and provides the direct RPC independently of an LLM call.
  Tier resolution uses configured account choices, not a guessed model ID.
- `internal/conversation/chat_route.go:12–45` reads/upserts/deletes the separate
  SQLite conversation_chat_routes row; it does not write global task settings.
  Persistence here means a database-backed route, not a crash-durability test.
- `internal/server/server.go:3830–3838` loads the route into each turn request;
  `internal/runner/core.go:318–350` copies it into a local route before resolving.
  `internal/chatroute/route.go:65–101` validates locus/profile/model, clones the
  saved profile, builds a single provider and pins its task route without a backup
  chain. `internal/hostsvc/providers/providers.go:588–591` uses that builder.
  `internal/runner/core.go:447–449,589,685,714` excludes explicit pins from
  cross-tier fallback. Managed-policy denial remains possible, not silent rerouting.
- `internal/worker/session_model.go:31–74` scopes requests to the worker's active
  conversation; `internal/worker/host.go:971–990` checks that scope again and
  invokes the host control. `host.go:629–630` serializes the turn's selected route,
  and `worker.go:318–320` creates its request-local copy on the worker.

Unresolved: exhaustive delegated/background execution-path isolation (the
compared control/storage paths do not mutate their defaults), actual provider
model availability, deployed host/worker interoperability, resume UI presentation,
and crash durability. No live routing or model-availability certification.

## Final disposition and publication boundary

All three requested sections received bounded source comparison. README changes
are restricted to attempted rollback and reauthentication's saved-identity update.
Later Permission gating and session-model text otherwise remain unchanged.
The unresolved items in each increment remain explicit exclusions; this report
does not close other families from the prior audit or certify runtime behavior.

Only docs/agent/README.md and this new report are checkpoint-owned. Unrelated
cloud-routing working files are left intact. No tests, builds, network, application
changes, shared-ledger edits, push, merge, or delegation.

## Actual scoped checks

- git diff --check -- docs/agent/README.md docs/audit-readme-control-closure.md passed.
- Both owned files passed a line-by-line trailing-whitespace check.
- Offline inline relative-link filesystem check: 3 targets, zero missing,
  restricted to the four in-scope README headings and this report; external URLs
  and fragment-anchor validation excluded. No network/link fetching.
- Reviewed README diff: only the two in-scope paragraphs/families above changed.
