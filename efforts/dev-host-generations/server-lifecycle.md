# Lifecycle-only server integration

`NewCoexistenceServer` is an explicit fixture-only constructor alongside (not called by) `NewServer`. It opens the exact declared prepared store and requires immutable build/host identity, a reload hook, and a dedicated safe participation-close hook. It must **not** be registered for ordinary RPC handling. No startup, proto, taskrun, modelhost, broker, or agentclient enablement is included.

Implemented control surface: explicit `Retain`, sorted `ListRetained`, exact host/conversation/epoch `AttachRetained` (returned handle provides `Detach`), `AdmitRetainedWork`, `ReleaseRetained`, destination `AcceptIncoming`, source `HandoverRetained`, destination `ClaimIncoming`, and explicit `RetireCoexistence`. Reconnect cannot acquire or launch. Disconnect only detaches. Source handover success means reservation pending, not transfer complete. Claim retries return the durable `HandoverAlreadyClaimed` receipt, never another capability/epoch.

Destination acceptance returns an opaque in-process receipt, bound to conversation/source epoch/source host/handover ID/destination host, selected source build, exact contract, and the same fixture path. Its broker participation precedes reservation and blocks retirement. No timeout cancellation is supplied. `Ownership.ReserveHandover` closes the integration gap where the old convenience method generated its ID only during reservation; explicit IDs are checked against existing reservations and durable claim receipts. Source ownership retains its OS lock through the conditional reservation commit; the ownership helper unlocks only afterward.

Host work tickets cover acquisition through durable reload and admission registration, and relinquishment through commit, OS unlock, admission removal and cleanup. Busy/attachment refusals do not cancel anything and leave ordinary admission usable. Persistence, reload, unlock, or callback errors conservatively retain blockers and closed/unadmitted ownership; they do not guess whether it is safe to reopen. There is intentionally no reconciliation/recovery API in this slice. Hook failure during retirement is visible and leaves admission retired/closed; the general Shutdown path is never called.

## Caller obligations / interfaces needed next

- Before any normal request enablement, integrate **every** foreground turn, continuation, task acceptance/execution, completion callback/wake, and persistence path with exact retained-owner work tickets and `conversation.WithOwnership` capabilities. Ownership context plumbing remains internal to lifecycle reload for now; normal request work needs a scoped context-plus-ticket adapter, coordinated with taskrun's prepared-store/admission integration.
- Advance tickets without an idle gap; never complete on transport cancellation while effects, callbacks, or writeback survive. Only explicitly reconciled durable outcomes may clear unresolved persistence blockers.
- Reload must inspect durable conversation/task outcomes without starting/replaying live work. Cleanup must finish source callback work synchronously; returning an error preserves an unresolved host ticket. Hooks run under the lifecycle serialization mutex: do not re-enter Retain/List/Accept/Claim/Release from those hooks. The retirement readiness probe is independent and safely refuses while their host ticket is live.
- The participation-close hook must release **only this client's** participation. Supply an audited modelhost/runtime hook, not StopAll, Shutdown, backend restart, or a wrapper that can fall back to those. In-process tests exercise hook selection and sibling server usability, not native backend qualification.
- Future authenticated transport must carry equivalent exact-peer acceptance proof and distinguish pending reservation from completed durable claim. An arbitrary caller-provided tuple is not acceptance. There is no new registry/service here.
- Shared-config mutation guards, safe runtime connection/no-fallback policy, all admission-path integration, RPC/proto wiring, crash recovery, and uncertainty reconciliation remain prerequisites for production/startup enablement.

## Tests

Synthetic temporary owned fixtures only, deterministic in-process A/B/C servers. Coverage includes busy tickets of all work kinds, multiple attachments, unresolved persistence, destination participation before reservation, source lock contention, C refusal, fresh epochs, retry receipts, stale reconnects, source callback cleanup fencing retirement, acquisition reload fencing retirement, failed persistence/reload/cleanup remaining closed or blocked, dedicated participation hook failure, and sibling work remaining usable.

Commands (from `source/server`):

```
go test -race ./internal/server ./internal/conversation -run 'Test(CoexistenceLifecycle|CoexistenceAcquire|Ownership)' -count=1
go test ./internal/server ./internal/conversation -count=1
```

No native applications, user stores, or process-identity investigation were used. These tests are not native multi-process/runtime qualification.
