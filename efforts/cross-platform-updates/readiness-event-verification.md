# SubscribeEvents readiness acknowledgment

The real `DrainForUpdate` handler now publishes `ClientEvent.update_readiness`
after the reversible admission seal is established. It contains only
`operation_token` (32 lowercase hex characters) and `phase` (`DRAINING`, `IDLE`,
`RELEASED`). No paths, process identities, commands, or installation authority.
The token identifies the attempt; it is not authentication.

The updater's call asserts local input validation, acquisition of its OS barrier,
and independent initial native evidence have already happened. The server checks
only token syntax, admission coverage, and seal ownership: **it does not verify
those local assertions**. This notice is not staged-update verification, exit
proof, publication permission, or client-exit authorization.

## Harness observation

Open the existing `SubscribeEvents` stream on the fixture agent before starting
the actual updater. Match `update_readiness.operation_token` to the attempt and
accept either DRAINING or IDLE as the seal acknowledgment (fast transitions may
coalesce). Keep the updater-owned DrainForUpdate connection alive. A late
subscriber receives the current live snapshot. The TUI footer displays
`update: draining (exit voluntary)` or `update: idle (exit voluntary)`; it does
not prompt, quit, cancel work, restart, or replace pending permission/auth choices
or the unsent draft. Any normal user exit remains voluntary.

Cancellation/owner loss releases admission and publishes RELEASED for that
exact internal owner generation. Delayed old callbacks cannot withdraw a newer
owner, even with the same caller token. Existing subscribers clear the notice;
a subscriber attaching after release gets no historical readiness event. SDK
stream loss produces an empty readiness snapshot before reconnect; UI clears it.
No notice is durable across agent restart. Missing acknowledgment is not success.

Readiness uses a separate coalesced live-state slot/wakeup, not the ordinary event
queue; it cannot evict permission/configuration/installed-TUI events. Ordinary
broadcast delivery retains its existing bounded queue policy. Callbacks run
outside the admission mutex; release does not depend on subscriber consumption.

## Scope and evidence

Focused race tests cover real gRPC streams, validation/failed seal, competition,
late subscription, ordinary event preservation, idle/release/empty replay,
owner-generation fencing, and callback lock safety. SDK tests cover mapping and
stream-loss clearing; UI tests cover permission/auth decisions and draft retention.
Server and CLI `go build ./...` succeed. No live application was used.

Still pending: initiating local UI update action and independent launch/wait-for-
ack integration. This event alone authorizes no client exit. It adds no shutdown,
force, filesystem mutation, native exit proof, installation completion, or restart
integration. Updater command and native installation harness are unchanged by
this slice (parallel work owns them).
