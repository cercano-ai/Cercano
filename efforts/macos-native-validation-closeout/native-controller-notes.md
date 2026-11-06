# Bounded production native-controller prerequisite

This is a Phase 4 prerequisite, **not the A/B Homebrew rehearsal or runtime dependency-retention acceptance**. Only the controller test and this note are in this checkpoint. No production changes, packages, VM settings, live agent state, models, downloads, or credentials are needed.

## Reproduce (macOS, repository with cached Go dependencies)

From `source/server`:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-p=2 GOMAXPROCS=2 \
  CERCANO_NATIVE_CONTROLLER=1 \
  go test ./cmd/cercano -run '^TestNativeProductionController$' -count=1 -v -timeout=180s
```

The opt-in variables belong to the test controller, not product switches. Optional `CERCANO_NATIVE_AGENT=/absolute/path/to/cercano` selects a supplied product artifact (copied, never modified); Go build metadata must identify the product entrypoint and no build tags. The default builds the actual untagged entrypoint offline with a 90-second deadline and two-way compiler concurrency. The supplied-artifact mode currently still performs that bounded default build first. `CERCANO_NATIVE_STATE_PARENT=/tmp` selects the disposable state parent; the controller creates and removes a unique private `nc-*` child. Keep the resolved runtime-root path within the product's 38-byte limit. The test logs canonical keg identity and hashes. It is guest-compatible but this run did not access the guest.

## Mechanism and assertions

Reuse `installed_cold_host_test.go` for isolated prepared store, canonical fixture `Cellar/cercano-agent/A/bin/cercano`, `development-launch --installed`, persisted admission/selection and authenticated client connection. Reuse `loaded_hold_acceptance_test.go` for a fake provider seeded before the authenticated coexistence-protected modelhost starts. Public `PrepareDevelopmentStore`, `modelhost.WriteRecord`, `NewServer`, and agentclient conversation/stream/persistence APIs retain their normal authorities. No memory secret store is injected into the product; child environment is allowlisted.

The test-owned loopback endpoint implements OpenAI SSE completions with fixed `native-controller-ok`. Local `open_only` configuration advertises the seeded llama endpoint and routes Ollama discovery to the same owned HTTP service (non-completion requests return 404), avoiding a live default localhost service. Synthetic confirmed capacity is 32768 tokens. No model file is opened. Provider start count must remain exactly one (the seed).

Before dispatch, only the host exists. After ordinary authenticated dispatch, require a new worker PID, worker command arguments, and macOS `lsof -a -p PID -d txt -Fn` mapped-text executable identity matching the canonical keg. Require exact streamed final output, persisted user and assistant contents via `ResumeConversation`, and at least one completion request. Owned processes are selected only by the exact unique fixture executable, terminated, escalated if needed, and checked absent; HTTP/gRPC listeners and temporary state are closed/removed. No global process-name kill is used.

## Observed verification

The command above was run with output redirected to `/tmp/native-controller-probe.log` and its status preserved (`result=$?; tail -15 /tmp/native-controller-probe.log; exit $result`). Result: PASS, package elapsed 9.516 seconds, test 9.01 seconds.

- Host before turn: PID 99275; fresh worker: PID 99277.
- Artifact SHA-256: `4b7fc086caaa4b26e721576e5714f1284ec23058336f0570a11c7c6122dbb04a`.
- Mapped worker text: `/private/tmp/nc-4253854416/Cellar/cercano-agent/A/bin/cercano`.
- One completion call; final `native-controller-ok`.
- Persisted user `Reply with native-controller-ok` and assistant `native-controller-ok`, conversation `381a5f714ff61d2154beba3b56fe727f`.
- Cleanup logged `owned process cleanup complete`.

Earlier investigation reported model-resolution failure, insufficient synthetic context, empty SSE output, and an overlong runtime root; these historical probes were not rerun here and are not claimed as newly verified defects. The current run above verifies the corrected fixture through production dispatch, without changing production code.

Limitations: single generation, deterministic synthetic inference, macOS executable probe, no Homebrew upgrade/cleanup transaction, no real runtime library/model retention, no guest rehearsal or human acceptance. The temporary log remains local evidence; fixture state/processes were removed. Phase 4 plan status is unchanged. Source revision is represented by the explicit harness/docs checkpoint; the product hash above identifies the tested artifact, not a release or signed package.
