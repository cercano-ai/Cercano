# Destination resume: server unit

`LifecycleResume(LifecycleControlRequest)` runs on the **already launcher-selected
 destination**. It does not select or start a destination, stop a source process,
 or implement a CLI prompt.

## Caller contract for the follow-up CLI unit

Use the destination's existing authenticated control connection. Supply:

- `target`: the exact selected destination `LifecycleIdentity`;
- `owner.conversation`: the conversation to resume;
- `destination_endpoint`: that destination's registered loopback control endpoint.

Do not resolve a second destination or supply a source endpoint/token. The server
resolves the exact owning source from private development host records and
checks its authenticated lifecycle identity before requesting relinquishment.

A successful `ready=true` response includes the current destination `owner`.
A live handoff also returns `operation_id`. Transport failures are retried against
**the same destination and conversation**; do not manufacture a new handover ID.
The private store-adjacent `.resume-operations` journal retains the exact source,
owner epoch, operation ID and receipt before source mutation. Interrupted
reservation/claim retries restore that operation, including after destination
restart; a historical claim alone cannot restore a changed epoch.

For authenticated legacy sources, `Unimplemented` on force relinquish triggers
cooperative relinquish. If cooperative relinquish is blocked, the response has:

- `confirmation_required=true`, `ready=false`;
- `source`: the exact authenticated source identity;
- `owner`: the source conversation/epoch observed for this operation;
- `operation_id`: stable identifier for this pending operation;
- `blocked_reason`: diagnostic text (not a machine-readable action).

This response authorizes **no stop or signal**. The follow-up CLI must implement
an explicit legacy confirmation interaction and separately reviewed exact-source
stop handling, revalidate the source/operation/owner before acting, and then retry
the same destination request. There is intentionally no confirmation/stop RPC in
this server unit. Never interpret an unavailable source or generic RPC failure as
proof of death. Only the existing trusted recovery guard can establish death;
the server repeats that guard and exact ownership checks before recovery.

## Scope and verification

Server integration tests use real loopback gRPC peers and shared ownership
storage. Coverage includes force+claim, authenticated legacy cooperative and
blocked outcomes, durable interrupted retries (before source contact, lost source
response and after claim), guarded dead-owner recovery, source-authentication
failure and ownership races, including a race before legacy confirmation.
Development host tests cover exact private-record selection and ambiguity.

CLI `dev_host_reopen` still needs to call this RPC instead of its old
acquire/recover-only sequence. CLI prompting and legacy whole-host stopping are
not implemented here; this is not an end-to-end resume completion.

### Verification for this unit

Run from `source/server`:

- `go test ./internal/server ./internal/conversation -count=1` — passed.
- `go test ./internal/server -run '^TestDestinationResume' -count=1 -race` — passed.
- `go test ./cmd/cercano -run '^TestDevelopment' -count=1` — passed.
- `go test ./pkg/proto -count=1` — passed.
- `go build -o /tmp/cercano-resume-contract-diag-20261103c ./cmd/cercano/` — passed.
- `git diff --check` — passed.

The broader command-package suite is **not green**: a full package run reported
`TestDevHostTerminalPTY` timing out waiting for durable fixture history after
reconnect. A run excluding that test reported
`TestEverydayLauncherEntrypointRefusalsPTY` expecting an injected publication
failure but receiving `config authority must be canonical and inside isolated
prepared state`. These PTY fixture failures remain unresolved; the scoped server,
ownership, development-host and proto results above do not constitute full-suite
or end-to-end sign-off. No live runtime operation or deployment is part of this unit.
