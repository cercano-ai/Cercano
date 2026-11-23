# Pinned transport recovery coverage

Branch: `fix/pinned-auto-recovery`. Implementation checkpoint: `00baa19a6ff5`.
Status: ready for independent review, **not landed**. Worker teardown and server
release lifecycle work (task522a654) is intentionally excluded.

## Behavior and invariants

A pinned chat transport failure restores a fresh connection through the existing
`connectIntent` policy and identity/session validation path. Chat is never replayed:
its error says delivery is unknown, no message was resent, and the conversation
must be inspected before sending again. Read-only attachment subscriptions may
reopen after validated recovery; takeover metadata is not replayed.

Recovery is serialized, has a three-second budget including waiting, observes
caller cancellation and client close, and fences late failures by connection
identity. Overlapping attempts share a completed failed attempt as well as a
successful replacement. Explicit `ReconnectPinnedSession` still forces fresh
validation. Connection installation and close synchronize under `connMu`; SDK
agent-handle readers now snapshot under that lock. Pinned event streams retain
the failed connection identity rather than accidentally replacing a newer handle.

The single-transport dialer, credential policy, endpoint, host/contract/schema,
conversation and epoch validation remain unchanged. No discovery, launch fallback,
or host migration is added. Pinned clients still have no background watcher:
recovery is demand-driven by chat/attachment/event-stream transport failures.

## Regression matrix

| Scenario | Coverage | Boundary / limitation |
| --- | --- | --- |
| Ambiguous chat failure | `TestPinnedChatAutomaticallyRevalidatesWithoutResend` | Loopback gRPC handler records submission then returns Unavailable; verifies fresh validation and exactly one submission. |
| Same-host transport loss | `TestPinnedAttachmentTransportDrop/A` | Stops real loopback gRPC transport, rebinds original endpoint with same identity, automatically revalidates, then successfully performs a subsequent RPC. Fake service, not a full daemon restart. |
| Replacement host refused | `TestPinnedAttachmentTransportDrop/B` | Automatic attachment recovery fails closed. |
| Stale retained ownership refused | `TestPinnedAttachmentTransportDrop/stale` | Original host reports STALE retained session, representing epoch rejection; not a real ownership migration. |
| Credential-enabled replacement refused | `TestPinnedAttachmentTransportDrop/credentials` | Isolation requested; same-identity replacement enables credentials and is refused. |
| Concurrent recovery / cancellation | `TestPinnedConcurrentRecoveryAndCancellation` | Twenty overlapping old-connection recoveries produce one fresh validation; canceled gate waiter and reconnect-after-close refused. |
| Close during validation | `TestPinnedCloseDuringValidationCannotInstall` | Blocks fresh retained-session validation, closes client, verifies prompt termination and no replacement installation. |
| CLI delivery ambiguity | `TestMainDriverPinnedRecoveryDoesNotResubmit` | Real SDK and loopback gRPC through CLI driver Submit; warning survives UI event conversion, one submission and fresh validation. |
| Existing security regressions | Full `pkg/agentclient` suite | Includes explicit pinned identity/session refusal and credential transport-isolation tests. |
| Server/SDK interface integration | Selected `internal/server` tests below | Existing real server attachment, replay cursor, pending approval and pinned identity tests; no server edits. |

The initial automatic chat regression was recorded failing before implementation
with `fresh validation count = 1, want 2` in the prior execution history. The
final-state runs below were rerun in this worktree.

## Verified commands

From `source/server`:

```sh
go test -race ./pkg/agentclient -count=1
go test -race ./internal/server -run 'TestCoexistenceIdentityPinnedIntegration|TestAttachConversation_|TestBackgroundFinalArrivesThroughSDKAttachment|TestAttachmentPagedReplayDoesNotDropAndResumesCursor|TestAttachReplaysPendingApprovalWithoutGrantingIt' -count=1
```

Both passed (agentclient 91.839s; selected server interface tests 6.795s).

From `source/clients/cli`:

```sh
go test -race ./internal/ui -count=1
```

Passed (13.444s). `git diff --check` passed before checkpoint.

## Review and remaining limits

No full monorepo test run or live daemon/terminal session was performed. No live
process, configuration or database was changed; no push or landing occurred.
The initial attachment-open/takeover failure restores connectivity but does not
retry the potentially mutating request. Generic unary RPCs do not automatically
retry. A recovery budget expiring while the endpoint is still down surfaces a
failure; callers can explicitly retry reconnect later. Concurrent failed-attempt
sharing is implemented but not separately stress-tested. Worker teardown/release
integration must be reviewed after the independently owned lifecycle branch is
available. Independent review remains required before landing.
