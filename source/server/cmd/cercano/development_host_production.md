# Prepared production development-host dependencies

The factory consumes `state/config.yaml` (defaults plus YAML, deliberately not
legacy environment-merging/migrating config loading). Config and credentials are
read-only. The default constructor opens the native keychain read-only **after
coexistence preflight**; tests inject `secrets.NewMemory()`. Construction errors
are returned, not hidden behind fixture or open-provider fallback.

The same prepared durable server supplies the agent, ADK coordinator, inference
runners, toolstack dispatch engine, capability registry and pinned prepared-worker
runner. Authenticated lifecycle readiness and peer controls use the invocation's
`auth.Authorize` and state-bound `developmentPeerDial`. Diagnostics live under
`state/hosts/<host>/`. No startup migrations, retention sweeper, config watchers,
or legacy database are installed.

## Cloud-only setup

A prepared private state needs an explicitly provisioned `config.yaml` in
addition to its coexistence database. For example (use an appropriate real
configured profile and credential provisioned separately):

```yaml
locus_mode: cloud_only
active_cloud_profile: work
cloud_profiles:
  - name: work
    provider: openai
    flavor: chat_completions
    model: your-configured-model
```

Cloud-only constructs **no local runtime, local inference provider or embedding
engine** and requires no `modelhost.yaml`. The managed dispatch graph selects the
configured cloud route. There is no substitute open fallback. Configured profile
construction failures remain fatal. Legacy embedding-classification routing is
not available in this mode.

Cloud profiles use the real provider service and `cloudfactory.BuildCloudProvider`
(messages, chat_completions, responses, Bedrock); subscription routes use guarded
read-only token sources. Static keys are resolved by profile name. Legacy-only
configuration is not migrated. Cloud-only does not prove external credential,
refresh, network or inference success.

## Local lanes: explicit prewarm and attach only

`open_only` and mixed modes require an already-running, API-compatible,
**coexistence-protected modelhost**, with the selected models already configured
and prewarmed. State must contain a private regular `modelhost.yaml`:

```yaml
run_dir: /absolute/canonical/private/protected-modelhost-record-directory
```

The directory must already exist, be private and canonical (no symlink aliases).
It pins the exact modelhost **record directory**, not ambient machine discovery.
The connector verifies the host's authenticated protection assertion. Attachment
is bounded and failclosed. Supported local engines are `llama_server` and
`mistralrs`; no Ollama, in-process runtime, automatic launch/replacement, or
unprotected modelhost fallback is supported.

**Local cold starts are unsupported by this host.** Ordinary legacy modelhost
setup does not establish the protection/prewarm prerequisite. An operator must
separately provision a protected participant before enabling local lanes; do not
point this resource at an ordinary unprotected host or expect this command to
upgrade it. This factory never creates or replaces a real runtime.

## Invocation-local compiled acceptance seam

`developmentProductionDependencies.newOpenProvider(manager, config)` can return
a synthetic `inference.Provider` alongside an injected inert `connectRuntime`
manager and memory `openSecrets`. It feeds the real production dispatch graph;
no replacement server is necessary. Constructor errors/nil providers fail closed.
Without injection the real local engine adapters and real cloudfactory remain
unchanged. This local-provider seam does not override native cloud construction.

`development_provider_defaults.go` defines `developmentProductionDefaults` under
`!cercano_development_acceptance`. A compiled acceptance helper may provide the
same narrow symbol in a file tagged `cercano_development_acceptance`, returning
invocation-local dependencies for the **same** production factory. Its runtime
connector signature is `(context.Context, config.Config, state, host string)`.
No acceptance implementation or production fake default is included here. The
normal binary has no environment-enabled dependency override; tagged builds
without a helper intentionally fail to compile.

## Focused verification

From `source/server`:

```
go test ./cmd/cercano -run '^TestDevelopmentProduction' -count=1
```

These probes pass (2026-10-26): authenticated gRPC identity plus LifecycleList
readiness on injected open/cloud graphs, real Read builtin inside durable
foreground admission, cleanup/config preservation, cloud-only with unusable
local runtime and no attachment resource, injected synthetic provider reaching
production dispatch, constructor failure cleanup, missing explicit attachment,
preflight-before-opener, unsupported snapshots, opener failure, and runtime
failure without fallback. Tests use memory credentials/inert managers and do not
call native credentials, model inference or child processes. The separately
owned coexistence contract correction is present in the working tree for these
probes; this checkpoint does not own it. Actual compiled-launcher acceptance and
real protected-runtime/cloud inference remain separate evidence.

Permission decisions remain strict and session-local: no legacy permissions
watcher or persisted allowlist.
