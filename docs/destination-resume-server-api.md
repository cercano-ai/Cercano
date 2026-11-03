# Destination resume contract and implementation

`LifecycleResume(LifecycleControlRequest)` runs on the **already launcher-selected
destination**. Normal CLI resume uses that destination's existing authenticated
control connection. It does not select/start another destination, duplicate launcher
build logic, or attach to the historical source. `defaultSelection` is unchanged.

## Caller and retry contract

Supply the exact selected `target`, `owner.conversation`, registered loopback
`destination_endpoint`, and current requester `LifecycleIdentity`. Do not supply a
source endpoint/token. The server resolves the exact owning source from private
host records and authenticates its lifecycle identity.

A successful `ready=true` response includes current destination ownership. Capable
live sources force-relinquish automatically; verified dead owners recover
automatically without routine confirmation. Force `Unimplemented` falls back to
cooperative relinquish. Neither RPC unavailability nor a generic failure proves
death: only the trusted recovery guard does, with repeated exact-epoch checks.

Transport retries use **the same destination and conversation**; do not manufacture
a new handover ID. The private store-adjacent `.resume-operations` journal retains
source, owner epoch, operation ID and receipt before source mutation. Interrupted
reservation/claim retries survive destination/UI restart and lost responses.
Historical claims cannot restore changed epochs.

## Blocked legacy source

Blocked cooperative relinquish returns `confirmation_required=true`, `ready=false`,
exact `source`, observed `owner`, stable `operation_id`, diagnostic `blocked_reason`,
and, only when a verified adapter can prepare one, `legacy_process_identity`.
This response authorizes **no stop or signal**.

The CLI asks explicit whole-source-host interruption confirmation, names the source
and requested conversation, and warns that sibling conversation/task inventory is
incomplete and external effects may remain uncertain. Decline and stale UI callbacks
send no continuation. Acceptance repeats `LifecycleResume` on the same destination
with `confirm_legacy_stop=true`, the offered source/owner/operation/process identity,
and original requester. Server validation binds these fields to the durable journal;
a changed requester, source, epoch, operation or process identity fails closed.
Destination and requester hosts cannot be stopped.

`LegacyProcessStop` is an injected trusted platform boundary: preparation must prove
UID, PID/start identity, executable and exact host attribution. Stop must hold source
launch exclusion, revalidate the snapshot and operation immediately before signaling,
and verify death. Recovery independently repeats the established absence guard before
exact-epoch recovery. A lost stop response can finish through guarded recovery without
a second stop; approval is not inferred from a prior confirmation-required response.

### Remaining platform blocker (production stops disabled)

No production `LegacyProcessStop` is wired. Existing Darwin upgrade primitives verify
kernel executable/start identity and listener ownership for upgrade targets; they do
not establish the exact historical development-host attribution under that host's
launch-record exclusion. `VerifyHostAbsent` proves absence under exclusion, not signal
authority. PID/argv matching alone is not sufficient. Unsupported platforms also fail
closed. Thus capable force transfer, dead recovery, CLI confirmation and injected
confirmed continuation work, but accepted legacy interruption in production returns
an explicit safe-stop-unavailable refusal, with no signal. Completing that platform
adapter is required for end-to-end legacy interruption; this is not full completion.

## Verification (2026-11-03)

Only isolated loopback test peers, temporary stores and injected process adapters
were used; no live host stop, production mutation, deploy or restart was performed.
Targeted tests were observed failing before wiring/fixes (CLI old acquire/recover RPC
policy, then stale-requester approval); regression coverage includes selected CLI
destination, automatic dead recovery, capable force, cooperative fallback, legacy
accept/decline/stale identity, sibling-impact wording, restart and lost stop response.
A new unsupported-at-offer test first failed with a misleading stale-approval error;
validation now explicitly reports safe legacy process stop unavailable when no
verified process snapshot could be offered, without signaling. The scoped checks
below were rerun after that fix.

From `source/server`:

- `go test ./internal/server ./internal/conversation ./pkg/proto -count=1` — passed.
- `go test ./internal/server -run '^TestDestinationResume' -count=1 -race` — passed.
- `go test ./cmd/cercano -run '^TestDevelopment' -count=1` — passed.
- `go build -o /tmp/cercano-resume-contract-diag-20261103c ./cmd/cercano/` — passed.

From `source/clients/cli`:

- `go test ./... -count=1` — passed (including UI).
- `go test ./internal/ui -run 'Test(Resume|LegacyResume|DeadOwner|LauncherResume|ForcedResume)' -count=1 -race` — passed.
- `go build -o /tmp/cercano-cli-resume-contract-diag-20261103c .` — passed.
  An initial build invocation incorrectly used nonexistent `./cmd/cercano/`; corrected
  to the CLI root entrypoint above.

### Command-package failures: baseline vs regression

Ran `go test ./cmd/cercano -run '^(TestDevHostTerminalPTY|TestEverydayLauncherEntrypointRefusalsPTY)$' -count=1`
on both the working tree and an isolated archive of server commit
`9418eea799d9947282ab9e91814cf8d9efef6dbb`. Verified all 3,357 archived tracked
blobs against that commit with `git hash-object` (zero mismatches).
**Both tests fail identically on both**:

- `TestDevHostTerminalPTY`: missing durable fixture history after reconnect; captured
  context-meter warnings include `coexistence write requires explicit ownership capability`.
- `TestEverydayLauncherEntrypointRefusalsPTY`: exit 78, `config authority must be
  canonical and inside isolated prepared state`, before the expected injected
  publication failure.

Logs: `/tmp/resume-9418eea-pty-baseline.log` and `/tmp/resume-working-pty.log`.
These are established pre-existing failures, not introduced by this change. They
remain unresolved; the command package/full server suite is not green and no full
suite sign-off is claimed. The archived baseline is retained in `.resume-baseline/`
(untracked, not part of the implementation commit) for reproduction.
