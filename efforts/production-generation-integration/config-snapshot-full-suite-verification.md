# Config snapshot isolated full-suite verification — 2026-10-28

## Snapshot and disposition

Verified branch `feat/production-generation-integration` at `381d10e29aa33c7226f3091f6e4fdd6e7018f63d`, a documentation-only descendant of candidate `fd55a88d2aed78836832781998dba8dac0887bfd`. `git diff --exit-code fd55a88d2aed HEAD -- source` passed. Initial working tree was clean. No production/test source changed. This note is the only repository change from this verification; checkpoint is documentation-only, with no landing or push.

**Not green:** full server suite fails. CLI full suite and both builds pass. No race-suite claim is made here (prior parent races are separate evidence). No live-provider, native-credential, installed-app, deployment, all-platform or acceptance qualification is claimed.

## Safety and exact execution

Read root AGENTS and the credential-test isolation incident and snapshot effort evidence. Enabled fixture paths inspected use memory stores, fake credential fetchers/provider factories, or injected secret openers; notably in-process worker fixtures use `NewWithSecretStore` and the reasoning diagnostic injects its capability-secret opener. `credentialtest.Enable` alone does not make native openers safe. Outer credential policy remained disabled; tests with scoped enablement are bounded fake fixtures, not authorization for native credentials. Existing test-owned child harnesses ran; compiled launcher acceptance and live integration opt-ins stayed unset. No production app was launched. This is source-bound safety evidence, not OS-level tracing of every syscall.

Private root: `/tmp/cv.L4sqGn` (0700), created before execution. Logs and harness are retained there, not committed; original paths remain unchanged. Offline Go cache/toolchain locations are the only preserved user-home resources. Environment was constructed from an allowlist, not inherited, so provider keys, provider endpoints, proxy variables, integration flags and live app/config variables were absent.

Exact child environment (nonsecret):

```text
PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
HOME=/tmp/cv.L4sqGn/h
XDG_CONFIG_HOME=/tmp/cv.L4sqGn/c
XDG_DATA_HOME=/tmp/cv.L4sqGn/d
XDG_STATE_HOME=/tmp/cv.L4sqGn/s
XDG_CACHE_HOME=/tmp/cv.L4sqGn/k
TMPDIR=/tmp/cv.L4sqGn/t
TMP=/tmp/cv.L4sqGn/t
TEMP=/tmp/cv.L4sqGn/t
CERCANO_DISABLE_CREDENTIALS=1
GOPROXY=off
GOSUMDB=off
GOTOOLCHAIN=local
GOCACHE=/Users/bryancostanich/Library/Caches/go-build
GOMODCACHE=/Users/bryancostanich/go/pkg/mod
GOPATH=/Users/bryancostanich/go
GOROOT=/opt/homebrew/Cellar/go/1.26.3/libexec
AWS_EC2_METADATA_DISABLED=true
LANG=en_US.UTF-8
```

Outer harness invocation: `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off python3 /tmp/cv.L4sqGn/run.py`. Exact argv/environment/cwd are retained in `manifest.json`; timings/exits in `results.json`. Each Go subprocess had a 1200-second harness bound. Initial umask was 077. Commands, run sequentially:

| Working directory | Command | Exit | Seconds |
| --- | --- | --- | --- |
| source/server | `go test ./... -count=1 -json -timeout=10m` | 1 | 67.33 |
| source/clients/cli | `go test ./... -count=1 -json -timeout=10m` | 0 | 5.71 |
| source/server | `go build -o /tmp/cv.L4sqGn/b/server ./cmd/cercano/` | 0 | 3.22 |
| source/clients/cli | `go build -o /tmp/cv.L4sqGn/b/cli ./` | 0 | 1.55 |

Logs: `logs/server-tests.log`, `logs/cli-tests.log`, `logs/server-build.log`, `logs/cli-build.log`, all under the private root. Binaries were compiled only, not launched by this harness.

## Counts and failures

Counts are Go JSON terminal events, not unique assertions. Including nested subtests, server: **5402 pass, 18 fail, 37 skip**; top-level tests: **3869 pass, 14 fail, 37 skip**. Package events: **114 pass, 3 fail, 12 skip/no-tests**. CLI: **1595 pass, 0 fail, 1 skip** including subtests; top-level: **1328 pass, 0 fail, 1 skip**. CLI package events: **10 pass, 0 fail, 1 skip/no-tests**.

### Isolated-policy fixture mismatches (not relaxed)

These ten top-level tests fail under required outer disabled admission:

* cmd/cercano: `TestDevelopmentProductionInjectedGraph` (`open`, `cloud`), `TestDevelopmentProductionOpenerFailsOnceWithoutFallback`, `TestDevelopmentProductionRuntimeErrorWithoutFallback`, `TestDevelopmentProductionProviderConstructor` (`synthetic`, `error`), `TestDevelopmentProductionCloudOnlyWithoutRuntime`.
* internal/server: `TestRebuildCloudKeylessBaseURLCarveout`, `TestDestinationRedirectProviderGraph`, `TestProviderCandidatesRetainBuiltProfileIdentity`, `TestProviderGraphRetainsTaskAssignmentsUntilRebuild`, `TestProviderGraphConcurrentReadAndRebuild`.

Most report `credentials disabled by CERCANO_DISABLE_CREDENTIALS`; provider-constructor cases report zero constructor calls because admission stopped before construction. The opener/runtime cases likewise report zero calls. These are policy/fixture mismatches, not evidence to enable production credentials or weaken admission. Follow-up must use already bounded fake paths, not a global enablement.

### Ordinary unresolved failure

`internal/server.TestRemoveCloudProfile` fails at `cloud_profiles_test.go:496`: `want Ok, got: configuration saved but not applied: primary has no preferred profile`. This test already enables only a memory-store fixture. It is not explained by the required disabled outer policy. It fails again in the independent bounded probe below. No config semantics or test expectations were changed to suppress this result.

### Initial isolation artifact, separately probed

With umask 077, `internal/capabilities/builtins.TestWriteFileCapability_OverwritePreservesMode`, `TestEditFileCapability_PreservesMode` see 0700 rather than their assumed 0755 input fixture, and `cmd/cercano.TestPrepareDevelopmentCarriesConfig` sees a non-0644 original fixture. Source inspection shows fixture creation modes are filtered by the harness umask before production logic runs.

A follow-up used the same allowlisted environment and private root, **umask 022**, log explicitly chmod 0600, and this exact command from source/server:

```text
go test ./internal/capabilities/builtins ./cmd/cercano ./internal/server -count=1 -json -timeout=5m -run '^(TestWriteFileCapability_OverwritePreservesMode|TestEditFileCapability_PreservesMode|TestPrepareDevelopmentCarriesConfig|TestRemoveCloudProfile)$'
```

The three mode tests **passed**; `TestRemoveCloudProfile` **failed with the same error**. Exit 1. Manifest: `probe-manifest.json`; log: `logs/mode-and-remove-probe.log`. No complete suite was rerun with umask 022, so original aggregate counts above are not rewritten as a green result.

## Skips and limits

All 37 server skip names/reasons are retained privately in `server-tests-skips.txt`: live/paid provider and tool-history integrations, sandbox opt-in, compiled PTY acceptance, subprocess-helper entrypoints, platform-specific installer tests, an aspirational semantic merge case, and `anthropic.TestNormalize_NetworkError` (transport hung until its deadline, making classification meaningless). CLI `overlay.TestCatalogTerminal` skipped; full reason retained in `cli-tests-skips.txt`. Package skip counts also include packages with no tests. No live/compiled opt-in was enabled to reduce skips.

This establishes one isolated full-suite run on source identical to fd55a88d2aed, passing CLI tests/build and server build, with explicit remaining failures. It does not establish complete server regression success, zero external syscall activity by instrumentation, native backend behavior, live model behavior, installed application behavior, production readiness, or completion of the effort.


## Post-repair full rerun — 2026-10-28

**Both full suites and both builds pass in this rerun.** This supersedes the earlier run's unresolved-failure disposition for the source snapshot below, without changing its historical evidence. This is a bounded regression/build result, **not production acceptance** or an independent fixture-safety approval; parent fixture-safety review is separate.

### Exact source and isolation

Tested `f16cc9d2dd432f5002e37d6ae333d60a58e04f79`, including repairs `73d890c8dc54` and `93872a5fdaea` (and bounded provider-graph fixture repair `9b07d5b3`). Initial working tree was clean. The prior `/tmp/cv.L4sqGn/run.py`, this evidence note, root AGENTS, and the credential-test isolation incident were read before execution.

Fresh retained private root: `/tmp/cv.3p1hchdz`, owner UID 501, mode **0700**. HOME, XDG and temporary directories were newly created inside it. The harness kept umask 077 for its own logs/manifests and set **child umask 022** before each Go command, preserving filesystem-mode test semantics without exposing the private root. The prior harness was adapted only for the new root, child umask, and source-integrity recording. Each command retained the 1200-second outer bound and tests retained the 10-minute per-package bound.

Both the outer Python invocation and Go child environment used an explicit allowlist (`/usr/bin/env -i` for the outer invocation), not inherited provider/config environment. Exact environment:

```text
PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
HOME=/private/tmp/cv.3p1hchdz/h
XDG_CONFIG_HOME=/private/tmp/cv.3p1hchdz/c
XDG_DATA_HOME=/private/tmp/cv.3p1hchdz/d
XDG_STATE_HOME=/private/tmp/cv.3p1hchdz/s
XDG_CACHE_HOME=/private/tmp/cv.3p1hchdz/k
TMPDIR=/private/tmp/cv.3p1hchdz/t
TMP=/private/tmp/cv.3p1hchdz/t
TEMP=/private/tmp/cv.3p1hchdz/t
CERCANO_DISABLE_CREDENTIALS=1
GOPROXY=off
GOSUMDB=off
GOTOOLCHAIN=local
GOCACHE=/Users/bryancostanich/Library/Caches/go-build
GOMODCACHE=/Users/bryancostanich/go/pkg/mod
GOPATH=/Users/bryancostanich/go
GOROOT=/opt/homebrew/Cellar/go/1.26.3/libexec
AWS_EC2_METADATA_DISABLED=true
LANG=en_US.UTF-8
```

Outer invocation: `/usr/bin/env -i <the KEY=value entries above, using /tmp rather than /private/tmp for HOME/XDG/TMP paths> python3 /tmp/cv.3p1hchdz/run.py`, from the repository root. On macOS these name the same private root; the harness resolved its own path and used the recorded `/private/tmp` spelling for Go children. No keys, live endpoints, live/integration flags, or compiled-launcher opt-ins were supplied. Required outer credential admission stayed disabled; scoped fake-enabled fixtures are not permission for native credentials. No native credential operation, live startup, or user-config operation was intentionally invoked. Normal suite-owned bounded test children ran; the build outputs were not executed. This run is not syscall-instrumented proof of no native backend access, and does not replace the parent's concurrent fixture-safety review.

### Commands and results

| Working directory | Exact command | Exit | Seconds |
| --- | --- | --- | --- |
| source/server | `go test ./... -count=1 -json -timeout=10m` | 0 | 67.73 |
| source/clients/cli | `go test ./... -count=1 -json -timeout=10m` | 0 | 5.7 |
| source/server | `go build -o /private/tmp/cv.3p1hchdz/b/server ./cmd/cercano/` | 0 | 3.19 |
| source/clients/cli | `go build -o /private/tmp/cv.3p1hchdz/b/cli ./` | 0 | 1.54 |

Counts below are Go JSON terminal events, including nested subtests where indicated; package skips include no-test packages.

| Suite | All test pass/fail/skip | Top-level pass/fail/skip | Package pass/fail/skip |
| --- | --- | --- | --- |
| server-tests | 5431 / 0 / 37 | 3889 / 0 / 37 | 117 / 0 / 12 |
| cli-tests | 1595 / 0 / 1 | 1328 / 0 / 1 | 10 / 0 / 1 |

All 14 top-level failures listed in the original run passed in this full rerun, including `TestRemoveCloudProfile`, the ten disabled-admission fixture mismatches, and the three mode-sensitive tests. Exact names/statuses are retained in `previous-failures-status.json`. There are no failing test/package events, non-JSON test-log lines, or timed-out commands. No policy or expectation was weakened in this verification; no source edit was made.

### Source integrity and retained artifacts

The harness recorded full HEAD, git status, and SHA-256 for every tracked file under `source` before execution and after **each** of the four jobs. HEAD, clean status and every source hash matched at every boundary (`source-before.json`, `source-after.json`, `source-checks.json`). This is boundary verification, not monitoring of transient edits between snapshots. The only subsequent repository edit is this evidence appendix; no production or test source was changed by the verifier.

Private artifacts under `/tmp/cv.3p1hchdz`: `run.py`, `manifest.json` (argv/environment/cwd/SHA/modes), `results.json`, source snapshots/checks above, `summary.json`, `previous-failures-status.json`, `server-tests-skips.txt`, `cli-tests-skips.txt`, and all four `logs/*.log`. Logs and binaries are retained privately, not committed. SHA-256:

| Artifact (relative to private root) | SHA-256 |
| --- | --- |
| `run.py` | `9a1d175ec3deb15e4e76a3117f9f8cc32190c888e6c4873eb237df72c221e9d2` |
| `logs/server-tests.log` | `0ffc2a88e6c6600cf95c65a9b4f52d2b605ad4e58093bf703a73270af9149928` |
| `logs/cli-tests.log` | `0fffc809a02ee56ce429b414df5ba69910af2e554e9d8b0991ca1eb233264847` |
| `logs/server-build.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `logs/cli-build.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `b/server` | `a77e0ea3627a98762977a5ca02f6aa315d8f3b907455c7b2c2a9dcc5767e1e6d` |
| `b/cli` | `f5c0ceabc3638341f73b56c67ad81eb2ea37ded58964eb51e37d72038d8ac01c` |

### Exact skipped tests

No opt-in was enabled to eliminate skips. The complete skip output, with source line/reason, is retained in the private skip files and `summary.json`; the table records every skip here as well. Package names below omit the common `cercano/source/` prefix.

| Package | Test | Reason |
| --- | --- | --- |
| `server/internal/agent` | `TestLiveToolHistoryComparison` | tool_history_live_test.go:27: set CERCANO_TOOL_HISTORY_ENDPOINT to opt into live diagnostic |
| `server/internal/agent` | `TestSmartRouter_Integration_SelectProvider` | router_integration_test.go:20: Skipping integration test; set INTEGRATION_TEST=1 to run |
| `server/internal/cloudfactory` | `TestMistralLiveCodeRead` | mistral_live_code_test.go:21: paid coding-model test requires explicit opt-in |
| `server/internal/cloudfactory` | `TestMistralLiveAdapterSmoke` | mistral_live_test.go:24: paid live test requires explicit opt-in |
| `server/internal/llm/bedrock` | `TestIntegration_Chat` | client_integration_test.go:42: set INTEGRATION_TEST=1 to run live Bedrock tests |
| `server/internal/llm/bedrock` | `TestIntegration_Stream` | client_integration_test.go:66: set INTEGRATION_TEST=1 to run live Bedrock tests |
| `server/internal/llm/bedrock` | `TestIntegration_ToolCall` | client_integration_test.go:101: set INTEGRATION_TEST=1 to run live Bedrock tests |
| `server/internal/llm/openai` | `TestIntegration_Chat` | client_integration_test.go:61: set INTEGRATION_TEST=1 to run live OpenAI tests |
| `server/internal/llm/openai` | `TestIntegration_Stream` | client_integration_test.go:88: set INTEGRATION_TEST=1 to run live OpenAI tests |
| `server/internal/llm/openai` | `TestIntegration_ToolCall` | client_integration_test.go:132: set INTEGRATION_TEST=1 to run live OpenAI tests |
| `server/internal/llm/openai` | `TestIntegration_Vision` | client_integration_test.go:164: set INTEGRATION_TEST=1 to run live OpenAI tests |
| `server/internal/llm/openai` | `TestIntegration_CleanError` | client_integration_test.go:195: set INTEGRATION_TEST=1 to run live OpenAI tests |
| `server/internal/llm/responses` | `TestChatGPTRoute_Stages` | chatgpt_route_integration_test.go:65: set CHATGPT_INTEGRATION=1 to run live codex-backend tests |
| `server/internal/llm/responses` | `TestChatGPTRoute_ReasoningRoundTrip` | chatgpt_route_integration_test.go:143: set CHATGPT_INTEGRATION=1 to run live codex-backend tests |
| `server/internal/llm/responses` | `TestChatGPTRoute_BuildRequestReplica` | chatgpt_route_integration_test.go:197: set CHATGPT_INTEGRATION=1 to run live codex-backend tests |
| `server/internal/llm/responses` | `TestIntegration_Chat` | client_integration_test.go:38: set INTEGRATION_TEST=1 to run live Responses tests |
| `server/internal/llm/responses` | `TestIntegration_Stream` | client_integration_test.go:61: set INTEGRATION_TEST=1 to run live Responses tests |
| `server/internal/llm/responses` | `TestIntegration_ToolCall` | client_integration_test.go:95: set INTEGRATION_TEST=1 to run live Responses tests |
| `server/internal/llm/responses` | `TestIntegration_ReasoningRoundTrip` | client_integration_test.go:120: set OPENAI_RESPONSES_MODEL to an o-series reasoning model to run this |
| `server/internal/llm/anthropic` | `TestNormalize_NetworkError` | normalize_test.go:128: transport hung until deadline; class check not meaningful |
| `server/internal/brewrestart` | `TestIdentityChild` | inspect_darwin_test.go:105: subprocess only |
| `server/internal/brewrestart` | `TestLaunchChild` | launch_darwin_test.go:112: subprocess only |
| `server/internal/compactor` | `TestLongitudinal_MergeGrowsUnbounded_Aspirational` | longitudinal_test.go:191: non-goal: semantic collapse of distinct phrasings requires non-deterministic similarity; out of scope for the pure deterministic merge |
| `server/internal/modelhost` | `TestAbruptClientProcessHelper` | attachment_lifetime_test.go:132: subprocess helper |
| `server/internal/localruntime/llamaserver` | `TestInstallAlreadySatisfied_WingetKnownCodesTreatedAsSuccess` | install_test.go:213: winget exit-code handling is windows-only |
| `server/internal/localruntime/llamaserver` | `TestInstallAlreadySatisfied_UnknownNonzeroExitStillFails` | install_test.go:231: winget exit-code handling is windows-only |
| `server/internal/localruntime/llamaserver` | `TestInstall_TreatsWingetAlreadyInstalledAsSuccess` | install_test.go:244: winget exit-code handling is windows-only |
| `server/internal/localruntime/llamaserver` | `TestDefaultInstallCommand_WindowsUsesWingetWhenPresent` | install_test.go:277: windows-only: exercises the real winget branch of defaultInstallCommand |
| `server/internal/localruntime/llamaserver` | `TestDefaultInstallCommand_UnsupportedPlatformNamesManualInstallURL` | install_test.go:293: darwin has a managed install path (brew) |
| `server/internal/server` | `TestLiveOpenModelDispatch_ReadToolSmoke` | live_open_model_dispatch_test.go:34: set CERCANO_LIVE_OPEN_MODEL_TEST=1 plus CERCANO_LIVE_OPEN_MODEL_BASE_URL and CERCANO_LIVE_OPEN_MODEL_MODEL to run live local/open-model dispatch smoke |
| `server/test/integration` | `TestGenericGenerator_Integration_GenerateValidCode` | generic_generator_test.go:23: Skipping integration test; set INTEGRATION_TEST=1 to run |
| `server/test/integration` | `TestSandbox_GenerateAndRunTests` | sandbox_test.go:20: Skipping sandbox test; set SANDBOX_TEST=1 to run |
| `server/pkg/config` | `TestSnapshotProcessHelper` | snapshot_process_test.go:84: test-binary subprocess only |
| `server/pkg/agentclient` | `TestBinaryDiscoveryInstalledHelper` | binary_discovery_installed_test.go:162: subprocess only |
| `server/pkg/agentclient` | `TestBinaryDiscoveryHelper` | binary_discovery_test.go:81: subprocess only |
| `server/cmd/cercano` | `TestEverydayLauncherCompiledPTY` | everyday_launcher_acceptance_test.go:15: set CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1 for real tagged launcher acceptance |
| `server/pkg/agentclient` | `TestInstalledAgentReuseHelper` | installed_reuse_test.go:148: subprocess only |
| `clients/cli/internal/overlay` | `TestCatalogTerminal` | catalog_terminal_test.go:31: requires a terminal driver |

No race run, live-provider/native-backend validation, installed application/compiled launcher acceptance, deployment check, all-platform qualification, or overall production-readiness claim is made. The result establishes only isolated full server+CLI regression and compilation success for the exact source SHA above under the recorded policy and skips.
