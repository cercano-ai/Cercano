# Lifecycle resume blocker diagnostics

This instrumentation is **internal/log-only**, not a remotely callable snapshot
RPC. No protobuf or authentication surface is added. Use existing structured
failure logging: an error returned by `LifecycleResume` emits
`lifecycle.resume_refused` when the server's failure-log writer is configured.
Successful calls (including a legacy confirmation offer returned without an
error) do not emit this failure event. Existing RPC responses are unchanged.

Each event has `stage`, `check` (the refusal/delegation site, `resume.go:<line>` in
that binary's source), `grpc_code`, and a bounded `error_kind` (including
`admission_busy`/`admission_stale`). Delegated recovery/claim failures identify
the delegation site, not a line inside that helper. After existing control
selection/authorization succeeds it also includes `conversation_id` and a local
`blockers` snapshot. Denied control requests omit identifiers and snapshots.
Raw errors, prompts, message contents, tokens, receipts, endpoints, database paths,
and request bodies are not included. Existing failure-log retention/access rules
still apply: authorized conversation/owner identifiers are operational metadata.

## What the snapshot means

The lifecycle snapshot accepts an exact conversation/host/epoch identity and
copies in-memory accounting under the lifecycle mutex followed by the admission
leaf mutex. Resume logs resolve an omitted host+epoch to the current in-process
capability only after successful control authorization; partially supplied or
mismatched identities are not substituted. Unknown/mismatched owners report
`Known=false`, without another epoch's readiness or admission details.

For a known owner it reports readiness, acquisition stage, admission open/closed,
draining/retired flags, attachment count, work counts by kind, and unresolved
persistence ticket counts. Host work/unresolved counts and retained-session count
are explicitly host-wide, **not attributed to this conversation**. Work-kind JSON
map keys are: 0 foreground, 1 task acceptance, 2 task execution, 3 callback,
4 persistence. Maps are independent copies. No ticket or attachment identities
are exposed.

`Reason` is a prioritized summary, not a list of every blocker: incomplete
acquisition, retired host, missing admission owner, closed admission, session
work, attachments, host work, then ready. Read all fields for simultaneous
blockers. `Ready` is the existing lifecycle readiness bit, not proof that a
transition is allowed or that no work/attachment exists. `acquisition_unresolved`
means acquisition is not ready; it can also describe in-progress scheduling.
`AcquisitionStage` records the last reached acquisition stage, not raw errors.
For example, the isolated failed-reload fixture reports not ready, stage `reload`,
and an unresolved host persistence ticket even though admission registration has
not occurred. A host ticket alone cannot establish which acquisition failed.

The snapshot performs no database/journal I/O, hooks, reload, acquisition,
receipt observation/delivery, persistence writes, or state changes. In particular
it does **not** call `CheckRetainedSession` or `LifecycleList`; those may observe
launch receipts. Logging writes only through the existing failure logger after
collecting the snapshot and releasing lifecycle locks. Admission, ownership,
sticky failed reload, and retry/release behavior are unchanged.

## Limits and isolated verification

There are no accurate work/attachment start timestamps in this accounting, so
ages are unavailable. The snapshot does not inspect durable ownership, operation
journals, remote hosts, or another process's in-memory state, and cannot list
other retained conversation identities. It cannot prove the historical cause of
a live refusal. Events describe the observation at logging time, not an atomic
transaction with the earlier failure; concurrent state may advance meanwhile.

Already-running old binaries do not acquire this instrumentation. Only binaries
built with this change can emit these events. No deployment/restart is performed
by the tests or this change.

Focused tests use temporary databases/journals and local fixture transports. They
cover two sticky failed-reload resume refusals with only one reload, a ready
control, per-kind work/attachments, unknown/mismatched ownership, denied-request
log privacy, repeated snapshot comparison of owner/readiness/accounting and all
fixture file contents/mtimes (including pending and delivered launch receipts), and concurrent
snapshot/accounting access under the race detector. They are not evidence about
the historical live cause.
