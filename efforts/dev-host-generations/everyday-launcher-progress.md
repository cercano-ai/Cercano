# Compiled launcher acceptance progress

## 2026-10-26 bounded checkpoint — FAILED, not acceptance/landing

- Read `development_host_production.md` and `development_provider_defaults.go`.
  The production symbol split uses `!cercano_development_acceptance`; explicit
  acceptance builds therefore use both `developmentfixture` and
  `cercano_development_acceptance`. The helper requires both tags.
- The helper supplies memory secrets, an inert local manager, and synthetic
  provider through `newOpenProvider`, without replacing the production graph.
- The harness snapshots actual source (including uncommitted helper bytes),
  creates a local snapshot commit for publisher identity, and launches the real
  shell/publisher/compiled host/compiled CLI in a PTY. Its Go wrapper delegates
  every command to Go and adds only build tags. HOME/XDG/private prepared state
  are temporary; outer is disabled, Go dependency downloads disabled.

## Observed gate results

1. `go test ./cmd/cercano -run 'TestEverydayLauncherEntrypointRefusalsPTY|^TestDevelopmentProduction' -count=1`: PASS (0.915s package test time).
2. `CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1 CERCANO_COMPILED_LAUNCHER_EVIDENCE=/tmp/compiled-current-evidence go test ./cmd/cercano -run '^TestEverydayLauncherCompiledPTY$' -count=1`: FAIL (57.168s package test time).
   Ordinary launcher A acquired a compiled generation and the real CLI opened.
   `FIXTURE_DISPATCH` then produced
   `Locus: locus mode "open_only": no local provider available for main work`.
   No inference call or independent task reached the provider barrier within
   40s. `provider.jsonl` contains only the constructor event. Fixture input
   configuration records `llama_enabled: true` and offline model overrides.
   The root cause of provider unavailability is not established; this is not
   yet evidence of a worker-dispatch-authority failure.
3. `go test ./cmd/cercano -count=1`: FAIL (92.693s package test time).
   `TestDevHostTerminalPTY` timed out on missing `entered` barrier and
   `TestDevHostCompiledExecution` received a Locus cloud-provider-unavailable
   response instead of its fixture build marker. These independently owned
   fixtures were not changed here. This command does not include the opt-in
   compiled gate unless its environment flag is set.

The persisted `everyday-launcher-compiled-evidence/` files are from result 2;
no tokens/credential contents are included. Absolute temporary paths in this
record no longer denote live state. The harness cleans its owned subprocesses.

## Blocker and remaining coverage

Communicated production integration blocker: the injected open provider is
constructed, but ordinary foreground work reports no local provider. No edits
to main/factory/server/worker/scripts were made to bypass it. No seeded task
was substituted for independent dispatch.

A/B execution and the downstream task/lifecycle assertions have NOT run.
Additionally the prototype must still implement concurrent reuse, attached
saved-selection reconnect with no spawn, unrelated transfer, notification
delivery, retained-session retirement refusal, and B inference usability.
An explicit failure at the end prevents accidental success before those
assertions exist. This checkpoint is a tested failing harness/evidence only;
it is not completed acceptance, a passing full gate, or approval to land.
