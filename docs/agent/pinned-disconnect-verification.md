# Pinned disconnect candidate verification

Candidate: `fix/pinned-disconnect-observation`. The 2026-11-24 review blocker
below was reproduced and repaired in the same worktree, preserving all six
modified tracked and three untracked candidate files. Not landed, pushed or
installed. Historical review findings follow the repair verification.

## Repair verification (2026-11-24)

Before editing, the exact retained `demand-overlay.json` probe below failed with
`new demand after host restored is blocked by cached failure: context deadline exceeded`.
The retained HEAD `overlay.json` also failed `TestNewTestPinnedIdleHostExit`
with `idle transport loss not detected: connected`.

Policy: only a new Chat submission uses demand recovery, before creating its RPC.
A completed Unavailable/deadline failure may be retried once per later demand
through the same bounded serialized gate. Observers retain cached failures;
waiting callers retain generation fencing and share the completed outcome.
Auth, authorization, credential isolation and lifecycle refusals remain cached.
Fresh validation is still required before Connected; no host launch, migration,
Chat replay, or delayed delivery is introduced. A failed preflight explicitly
reports `request not sent`. Previously interrupted RPCs remain uncertain and
require inspection and explicit user retry. The CLI guidance points to sending
again, not an unavailable reconnect command.

Permanent regressions cover failed idle recovery followed by same-owner restart
and new Chat demand, twenty cached observer calls with no validation or Chat,
concurrent failed demands sharing auth refusal, repeated demand refusal for
stale/retired/auth/policy failures, and actual CLI driver + Model.Update recovery
with exactly one new submission and one outage notice. Existing cancellation,
fresh-validation barriers, generation fencing and intermediate-final zero-replay
tests remain intact.

A disposable overlay removing only the new pre-send demand hook made both
`TestPinnedDemandAfterFailedIdleRecovery` and `TestPinnedIdleDisconnectUI/host`
fail (`grpc: the client connection is closing`). Artifacts:
`/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/pinned-demand-verification-g7ccjpbp`.
The old review probe calls the observer-only internal recovery path; that path
intentionally still caches failures. The permanent tests use the reachable send
boundary instead.

Passed after repair (external timeouts 700/900 seconds for grouped commands):

```sh
go -C source/server test -race ./pkg/agentclient -run 'TestPinned|TestConnectionFailureClassification' -count=5 -timeout=300s
go -C source/clients/cli test -race ./internal/ui -run 'TestPinned|TestMainDriverPinned|TestConnRecovery' -count=5 -timeout=300s
go -C source/server test -race ./pkg/agentclient -run TestPinnedConcurrentFailedDemandSharesRefusal -count=10 -timeout=120s
go -C source/server test ./pkg/agentclient -count=1 -timeout=180s
go -C source/clients/cli test ./... -count=1 -timeout=180s
go -C source/server test ./internal/server ./cmd/cercano -count=1 -timeout=180s
go -C source/server build -o <artifact-directory>/cercano ./cmd/cercano/
```

The final five-repeat race runs include concurrent refusal and a failed user
send while the host is down: the latter reports not sent and is not delivered
when the host returns. The full agentclient suite was rerun afterward. No live host,
configuration, database, installed binary, or launcher changes were made. No
full-server suite or live-model/native-credential qualification is claimed.

## Fresh execution

Actual macOS host execution resolved `/usr/bin/git` and `/opt/homebrew/bin/go`.
The earlier unavailable-tool report is historical, not a current limitation.
All commands below passed from the candidate worktree:

```sh
go -C source/server test ./pkg/agentclient -run 'TestPinnedIdle|TestConnectionFailureClassification' -count=5 -timeout=120s
go -C source/server test -race ./pkg/agentclient -run 'TestPinned|TestConnectionFailureClassification' -count=3 -timeout=300s
go -C source/clients/cli test -race ./internal/ui -run 'TestPinned|TestMainDriverPinned|TestConnRecovery' -count=3 -timeout=180s
go -C source/server test ./pkg/agentclient -count=1 -timeout=180s
go -C source/clients/cli test ./... -count=1 -timeout=180s
go -C source/server test ./internal/server ./cmd/cercano -count=1 -timeout=180s
```

A server build also passed with output in the disposable artifact directory
`/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/pinned-review-bnenmicr`.
No earlier baseline artifact was identified inside the worktree; its historical
pathname was not supplied. A fresh safe Go overlay (`overlay.json` in that
directory) restored HEAD versions of client.go and pinned_session.go and isolated
the new idle-host-exit test as `TestNewTestPinnedIdleHostExit`. It failed as
expected after five seconds: `idle transport loss not detected: connected`.
The worktree was not modified to reproduce the baseline.

## Reproduced blocker: recovery after a failed idle attempt

The added cached-error guard in `pinned_session.go` rejects every later
non-nil failed-connection recovery request until explicit SDK recovery occurs.
A disposable loopback probe stopped the host, waited for Failed, restarted the
same retained host at the same endpoint, then invoked the shared demand recovery
path. It still returned the old `context deadline exceeded`. Explicit
`ReconnectPinnedSession` immediately succeeded. The CLI production sources have
no caller of that API (only an ordinary-generation test calls it). Thus the
message telling users to reconnect explicitly is not backed by an in-session
manual reconnect action, while subsequent demand cannot clear the cached error.
This is not covered by the passing candidate suites.

Reproduction (expected failure):

```sh
go -C source/server test -overlay /var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/pinned-review-bnenmicr/demand-overlay.json ./pkg/agentclient -run TestReviewDemandAfterFailedIdleRecovery -v -count=1 -timeout=30s
```

The overlay and probe are retained outside the worktree. No production repair
was attempted in this finalization pass. Approval/checkpoint is withheld pending
an intentional bounded demand/manual recovery design and its UI regression test.

## Scope and limits

The passing tests exercise real loopback transport, idle host exit, fresh
validation barriers/refusals, concurrent detectors, close during validation,
Model.Update status/notices, task/event recovery and intermediate final-response
zero-replay behavior. They do not establish complete recovery UX after a failed
attempt. Source review found a 50ms observation poll after failure (no redial
storm), immutable retained intent, and fresh validation before connection install.
No live-model, native-credential, installed-launcher, deployment or full-server
suite qualification is claimed. Tests/build used disposable fixtures; no live
host/configuration/database operations were performed intentionally.

Two read-only inspection commands also failed: `rg` was not installed in the
shell PATH, and a grep of `source/clients/cli/internal/ui/commands.go` reported no
such file. Inspection continued using grep and actual source paths.
