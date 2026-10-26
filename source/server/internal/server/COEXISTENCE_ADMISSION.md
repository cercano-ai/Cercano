# Prepared-server admission boundary

This change is a conservative closure, not a claim that every coexistence feature
is implemented.

* Rename, dismiss and context elision use the existing exact metadata-bound
  foreground admission before delegation. Failed writes keep the persistence
  blocker and suppress the success response. No ID-only owner lookup is added.
* Conversation/turn deletion is refused: deleting ownership-bearing rows under a
  live retained capability needs an explicit destructive lifecycle protocol.
* Regeneration (including incremental compaction) is refused before stream or
  generator effects, at both server and read-only persistence-service boundaries.
  Prepared agents have their ID-only recap/title and compaction schedulers removed.
  Retention and compaction generator setters refuse installation. Context-meter
  reads cannot schedule compaction through the read-only persistence service.
* Prepared config is frozen, its snapshot is shared-read-only, and its secrets
  are guarded read-only (an absent backend becomes memory-only, never native).
* Explicit guards cover setup reset, login flows, routing/backup/cloud-key and
  active-profile changes, catalog refresh, runtime installation/model start/stop,
  restart/download/cancel/delete, MCP restart and the restart runtime tool.
  Existing frozen config services continue to gate their other config writers.
* Permission decisions, session profile changes and rollover remain conservatively
  refused until their admission semantics are integrated.

## Concrete remaining work / startup contract

Background compaction is **disabled**, not implemented with retained authority.
`compactiongen.runCompaction` can call `Schedule` on contention and remaining
backlog; `Schedule` creates an ID-only timer with `context.Background`. Merely
carrying context into the first `CompactNow` call would still be unsafe. That
package is outside this change's ownership. Legacy persistence regeneration keeps
its old background behavior; it is unreachable through the prepared read-only
service. Enabling coexistence compaction requires fixing the generator's entire
retry/debounce/publication chain and testing queue retention there.

Startup must not start recap/title/retention/compaction generators before or after
prepared construction. Removing an Agent scheduler does not cancel work already
queued in an externally owned generator. Constructor factories must not start such
work. Server setters cannot stop independently running generators. Startup/native
runtime arbitration, autonomous download observers and external lifecycle of
watchers/managers remain separate integration concerns.

This is not an exhaustive public RPC/tool audit. Foreground model tools, external
runtime callbacks, and independently installed agent collaborators need separate
coverage before claiming complete shared-state safety. Per-session selections
currently fail closed rather than providing the spec's eventual functionality.

## Verification

`go test -race ./internal/server ./internal/hostsvc/persistence -count=1`
passes. New prepared-server tests cover stale admin refusal and unchanged title,
active rename release refusal, injected rename failure retaining the blocker and
suppressing the response, regeneration refusal before stream access, guarded
credentials, and selected global mutation refusals before collaborators run.
Existing prepared foreground and owned-completion tests cover admitted execution,
queued callback blockers, persistence failure and acknowledgment failure. They do
not prove a compaction queue implementation: no coexistence compaction is admitted.
