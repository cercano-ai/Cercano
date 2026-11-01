# Simplified-resume: inherited foreground and task enrollment review

Baseline: `997e22847489457b8d29d7aa37d0514d1a1fd3cc` on `simplified-resume`.
Scope: the two new review findings only; no repeated live acceptance.

## Reproduction and source/data evidence

The minimal UI regressions against baseline production files fail:

```
go test ./internal/ui -run 'TestInheritedForegroundOutputAndPermission|TestEnrollmentBeforeTaskSubscription' -count=1
--- FAIL: TestInheritedForegroundOutputAndPermission
    inherited foreground output dropped
    inherited pending permission dropped
--- FAIL: TestEnrollmentBeforeTaskSubscription
    task subscription starts concurrently with enrollment
```

The baseline was also restored temporarily for a repeat red check, then the
working fixes restored. The repeat red check used only the two minimal tests:
keeping the later transport test in that baseline run first produced a build
failure because it references the new attachment acknowledgement helpers; after
excluding that post-fix test, both behavioral failures above were reproduced.
The tests exercise actual UI update/command paths, not
text searches. The enrollment test alone establishes the concurrent command
shape, not a particular network scheduler outcome.

Additional data probes:

- The real server's `TestAttachReplaysPendingApprovalWithoutGrantingIt` now
  asserts the replayed pending permission retains foreground=false-background
  identity and a nonempty epoch/generation. Its scripted tool remains unexecuted
  through observer disconnect/replay until explicit approval.
- `TestTaskObservationAfterEnrollmentTransport` uses the real pinned SDK and UI
  commands against a disposable loopback gRPC peer. A task subscription before
  enrollment receives PermissionDenied and ends (the SDK does not reconnect it).
  Initial enrollment and explicit takeover then both start a working task stream
  after acknowledgement, even with no main-chat output. Stale enrollment
  generations and canceled contexts cannot restart observation.
- `TestForegroundAttachmentTransportLocalAndInherited` drives real SDK decoding
  of foreground output and pending permission through loopback gRPC. A new
  observer receives inherited events; a locally initiating stream classifies its
  mirrored events as local, even when the observer wins the transport race.

## Changes

- Replace the UI's blanket foreground suppression with SDK `ConversationUpdate.Local`
  provenance. The SDK registers initiating requests before opening their RPC,
  resolves their epoch/generation from streamed data (including nested permission
  payloads), and waits cancellably when observer delivery wins the race. Track
  continuation generations as well as the first turn. Retain small version
  identities for the client lifetime to suppress replay after completion.
- Return an explicit successful-attachment UI message, then start task observation
  from that message. Both initial attach and confirmed takeover use this path;
  task startup no longer races pinned terminal enrollment. Existing parent and
  generation fencing remains in place.
- No changes to takeover confirmation/default-no, retry takeover metadata,
  ownership/work admission, or the prior trailers-only rejection fix.

## Validation

All commands below passed. Environment for Go tests:
`HOME=/tmp/cercano-review-home XDG_CONFIG_HOME=/tmp/cercano-review-home/.config`.

From `source/clients/cli`:

```
go test ./... -count=1
go test -race ./internal/ui -count=1
```

From `source/server`:

```
go test -race ./pkg/agentclient -count=1
go test -race ./pkg/agentclient ./internal/server -run 'Attach|Takeover|Terminal|Foreground|LocalAttachment|Completion|PendingPrompts' -count=1
```

The final focused server/client race command was rerun after extending the
transport regression to include pending permissions. The full SDK race suite
also covers `TestAttachmentTrailersOnlyConflictPreservesTakeoverTarget` and
`TestTakeoverMetadataOnlyOnInitialEnrollment`. Full UI tests retain the explicit
confirmation, duplicate suppression, and stale-background-event coverage.
`git diff --check` passed.

## Limits and safety

No live terminal acceptance was repeated. Transport peers are disposable and
loopback-only; the server pending-permission probe uses a temporary store and
scripted provider, not a live model. The transport enrollment peer models the
server's admission rule; real admission is covered separately by existing
terminal/takeover server tests. There is no new all-components, two-interactive-
terminal transport harness. The entire server test suite was not run; the full
CLI, full SDK race suite, full UI race suite, and focused server race suite were.
No production configuration/database/authentication/model service was used, and
no installation, restart, process signaling, push, or landing was performed.
