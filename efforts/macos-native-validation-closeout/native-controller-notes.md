# Bounded production native-controller prerequisite

This is a Phase 4 prerequisite, **not the A/B Homebrew rehearsal or runtime dependency-retention acceptance**. Only the controller test and this note are in this checkpoint. No production changes, packages, VM settings, live agent state, models, downloads, or credentials are needed.

## Reproduce (macOS, repository with cached Go dependencies)

From `source/server`:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-p=2 GOMAXPROCS=2 \
  CERCANO_NATIVE_CONTROLLER=1 \
  go test ./cmd/cercano -run '^TestNativeProductionController$' -count=1 -v -timeout=180s
```

The opt-in variables belong to the test controller, not product switches. Optional `CERCANO_NATIVE_AGENT=/absolute/path/to/cercano` selects a supplied product artifact (copied, never modified); Go build metadata must identify the product entrypoint and no build tags. The default builds the actual untagged entrypoint offline with a 90-second deadline and two-way compiler concurrency. The supplied-artifact mode currently still performs that bounded default build first. `CERCANO_NATIVE_STATE_PARENT=/tmp` selects the disposable state parent; the controller creates a unique private `nc-*` child and removes it only after successful process cleanup and a passing test. Keep the resolved runtime-root path within the product's 38-byte limit. The test logs canonical keg identity and hashes. It is guest-compatible but this run did not access the guest.

## Mechanism and assertions

Reuse `installed_cold_host_test.go` for isolated prepared store, canonical fixture `Cellar/cercano-agent/A/bin/cercano`, `development-launch --installed`, persisted admission/selection and authenticated client connection. Reuse `loaded_hold_acceptance_test.go` for a fake provider seeded before the authenticated coexistence-protected modelhost starts. Public `PrepareDevelopmentStore`, `modelhost.WriteRecord`, `NewServer`, and agentclient conversation/stream/persistence APIs retain their normal authorities. No memory secret store is injected into the product; child environment is allowlisted.

The test-owned loopback endpoint implements OpenAI SSE completions with fixed `native-controller-ok`. Local `open_only` configuration advertises the seeded llama endpoint and routes Ollama discovery to the same owned HTTP service (non-completion requests return 404), avoiding a live default localhost service. Synthetic confirmed capacity is 32768 tokens. No model file is opened. Provider start count must remain exactly one (the seed).

Before dispatch, assert exactly one owned `development-host` and zero workers. After dispatch, assert the same host and exactly one fresh worker, with macOS `lsof -a -p PID -d txt -Fn` mapped-text executable identity matching canonical A. The role inventory includes every command using the exact fixture executable, so unexpected roles fail rather than being hidden. Inventory is observation, never signal authority.

Stream tokens cannot establish success: require exactly one `TypeDone` with final exactly `native-controller-ok`, no stream error, exactly two persisted turns in user/assistant order with exact contents, and exactly one completion request. Provider start count remains exactly one.

Teardown uses existing authenticated `launchControls`, `LifecycleRelease` with the selected owner and management authorization, then `LifecycleRetire` with a ready response. There is no discovered-PID signal or fallback kill. Keep synthetic HTTP/modelhost services alive through retirement, then close them. Poll inventory for absence within a 15-second cleanup budget. Any uncertain cleanup/inventory failure fails visibly, reports residual roles/PIDs, preserves private state/logs, and requires escalation. Any other test failure also preserves state. A launch failure without a valid selection is not authority to signal a detached child.

## Observed verification (review correction)

Before edits, an isolated source assertion probe exited 1 with four FAILs: snapshot signaling present, exact completion-count assertion absent, separate final validator absent, role inventory assertion absent. After edits the same probe exited 0 with four PASSes. This is a structural red/green probe, not a claimed pre-fix behavioral unit run.

`TestNativeControllerAssertions` exercises token-only success, substring final, duplicate/empty final, error after final, missing/reordered/extra/wrong persisted contents, missing/extra/preexisting workers, changed host and non-fresh worker. Focused offline unit run passed (package 0.537s).

The first corrected opt-in run exposed a teardown ordering defect: ordinary deferred gRPC shutdown preceded lifecycle cleanup, causing a detached `model-host` to be started during retirement. That run failed visibly (package 20.456s), without signaling, and preserved `/private/tmp/nc-1000385746`. Host 99642 and worker 99644 retired, but model-host PID **99649** remained. The harness now retains fixture services until retirement completes.

Final bounded offline command (same environment as above):

```sh
go test ./cmd/cercano -run '^TestNative(ProductionController|ControllerAssertions)$' -count=1 -v -timeout=180s
```

Result: PASS, package 9.151s, production-controller test 8.61s; assertion subtests all passed.

- Host before turn: 99917, zero workers; fresh worker: 99919.
- Artifact SHA-256: `4b7fc086caaa4b26e721576e5714f1284ec23058336f0570a11c7c6122dbb04a`.
- Mapped worker text: `/private/tmp/nc-3173945982/Cellar/cercano-agent/A/bin/cercano`.
- Exactly one completion, one exact successful final, and two exact ordered user/assistant turns; conversation `277da4308ad96051633dff7dfd224370`.
- Cleanup logged `owned process cleanup complete (no signals)`; final-run fixture removed.
- Subsequent observation-only process inventory found no final-run processes, but still found **99649 `/private/tmp/nc-1000385746/Cellar/cercano-agent/A/bin/cercano model-host`** from the failed attempt. Its state/logs remain preserved. This residual requires escalation before further rehearsal; no unsafe fallback was attempted.

Limitations: single generation, deterministic synthetic inference, macOS executable probe, no Homebrew upgrade/cleanup transaction, no real runtime library/model retention, no guest rehearsal or human acceptance. No product/auth changes, live configuration, model/cloud access, or plan changes. The product hash identifies the tested artifact, not a release or signed package.
