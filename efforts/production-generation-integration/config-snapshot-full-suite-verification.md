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
