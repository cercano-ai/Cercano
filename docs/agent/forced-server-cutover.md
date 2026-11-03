# Explicit forced server resumption

A resume request from a terminal whose default server is B must not silently
connect it to retained owner A. Discovery returns a confirmation offer with
exact source/destination identities and the observed ownership epoch. Enter,
Escape and No decline without any mutation. Yes invokes the distinct
`LifecycleForceRelinquish` RPC on A, then claims the handover on B and validates
a fresh pinned connection before replacing the UI. Fresh conversations continue
to use B. Same-server terminal takeover remains a separate operation.

## Cancellation and fencing

* The source authenticates control access and probes the exact destination
  before changing admission.
* Forced closure revokes all old attachment references, even when **no work is
  running**. Attachment presence and durable ownership are not activity tests.
* New source work is fenced. Admitted foreground contexts, active turns, and
  independent tasks/descendants are cancelled. Unrelated conversations remain
  usable. Previously performed external effects cannot be rolled back.
* Producer tickets remain until execution, callbacks and writeback finish.
  Scoped dispatch evidence uses bounded, cancellation-independent writes under
  its existing ownership capability. Clean worker cancellation after teardown
  and callback join is not a persistence failure. Real write failures and joined
  callback failures retain their safety fence.
* Existing authenticated prepare/reserve/claim transfers ownership with a fresh
  epoch. No database schema change, lock bypass, or dead-owner adoption is added.
* Timeouts leave the source fenced, never advertised as successfully moved.
  The operation ID is stable for the exact owner epoch and destination; known
  receipts are kept in the current UI for explicit claim retry. Uncertain
  persistence is reported as uncertain persistence, not as running work.

## Compatibility and limits

Both participating server builds must implement the relevant protocol. An older
source that lacks the new RPC returns `Unimplemented`; the new client explicitly
reports that limitation and never falls back to terminal replacement on A.
Consequently the live-source RPC does **not** retrofit a running historical
executable. Dead-owner recovery is now a separate destination-side operation;
see [dead-owner-recovery.md](dead-owner-recovery.md). It requires verified source
absence, exclusive locks and transactional reconciliation, not cooperation
from the dead server. Live unresolved persistence tickets are not cleared by
this path. Restarting the CLI is not deployment of this server change.

A confirmed cancellation is irreversible: a failed destination preparation may
leave the source fenced and work cancelled. The exact operation can be retried;
no transport error is treated as authority to acquire the conversation anew.
Receipt recovery across a new CLI process is not added here; existing explicit
lifecycle receipt controls remain available. Detached external processes are not
claimed to be universally stopped by context cancellation.

## Verification boundaries

Regressions cover idle/stale attachments, admission before turn registration,
writeback blocking and retry, stale ownership refusal, cancellation of active
independent descendants while preserving unrelated chats, scoped dispatch
cancellation with durable stopped results and child release, ordinary-source
cross-module authenticated transfer and destination pinned dial, explicit
confirmation/default decline, and unsupported-host/no-fallback behavior.

The cross-module bridge runs an ordinary source with synthetic services and a
lifecycle-only destination. It proves real ownership transfer and client binding,
not a full two-terminal PTY journey, real model/tool-process cancellation, or
subsequent inference on an ordinary destination. Older live processes and the
user's database are not touched by these tests.

The old `--resume-journey` acceptance describes non-destructive attachment to A;
it is historical evidence and is not acceptance of this forced-cutover behavior.

## Recorded checks (2026-11-02)

All commands below passed using disposable paths and synthetic services.

```text
# source/server
go test -race ./internal/broker ./internal/server ./internal/taskrun ./pkg/agentclient ./internal/worker ./internal/hostsvc/tools ./pkg/proto -count=1 -timeout=120s
go test -race ./cmd/cercano -run '^TestProductionResumeOwnershipAfterClose$' -count=1 -timeout=60s
go test -c -o <temporary>/server.test ./cmd/cercano
# source/clients/cli, CERCANO_RESUME_PROBE_BINARY=<temporary>/server.test
go test -race ./... -count=1 -timeout=120s
go test -race ./internal/ui -run 'TestForcedResume|TestReopenProductionReleasedAndDisconnected' -count=3 -v -timeout=90s
```

After the final cancellation-isolation edits, the complete server and taskrun
package race suites and the rebuilt cross-module bridge were rerun successfully.
Detailed logs: `/tmp/fc-gate-xfncbh0g` and `/tmp/fc-verify-n72qtz3_`.

Baseline probes failed for stale references with zero work and for clean worker
cancellation retaining a persistence blocker. The scoped-dispatch integration
also exposed terminal evidence writes inheriting execution cancellation; its
fixed version passed three race-enabled repetitions. Genuine write failure and
panic retention regressions remain intact. A new cross-conversation epoch
isolation regression failed before correction and passes in the final suite.

An initial worker-suite run used a temporary Unix-socket path longer than the
Darwin limit. The final runs used short `/tmp` paths; no product change was made
for that test-environment issue.
