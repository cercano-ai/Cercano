# Credential-test isolation incident — 2026-10-26

During regression repair, scoped credential enablement was added to worker integration fixtures before their complete tool-assembly dependency chain had been checked. That was unsafe: host-side memory secrets and an isolated HOME do not replace worker capability secrets. Production tool assembly eagerly evaluates openWorkerSecrets(), which calls secrets.OpenKeychain(). With admission enabled this can reach keyring.Open.

## Evidence and limits

Read-only audit task `task_87d1fc9a4f25d06e74b431a59f613c55` inspected only repository source and synthetic retained test logs. It did not inspect Keychain, user configuration, credential contents, process dumps, or runtime environment values, and ran no tests.

Artifacts under `/tmp/cercano-test-fixes-isolated/continuation/`:
- `focused-after.log`: disconnected-completion test executed; worker package passed.
- `server-after2.log`: disconnected-completion worker request and synthetic reply establish execution of the worker path.
- `server-full.log`: worker package passed.
- `safety-focused.log` and `server-safe-full.log`: later credential-disabled runs report the unresolved failures.

Historical temporary test source snapshots were not preserved in those logs. Native-opening entry is inferable if the reported Enable additions were present during these runs, but exact backend invocation/success was not instrumented. The logs do not establish credential reads, writes, deletion, or enumeration; backend initialization side effects cannot be excluded. These runs must NOT be cited as zero-native-access evidence.

Source chain: worker_completion_integration_test.go registered worker.New(); reasoning_diagnostic_test.go constructed the production tool stack; worker_dispatch.go installed capabilities with Secrets: openWorkerSecrets(); secrets.OpenKeychain checked admission then supplied keyring.Open. Opener failure is tolerated by the worker, so passing tests do not prove a successful or absent native opening.

## Containment and repair

The unsafe temporary enablement was removed before the safe full-suite rerun. Five integration tests remain failures rather than being waived. A bounded follow-up is using existing tools/provider factory seams or per-instance secret-opener injection, preserving production defaults and prohibiting native fallback in fake fixtures. Enabled tests must not run until their dependencies are structurally fake-bounded. Production child environments remain credential-disabled.

This incident is a limitation of the execution process, not a claim that credentials were accessed or that no access occurred. No credential inspection, rotation, deletion, or other user-state action is authorized or performed as remediation.
