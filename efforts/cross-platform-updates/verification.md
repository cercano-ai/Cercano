# Phase 1 bounded baseline verification

## Directly observed — 2026-11-03

- Read Phase 1 of `plan.md` and `docs/agent/self-dev.md`. The effort directory contained only `plan.md` and `spec.md`; no previous verification file or saved baseline artifacts were available. No broader audit was performed.
- Read all Go tests and implementation in `source/server/pkg/update` before execution. Tests use temporary cache/install fixtures and local HTTP servers (plus localhost:1 for failure). Successful release checks call `DetectInstallMethod`, which may execute read-only `brew list cercano`: this suite is not fully hermetic, but contains no package installation, live binary replacement or agent launch actions.
- Ran exactly one focused test command, from `source/server`:

  ```text
  go test ./pkg/update -count=1 -timeout=60s
  ok  cercano/source/server/pkg/update  1.297s
  exit 0
  ```

- This directly establishes the update package's existing unit-test baseline, including advisory upgrade guidance and unchanged temporary fixture binaries/symlinks. It does not establish survival of native-manager upgrades, package cleanup, installed host/session continuity or automatic retirement. No failing probe was observed in this run.
- No separate builds, installs, live-agent actions or manual git inspection were performed. Source and specification were not changed.

## Prior reported trace — not independently revalidated here

The recovery task reports that installed executable/resource paths remain replaceable. Development-generation retention is a reuse candidate, not proof that installed discovery, reconnect and later worker starts use complete retained bundles after native-manager replacement or cleanup. These are reported baseline findings, not results of the test above; no saved detailed trace was available in this effort directory to substantiate additional path/line claims.

Reuse boundary: preserve native-manager package ownership; consider development-generation retention and session binding only as narrowly tested implementation candidates. Do not import the old custom-updater branch wholesale. Installed resource completeness, shared-runtime nonreplacement, shared-store admission and safe retirement still need their own evidence.

## Acceptance blockers and remaining scope

Disposable native runners have not been provided/established for Homebrew/macOS, Debian/APT Linux, or Chocolatey/Windows. Missing resources are disposable machines/VMs, fixture native package sources/artifacts and isolated user/session state; Windows locked-file and Unicode/space-path coverage also requires a native Windows runner. No runner provisioning or credential search was attempted. This is an installed-system acceptance blocker, not a platform pass; production signing/publication remains separately unauthorized.

The bounded recovery is complete, but Phase 1 as a whole is not. Installed-discovery, pinned-session, worker-generation, modelhost nonreplacement and formula-rendering baselines were not rerun. Trace and broader baseline tasks remain in progress. Only the disposable-native-environment task is changed to blocked; no existing completion or branch/base identity claim is newly verified here.

## Local continuation — 2026-11-03

### Authorization clarification

The current user explicitly authorizes continuing local implementation with native acceptance deferred. This bounded turn is **baseline documentation only**, not source implementation. Native installed-system acceptance remains mandatory before support claims, and the native-environment task remains blocked. No live install/config/agent mutation, model startup, installation build, credential provisioning, publication, push or merge was performed.

Read AGENTS.md, docs/agent/self-dev.md and the effort plan/spec/verification. The following is fresh source inspection, not native execution evidence.

### Installed path and reuse boundaries

- `source/server/pkg/agentclient/client.go:95-125,187-201,223-266`: ordinary Dial first reuses a reachable server, otherwise locks/rechecks, discovers a sibling `cercano` then PATH, and starts `cercano agent`. There is no retained-bundle selection in that discovery. `reconnect.go:347-366` redials and can call ensureServerLaunched again; ordinary reconnect is not generation pinning.
- `source/server/cmd/cercano/main.go:1187,1299` chooses legacyProductionStartup for ordinary entry; `production_startup.go:75-91` runs coexistence preflight only for nonlegacy admission. `main.go:355-386` constructs a generation SpawnPolicy only with a generation binding. `ordinary_generation_host.go:18-30` validates binding/activation before loading config: useful integration boundary, not proof of installed admission.
- `source/server/internal/worker/spawn.go:106-109,210-222,373-385`: later worker startup resolves through policy and invokes `cercano worker`; unpinned lookup consults the current executable sibling then PATH. `policy.go:60-106,145-154` pins generation binary and manifest contents for explicit generation policy, while zero policy uses legacy lookup. Package replacement/removal can therefore redirect or break later legacy lookup; that is a source-derived risk, not a reproduced failing test here.
- `source/server/pkg/agentclient/pinned_session.go:49-83,113-124`: explicit pinned-session dialing/reconnect validates host, contract/schema and retained session rather than launching a replacement. Reuse that admission boundary, not ordinary reconnection as evidence of continuity.
- Known bundle inputs: host/worker `cercano`, terminal `cercano-cli`, and generation manifest. `source/server/scripts/publish-dev-generation.py:282-288` accepts exactly those three members; this does not prove completeness of external runtimes. `internal/localruntime/llamaserver/provider.go:824` resolves `llama-server` via PATH; `internal/localruntime/mistralrs/provider.go:450` similarly resolves `mistralrs`. External runtime executables, their libraries/assets, model files and configuration authorities need explicit retention/admission accounting. No exhaustive platform dynamic-library/resource inventory was established in this bounded pass; the trace task stays in progress.

### Package commands and remote-only CLI

- `release/homebrew/cercano.rb.in:17-25` currently gives one formula both bin/cercano and bin/cercano-cli and invokes restart-after-upgrade from post_install. Lines 30-35 advertise restart. This is existing behavior to replace, not approved upgrade policy or evidence of split package readiness.
- `source/clients/cli/main.go:124-131` constructs localhost plus configured port and calls auto-launching Dial. The ordinary path inspected does not expose a remote host selector. `agentclient/client.go:128` offers DialExisting as a no-launch primitive, but that alone does not establish standalone remote CLI support. The intended dependency-only convenience package and ownership of the everyday cercano command remain implementation work; do not infer executable renaming from package names.

### Runtime replacement and shared data

- `source/server/internal/modelhost/ensure.go:193-212,249-268` retains a default older-host shutdown/replacement path; RefuseOlderHost and CoexistenceProtection gate it. `cmd/cercano/main.go:955` sets RefuseOlderHost for ordinary startup; `development_host_production.go:70-72` requires coexistence protection. These are different guarantees: refusing an older host is not proof of session-lifetime activation deferral or complete caller coverage.
- `production_startup.go:86-105,154-186` separates preflight/validated generation activation and exact borrowed prepared store from legacy access. `internal/conversation/store.go:392-397` documents fail-closed handling before persistent pragmas/migrations; migration calls still exist at 453-486. Shared config is loaded at `ordinary_generation_host.go:26` and persisted/watched at `main.go:591,621,688`. No proof was obtained that every admitted installed generation is protected against incompatible configuration/schema changes. Runtime-caller and admission trace remains in progress rather than claiming an exhaustive audit.
- Reuse only explicit generation validation, pinned reconnect, worker content validation and protected runtime attachment where tests establish their boundaries. Do not import the old custom updater wholesale, or carry forward acquisition, installation, verification/staging, rollback or mandatory restart machinery. Native managers remain delivery owners; development publication is not a portable production installer.

### Exact fresh test results

All commands had explicit tool timeouts. The selected Go fixture isolation properties were reported in prior context, not independently revalidated in this turn; because Go could not execute, no Go fixture or model process started. The Python rendering suite ran without package installation. No real model launch was requested.

1. From source/server, tool timeout 55s:
   `go test ./pkg/agentclient -run '^(TestBinaryDiscoveryInstalledLayout|TestInstalledAgentReuse|TestPinnedNoLaunchRefusals|TestPinnedTransportAndFreshValidation|TestPinnedTerminalIdentityLifetime)$' -count=1 -timeout=35s`
   Tool failed before test execution: `fork/exec /opt/homebrew/bin/go: no such file or directory`. No test pass/failure or exit code was reported by the runner; this is an environment blocker.
2. From repository root, tool timeout 20s:
   `python3 -B -m unittest discover -s release/homebrew -p test_render_formula.py`
   Exit 0; `Ran 11 tests in 0.012s`, `OK`. Rendering checks only; no native package operations. Passing current rendering tests does not endorse the restart hook.
3. From source/server, tool timeout 45s:
   `go test ./internal/worker ./internal/modelhost -run '^(TestDevHostWorkerGenerationReplacementRejected|TestEnsureRefuseOlderHostAtBothProbes|TestEnsureRefuseOlderHostStillLaunchesAbsentHost)$' -count=1 -timeout=30s`
   Same pre-execution missing-Go error. No tests executed. No toolchain installation/search outside the worktree was attempted.

The earlier pkg/update result above remains historical, not rerun. The local baseline is useful but **not complete**: Go execution, exhaustive resource inventory and remaining caller/configuration audit are unresolved. No new failing behavior was reproduced; no source fix or test fixture was added.

### First Phase 2 failing-fixture recommendation

Add an isolated installed-flow test adjacent to installed_reuse_test.go and worker generation tests: launch a copied test helper from a temporary manager-style A bundle, admit an A session through the actual installed selection boundary, then replace prefix selection with B and remove only fixture-owned A package files. Request a later A worker through the real worker selection path, recording a generation marker and requiring an A-only resource read. Assert the worker still uses retained A executable/resources, never B or hostile PATH; separately assert a new eligible session can use B. Start with the later-worker-after-A-removal assertion as the smallest red test. Current unpinned worker lookup is expected to fail or pick B; this expectation is not a run result. Use fake runtime/RPC endpoints, isolated HOME/TMPDIR/config and test-owned processes only. Extend to missing libraries/assets and interrupted retention after the first failure is captured. Do not use the live Cellar or launch models.
