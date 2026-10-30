# Stopped startup fixture recovery — partial infrastructure only

Recovered task `task_0bcc55a85e84802dc688b13a3affa4e3` on top of builder-admission checkpoint `b67b431d`. Initial scoped status contained exactly four modified files: `main.go`, `production_authority.go`, `production_composition.go`, and `production_lifecycle.go` under `source/server/cmd/cercano`. Their partial edits were retained and formatted, not discarded or replaced with a second application graph:

- Invocation-local local-provider factory used at ordinary startup and subsequent provider rebuilds; nil retains the original runtime-specific factory.
- Invocation-local MCP dependencies forwarded by actual startup, with optional start/stop hooks; nil retains real manager methods.
- Credential admission before calling the credential opener itself (including read-only opens).

Recovery adds refusal coverage for disabled, empty and malformed credential policy, asserting zero opener calls for both writable and read-only requests. Actual `startGRPCServerWithLifecycle` listener/store/catalog failure fixtures now inject and count the local fake provider, rather than constructing the default provider. Existing assertions verify runtime/store/listener cleanup and catalog cancellation. These tests stop before successful serving; they do **not** establish full startup/shutdown safety or success.

## Validation

From `source/server`, after the recovered edits and test changes:

```text
GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1 go test -race ./cmd/cercano ./internal/hostsvc/providers -run '^Test(ProductionCredentialRefusesBeforeOpen|ProductionListenerFailureClosesOwnedStore|ProductionFailureStages|ProductionProviderActivationBeforeDependencies|BuilderCredentialAdmission|CloudBuilderInvocationAndCredentialBoundary)$' -count=1 -timeout=120s
```

Passed both packages. `git diff --check` passed. Builder refusal regressions remain intact; no gate was bypassed. No full suite, successful startup/shutdown, enabled production startup, compiled acceptance or live qualification was run. The provider unit positive uses its existing memory-store/fake-builder scope; no new enabled scope was added. The paired provider composition test and multi-host MCP test were deliberately not selected: this recovery does not authorize overlapping full Agents in one process.

## Remaining safety and composition gaps

This is an explicit **partial infrastructure checkpoint**, not Phase 1 completion. A successful ordinary startup fixture still needs all dependencies assembled together and verified under credentials-disabled, empty-cloud configuration, with temporary configuration/history/permissions/accounting paths. Before any enabled startup scope, native opener and transport traps must cover every reachable dependency, not only the injected builder. HOME isolation alone is insufficient.

The new local-provider factory does not replace registered Ollama engines/embedders; any reachable engine or embedding transport also needs isolation. Runtime, enterprise, catalog, watchdog, MCP, credentials, cloud builders and worker/process paths must be verified invocation-local together. The MCP lifecycle hooks are retained infrastructure, not yet exercised by a successful full-startup fixture. Startup launches background cloud rebuilding and watchers, installs ordinary execution policy, and constructs an Agent with process-global policy interactions; their cancellation/drain and policy restoration ordering require a single-host positive startup/shutdown test and later-stage failures. Early-stage cleanup coverage is not a substitute. Do not enable a cloud scope to get past the builder or read-only credential gate.

Canonical activation remains disabled. Production defaults outside injection and policy precedence are retained. No installed launcher changes, migration, native credential access, live service/process operations, model spending, push or merge were performed. No plan checkbox is advanced by this recovery.
