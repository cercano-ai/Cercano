# Interrupted fixture recovery — 2026-10-28

Bounded recovery of task_12e5f718c4a947f2eb175fc3cdd0fbac; **four of ten fixtures verified, six incomplete**.

## Recovered state and safety disposition

Initial HEAD was `93872a5f` on `feat/production-generation-integration`. `git status --short` showed only three modified files (development_host_production_test.go, destination_redirect_test.go, routing_contract_repro_test.go) and untracked credential_graph_fixture_test.go. Recent history contained no fixture checkpoint. The tracked diff was 68 insertions/30 deletions.

The interrupted CLI changes added scoped credential enablement around `newProductionDevelopmentHostWithDependencies` in opener/runtime error tests. Although injected errors stop later construction, this still wraps production startup, explicitly prohibited by credentialtest.Enable's contract. Removed those interrupted edits by restoring that exact file from HEAD before execution. No CLI tests were executed. Original positive tests remain unchanged, not skipped or deleted. Production admission is unchanged.

Retained the four provider-graph call sites and memory/fake helper. Reviewed newTestServer construction, provider rebuild/buildProfile, and complete selected test bodies. Construction occurs with outer credentials disabled; no startup, capability installation, child launching, or inference occurs inside the enabled graph scope. The helper replaces secrets with an instrumented memory store and builder with a fake before enablement. It first asserts disabled refusal with zero secret reads and zero builds, then retains the original graph identity/assignment/redirect/concurrency assertions. Fake Chat and StreamChat panic if called. Added a direct-messages-only route guard: subscription/Bedrock credential resolution can occur before a fake builder and must not enter this fixture. Enable is not itself a native-opener trap and its HTTP guard permits loopback; safety here relies on the bounded graph path and fully fake dependencies, not on the helper alone.

## New execution

Only this command ran, from source/server:

```text
go test ./internal/server -count=1 -json -timeout=3m -run '^(TestDestinationRedirectProviderGraph|TestProviderCandidatesRetainBuiltProfileIdentity|TestProviderGraphRetainsTaskAssignmentsUntilRebuild|TestProviderGraphConcurrentReadAndRebuild)$'
```

Exit 0; all four tests passed, package 0.662s. No full suite, race suite, CLI tests, build, app execution, native credential or live-provider test was run.

Private evidence root (0700): `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/fixture-recovery-5g8ruqbn`. `manifest.json` retains exact argv, cwd and allowlisted environment; `tests.log` retains JSON output; `result.json` records exit. Files created with umask 077. HOME, XDG directories and temp directories were private; only offline Go toolchain/cache paths were retained. Required outer values were CERCANO_DISABLE_CREDENTIALS=1, GOPROXY=off, GOSUMDB=off; GOTOOLCHAIN=local. No inherited provider keys/endpoints/integration opt-ins.

## Retained historical evidence and remaining work

`config-snapshot-full-suite-verification.md` remains historical command evidence, not a new run: server full suite failed, CLI tests/builds passed, and the mode probe passed three tests but failed RemoveCloudProfile. Its private logs were not re-read or re-executed here. HEAD includes a later RemoveCloudProfile fix; this recovery makes no fresh verification claim for that fix.

Still incomplete: all five cmd/cercano fixtures listed in that note (InjectedGraph, OpenerFailsOnceWithoutFallback, RuntimeErrorWithoutFallback, ProviderConstructor, CloudOnlyWithoutRuntime), plus internal/server TestRebuildCloudKeylessBaseURLCarveout. Their earlier disabled-policy failures are not claimed repaired or newly reproduced. They require separate explicit refusal coverage and bounded fake positive coverage, not startup-wide enablement. No global enablement or production code changes were made.
