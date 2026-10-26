# Native compiled A/B lifecycle harness (2026-10-26)

This extends, and supersedes the dependency/identity-only limitations in,
`startup-integration-evidence.md`. It is **not full foreground/independent-task
acceptance**. No ordinary startup, installed launcher/reaper, current runtime,
user credential store, paid provider, merge or push was used.

## Integration

`cmd/cercano/dev_host.go` uses `NewPreparedDurableServer` and preserves the injected
`NewWorkerRunnerWithPolicy`, explicit prepared-store preflight, frozen config,
memory-only guarded secrets and protected pinned synthetic-modelhost attachment.
Background generators remain uninstalled. Only durable lifecycle controls,
`ResumeConversation` and read-only `CheckRetainedSession` are newly admitted.
All execution streams and other unary methods remain fail closed after auth.
Per-host token/peer filenames can be selected as regular fixture basenames;
A and B use separate private tokens and frozen peer declarations.

`TestDevHostCompiledStartup` now actually compiles two CLI artifacts, with
`-ldflags=-X main.version=fixture-build-exact` and
`-ldflags=-X main.version=fixture-build-B`, in separate temporary generation
directories. SHA256 manifests pin different artifacts. Each child gets only:

```
CERCANO_DISABLE_CREDENTIALS=1
HOME=<fixture>/home
XDG_CONFIG_HOME=<fixture>/config
TMPDIR=<fixture>/tmp
PATH=/usr/bin:/bin
```

A invocation (absolute temporary paths substituted by test):

```
<fixture>/generation/cercano dev-host --fixture <fixture> \
  --generation <fixture>/generation --modelhost <fixture>/modelhost \
  --host fixture-host-exact --listen 127.0.0.1:0
```

B uses `generation-B`, `--host fixture-host-B`, a reserved loopback port,
`--token-file token-B --peers-file peers-B.json`. Neither binary is rebuilt in
place. Artifacts, private tokens, manifests, DB and logs are temporary test
fixtures removed by cleanup; they are not retained release artifacts.

## Executed assertions

- Explicitly prepare the store; seed parent and sibling history and completed,
  already-notified task outcomes under fixture ownership, then release it.
- A acquires both sessions. Authenticated pinned client validates A's parent.
- Starting B preserves both durable histories and completed tasks. B identity is
  distinct; A's token cannot authenticate to B.
- Prepare B receipt, refuse premature B claim, explicitly relinquish quiescent A
  ownership, claim fresh B epoch and resume durable parent history through B RPC.
- Pinned A reconnect refuses after transfer and again after retirement; it does
  not migrate/reacquire. Separate existing agentclient tests instrument the launch
  function and assert zero calls for stale/released/retired/unreachable cases.
- A retirement refuses with its sibling retained. Explicitly release sibling,
  retire A, and successfully resume B again. Synthetic runtime participation
  falls from two attachments to one, rather than stopping B's participation.
- Actual invalid-credential child starts fail before any synthetic backend RPC.
  A/B report disabled credentials; command constructs memory secrets explicitly,
  not a native opener. Missing unary/stream auth and unauthenticated acquisition
  fail before dispatch; unauthorized history is absent. Execution stream still
  refuses even with valid auth. Fixture HOME/config stay empty; config.json is
  unchanged.

## Commands/results

Run from `source/server`:

```
go test ./cmd/cercano -run '^TestDevHost' -count=1 -v
# PASS (8.535s at first expanded run)
go test ./cmd/cercano ./pkg/devhostauth ./pkg/agentclient \
  -run 'TestDevHost|TestAuthenticationBeforeUnaryAndStreamEffects|TestPrivateTokenFile|TestAuthenticatedPinnedReconnect|TestPinnedNoLaunchRefusals' \
  -count=1 -v
# PASS: cmd 8.534s; auth 0.324s; agentclient 1.172s
go test -race ./cmd/cercano -run '^TestDevHost' -count=1 -v
# PASS on final code: 9.391s
```

The race run instruments the parent harness, not its normal separately compiled
children. A regression probe temporarily replaced `NewPreparedDurableServer`
with `NewPreparedServer` and ran
`go test ./cmd/cercano -run '^TestDevHostCompiledStartup$' -count=1 -v`.
It failed (exit 1) before ready with
`build, exact host, reload and safe participation hook required`. The durable
constructor was restored in a Python `finally` before final passing validation.
This probe occurred after the integration edits, not before the initial fix.
No claim of a pre-fix run of the original refusal callback is made here.

## Remaining gaps (not implied by passing lifecycle coverage)

No actual foreground model/tool execution was performed: the synthetic manager
has no provider registered, the prepared agent has nil provider/router, and the
worker factory has no provider/child callbacks. Stream execution remains denied;
opening that gate alone would not qualify deterministic execution. This change
adds no new fake inference provider or unsafe external-provider fallback.

The task records are **seeded completed/notified outcomes**, not live independently
executing tasks or pending-outcome automatic continuation. Those journeys remain
unverified in compiled agents. Synthetic attachment survival is not a loaded
model/resource-hold test. Zero-launch counters are in the existing agentclient
unit test, not an instrumentation counter inside the compiled agents. No native
credential-opener spy is linked into the child; exclusion is the explicit
memory-only construction plus environment/startup/auth checks above.

Manual TUI move/version/clickable identity checks, live sibling task completion,
pending completion wake execution and loaded shared backend holds remain gaps.
No whole-repository validation or full native A/B acceptance is claimed.
