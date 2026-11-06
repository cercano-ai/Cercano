# Native Ollama selection fixture

`native-ollama.yaml` corrects the selection assumption in the retained native
PTY probe `/tmp/cercano-native-inference.w3E2x1/pty.py`. It is a selection-only
fixture, not permission to run inference or evidence of resource acceptance.
The original evidence and closeout are not rewritten.

## Cause

The historical config is syntactically valid but incomplete for its intended
single-model Chat test. `config.Load` migrates legacy `open_model` only to the
active runtime's **everyday** override (`pkg/config/config.go:973`). Chat's
default quality is **premium**, which selects **most_capable**. The startup
open-model display resolves everyday (`openmodels.Resolver.ChatModel`), not
necessarily the Chat task's tier. Thus startup can report `qwen2.5:0.5b` while
Chat selects a different tier. `open_only` chooses a destination, not a tier.

`providers.service.MainModel` and `Main` resolve the Chat task's tier. With no
most-capable override, `openmodels.EffectiveModel` uses catalog defaults.
Before the production correction, `catalogdefaults.ForRuntime` used llama-server
recommendations for runtimes other than mistralrs, including Ollama. At the historical guest's 8 GiB this
returned `llama_server:catalog:qwen3-14b-q4_k_m`. The configured provider remains
Ollama; the catalog ID is not an Ollama model name. This explains the retained
HTTP 400, without assuming the legacy field overrides every task tier.

CLI `mainAgentDriver.Submit` submits conversation/input/workdir, not a global
model override. Server `StreamProcessRequest` seeds worker conversations with
`providerSvc.PrimaryModel()` (`internal/server/server.go:4213`) and restores
any saved session route. `runner.Core.RunTurn` uses the provider's task-bound
model; worker `streamOpenProvider.StreamChat` marshals that request to the host.
The persisted catalog ID therefore reflects the selected Chat model, rather
than a persistence layer substituting an unrelated model.

## Correction and regression

Keep the explicit Ollama everyday selection and assign Chat `quality: standard`
with `destination: primary`. Alternatively, an explicit Ollama `most_capable`
override supports premium Chat. Do not globally replace missing tiers with the
everyday model or silently switch backends.

`TestNativeFixtureSelection` loads real YAML and the real catalog with fixed
8 GiB RAM. It covers rejection of historical missing Chat quality, corrected standard
Chat,
explicit premium override, and the valid native catalog default. It checks the
conversation seed model and, for explicit Ollama cases, the task-bound model
through an in-memory provider boundary. It does not run an Ollama process.

Historical fixture-only failing-before probe (before the production correction):
remove `task_assignments` from this fixture and run
`go test ./internal/hostsvc/providers -run TestNativeFixtureSelection/corrected_standard_chat -v -count=1`.
It fails with everyday=qwen, chat_quality=premium, main=the catalog ID. Restore
the assignment and the regression passes. The probe restored bytes in finally.

Verified with downloads disabled (`GOPROXY=off GOTOOLCHAIN=local`):

- `go test ./internal/hostsvc/providers ./internal/openmodels ./pkg/config -count=1`
- `go test ./internal/server ./internal/runner ./internal/worker -run 'Test(ResolveTierModel|DispatchModelFor|GetConfig_ModelTiers|UpdateConfig_ModelTier|SessionModel|ManagedWorkerPreservesExplicitModelSelection|ManagedHostRequiresSelectionAwareWorker)' -count=1`

These are local selection/interface tests, not successful native inference,
a real CLI saved/reopened conversation, or release acceptance. Persistence
coverage here asserts the seed selection; existing session-model integration
tests cover routing separately.

## Production correction: runtime-compatible automatic defaults

`catalogdefaults.ForRuntime` now returns defaults only for runtimes with a
curated catalog: llama_server and mistralrs. The existing mistralrs embedding
exception remains unchanged. Ollama (and unknown runtimes) return no defaults;
explicit per-runtime model overrides still win. There is no invented Ollama
model name, task-bound premium-to-everyday downgrade, or runtime switch.
The legacy non-task `DispatchModelFor` everyday fallback is unchanged; task-bound
Chat uses the strict resolver and `DestinationModelFor` instead.

The existing absent-model handling now rejects the historical open-only Chat:
`Main` reports `locus mode "open_only": no local provider available for main work`;
actual `dispatch.Engine.Dispatch` reports `dispatch: selected task model unavailable`.
Configure an Ollama most_capable override for premium Chat, or explicitly select
standard quality to use the everyday override. Existing locus fallback policy
is unchanged for modes that permit cloud fallback.

Before editing production code, `TestForRuntimeCompatibility/ollama` failed
with the real 8-GiB catalog returning llama_server IDs for every tier, and the
historical fixture logged everyday=qwen2.5:0.5b, chat_quality=premium,
main=llama_server:catalog:qwen3-14b-q4_k_m. After correction the compatibility
test passes, the historical main model is empty, and both error paths above
are asserted. Explicit standard and premium cases still bind and invoke their
separate Ollama model names in memory; native catalog selection is unchanged.

Additional focused verification (downloads disabled as above):

- `go test ./internal/localruntime/catalogdefaults ./internal/openmodels ./internal/hostsvc/providers ./internal/dispatch ./pkg/config -count=1`
- The server/runner/worker interface tests listed above.
