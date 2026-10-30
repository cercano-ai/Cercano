# Background admission/status recovery — 2026-10-27

Recovered the uncommitted ordinary-host slice left by task_38a336504afefeec1e7be008c07b4324. Read shutdown-decision, shutdown-caller-obligations, shutdown-join-followup and latest compaction-scheduling evidence. Prior tool outcomes were not assumed to pass. Existing joins and owned-compaction implementation were not redone.

## Recovered and finished

- Existing edits added synchronous atomic task/completion/recap/compaction admission fences, including rejection of queued producer execution; admitted callbacks retain dependencies until the existing joins finish.
- Existing protocol/generated Go edits add observation-only shutdown, complete/phase/nonsecret incomplete reason. The ordinary gate exempts only ShutdownAgent and still invokes subsequent interceptors; enterprise policy admission now covers that method. Client acceptance no longer means completed disposal.
- Existing ordinary lifecycle wiring retains one disposer and joins RPC/background work before dependency teardown. Recovery additionally moved graceful transport drainage after runtime/resource disposal, so blocked disposal does not prematurely remove status transport. Final enterprise/policy release still follows transport drainage.
- Added authenticated bufconn coverage to the recovered remote test: observation does not initiate shutdown, status remains reachable after the work gate closes, missing credentials are rejected, ordinary work is rejected, and caller reason is not echoed. This uses an isolated temporary devhostauth token and does not change production authentication composition. No forced transport stop is used by the new remote/client fixtures.
- Prepared/development-host changes, their dependent composition/provider tests, and shared plan edits are excluded. Producer files contain only recovered fence additions, not a rerun of the completed compaction project.

## Actually executed in this recovery

All Go commands used `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`.

From source/server, final race run passed (all six packages, exit 0):

```
go test -race ./cmd/cercano ./internal/server ./internal/enterprise ./internal/recap ./internal/compactiongen ./pkg/agentclient -run 'TestProduction(SingleHostStartupShutdown|IncompleteShutdownControl|FailureStages|ListenerFailureClosesOwnedStore|RemoteStatusDuringBlockedCallback)$|TestTaskShutdown|TestShutdownControl|TestShutdownStatus|TestProducerFence' -count=3 -timeout=120s
```

`go test ./... -run '^$'` passed in source/server and source/clients/cli (generated bindings/client/mocks compile; not full-suite execution). Gofmt ran on scoped Go files; `git diff --check` passed. Generated Go was recovered and compiled; protoc-gen-go was not on PATH, so fresh regeneration is not claimed. No live activation, helper process, native credentials, native inference, forced termination, or user-data migration was performed. Canonical activation remains disabled.

## Concrete remaining limits

- Remote transport disappearance is unknown, not proof of completion or permission to replace. Final authority cleanup occurs after transport drainage; the retained local handle remains the final completion authority.
- Authentication is inherited from the configured transport/interceptors. The authenticated fixture proves the shutdown exemption does not bypass an installed authenticator; ordinary legacy composition has not gained a new token/mTLS boundary. Enterprise policy rejection has separate coverage, not a full managed-enterprise network acceptance test.
- These fences qualify task/completion and recap/compaction scheduling only. Watchdog/catalog/retention/provider-watcher-wide prompt no-new-work qualification and a combined blocked-runtime-disposer remote integration probe remain outside this recovered evidence.
- Existing UI request-error propagation and prepared/development-host retirement are not claimed fixed by this slice. The prepared worker owns its files. Full production/CLI suites and broad generation acceptance remain outstanding; the historical credential-disabled broad production-suite failure is not superseded by these targeted passes.
- Timeout never releases authority, reports retirement, or permits replacement. Repeated requests retain the same eventual exactly-once disposer. No activation gate or approved specification was changed.
