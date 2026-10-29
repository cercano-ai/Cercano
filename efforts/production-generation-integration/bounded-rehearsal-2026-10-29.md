# Bounded compiled A/B rehearsal — 2026-10-29

Status: PASS for the user-approved bounded synthetic rehearsal, NOT full ordinary-app production acceptance or live cutover approval.

## Exact candidate and evidence

- Candidate: `a3acd52a76a9cb707c5e17966da0e9809a5016be` on `feat/production-generation-integration`.
- Harness-created committed source snapshot: `2f0934095bd247b14607c0b994637b58594b59d6`; source tree `d3875f0784ceab3f6e2a01e944dcf1ea8bcfee17`.
- Tagged dependency fixture SHA-256: `1175ea81a962748d1063abdd75dbf485fab951671637af4c626eee2868dfadcd`.
- Final artifacts: `/tmp/cercano-bounded-a3acd52a-PASS/` (`compiled.json`, `summary.json`, PTY captures, worker/provider/MCP records, synthetic reopened rows, host logs and build records). Tokens are not exported; only their hashes are recorded. Private fixture roots remain for inspection.
- Final parent command exited 0 in 66.088s:

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off \
python3 efforts/dev-host-generations/everyday-launcher-compiled.py \
  --repo /Users/bryancostanich/git_repos/cercano-ai/Cercano-production-generation-integration \
  --evidence /tmp/cercano-bounded-a3acd52a-PASS
```

The same journey passed at `73d506cb3309`; final candidate additionally requires a durable assistant reply after retirement and exports notification flags/count. The source snapshot is built afresh; compiler wrapper adds explicit fixture tags, not replacement host/CLI implementations.

## What ran

Real shell launcher → Python publisher/launcher → compiled `development-launch` → validated compiled `development-host` → shared production composition and actual ownership-aware lifecycle → compiled terminal and pinned workers. Synthetic injection replaces provider, credential/runtime dependencies and external MCP peer, not ownership, task execution, persistence, permission decisions, settings RPC, transfer, shutdown or launcher lifecycle.

1. A ordinary terminal request and durable history.
2. An independently accepted A task held at a synthetic provider barrier.
3. Source-change publication/start of B while A and its task remain alive.
4. New terminal session on B.
5. Fresh A work and a newly entered A worker after B's publication boundary; PID, executable path/digest, manifest and generation assertions distinguish this from pooled reuse.
6. Real MCP tool call through ordinary manager, worker bridge and terminal permission reply. Authenticated public `UpdateConfig` changes loop limit 200 → 1. A subsequent actual worker request executes MCP and then presents production's one-step-limit instruction with no further tools; the setting is restored through the RPC. The settings client is a tagged CLI helper using public client APIs, not a mocked server or direct database mutation; this is not settings-modal keyboard acceptance.
7. A task reaches durable completed state; `notification_pending=0`, `wake_claimed=1`, and one completion-notice event. A retains the conversation until explicit reopen.
8. Busy `/resume` refuses without a pending destination reservation or deferred transfer. Physical B-client restart followed by explicit ordinary `/resume` transfers the idle original conversation; original history is displayed and new turns persist under that conversation.
9. Release A's spare session, explicitly retire A, observe A process exit BEFORE fixture cleanup, then require B's new user turn AND assistant reply in durable history.

One explicit private config authority is passed through the launcher and bound into records/selections; A and B use that same file and its adjacent fixed MCP definitions. No live config is selected. Host construction uses ordinary `config.Load` and watchers; the supplied lifecycle store remains exact.

## Local checkpoints

- Reused `4dbae0cbfe75` settings and `f81eae89ad92` per-host MCP.
- `23f5a5ef49d70249433f63234155d50f927920cc`: explicit isolated authority, record/reuse binding, validated bounded startup admission, CLI metadata preservation.
- `2cf82468ce6e`: child admission/context and producer joins. Actual rehearsal first reproduced a task returning its result but failing with stale ownership; a small regression reproduced this before the fix.
- `3cca80a1b5b9`: successful retirement wakes the actual host owner; failure/incomplete cleanup does not. Prior rehearsal returned ready but left A alive. Regression removes the owner select case to reproduce the failure.
- `73d506cb3309`: compiled MCP/settings/retirement harness and helpers.
- `a3acd52a76a9`: durable B-reply assertion and exported completion evidence.

Earlier failed runs retained at `/tmp/cercano-bounded-20261029-{1,2,3,4}`: initial two failures were PTY permission-label matching (ANSI styling), third exposed child authority failure, fourth exposed missing retirement-to-owner notification. They are not acceptance passes. Run 5 passed, followed by two checkpoint reruns.

## Other checks

All parent Go checks used `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`.

- `python3 source/server/scripts/test-launcher-safety.py`: 18 tests PASS (existing SQLite ResourceWarnings remain).
- `go test ./cmd/cercano -run '^TestDevelopmentLaunchAuthenticatedConcurrentReuse$' -count=5 -timeout=90s`: PASS.
- `go test -race ./internal/ui -run '^TestDevHost' -count=1 -timeout=90s` in CLI module: PASS.
- Final focused race command, twice, PASS in both packages:

```sh
go test -race ./cmd/cercano ./internal/server \
 -run 'TestDevelopmentConfigAuthority|TestDevelopmentHostCommand|TestDevelopmentSelectionConfig|TestBounded|TestProductionGenerationSettings|TestProductionMCPPrepared|TestDevelopmentHostRetirementWakesOwner|TestPreparedDispatch.*|TestPreparedScheduling' \
 -count=2 -timeout=120s
```

- Server `go build ./...`: PASS before retirement fix; final compiled harness rebuilds the actual server/CLI after that fix.
- CLI `go build -o /dev/null .`: PASS.
- Child reports affected six package suites and focused dispatch races passed; retirement handler/owner tests passed with race count20. Parent final focused race above independently covers the integrated paths.
- Bounded source reviews found no concrete blocker in startup binding or child scheduling lifetime. Retirement failure handling has injected handler/owner tests, not a failure-injected compiled journey.
- Full repository suites were NOT rerun for this milestone. Separately compiled application children are NOT race-instrumented.

## Activation and explicit limits

`productionStartupAdmission.validateActivation` still rejects general canonical generation activation. The real launcher mints an invocation-local bounded binding only after exact executable/digest, private authority paths, prepared compatibility and lifecycle identity validation. The production constructor verifies it before loading config/opening dependencies and passes the common activation gate. No test-only activation switch or removed compatibility check is used. Direct lower-level constructor tests explicitly supply test-local bindings and are not substituted for compiled-entrypoint evidence.

This remains the bounded generation mode, not unrestricted ordinary parity. Native credential/issuer behavior, real loaded-model/runtime administration, enterprise-managed accounts, live-history adoption, nested delegation, arbitrary MCP singleton constraints, MCP-definition editing, platform/crash qualification, and unsupported settings remain outside acceptance. Supported loop-limit behavior is proven; elision behavior is not independently exercised here. Unsupported shared settings return explicit errors. Read-only credential restrictions remain. Historical selections lacking authority binding fail closed; they are not automatically rewritten or migrated.

Cleanup is bounded and limited to recorded fixture processes, checked against PID/start-time/private-root command identity. Final report has `cleanup.remaining=[]`. Existing user processes, configuration/history and installed launcher were not changed. No native credentials, paid providers, push, merge or release publication was used. Generation publication occurred only inside disposable test state as authorized.

## Minimal first real-model test deployment (not executed)

Do not replace `~/bin/cercano`, change canonical defaults, or adopt existing databases. Use this exact untagged candidate through the repository launcher with a NEW private prepared state, an explicitly authored `STATE/config.yaml`, fixed reviewed MCP definitions and a short private runtime root. No automatic copying of live settings: preparation must run with a synthetic HOME containing only the authored test config. Set `CERCANO_DEVELOPMENT_CONFIG_AUTHORITY` explicitly for every A/B launch.

Before running, select one already-downloaded local model and its runtime. Start a dedicated protected modelhost for the test (do not attach to or restart the user's current modelhost), explicitly configure its run directory in the test state, and use `open_only` with no cloud profiles. No downloads or paid traffic. The untagged constructor currently opens the native credential service read-only even for local-only mode: this requires separate explicit permission, or a separately verified change to avoid unnecessary opening; the synthetic result is not native-access qualification.

Then launch A from the repository wrapper, keep a short independent task active, publish B from a separate source revision without modifying A's generation, and repeat the observed session/MCP/settings/transfer/retirement sequence. Restrict shutdown/cleanup to those newly created test processes. Retain all existing development history; use only the new disposable database.

Permission request to make before executing: approve creation of one named private test state/runtime directory; launching/stopping only its A/B hosts, workers, MCP peers and dedicated protected modelhost; read-only use of one named already-downloaded model; and, if using the current untagged constructor, native credential-service opening in read-only mode. Ask the user to name the model/runtime. This does NOT request live launcher replacement, migration/adoption, existing-process termination, cloud traffic, push or release publication. If read-only native opening is declined, report that exact constructor boundary rather than enabling fake dependencies in a supposedly real-model deployment.
