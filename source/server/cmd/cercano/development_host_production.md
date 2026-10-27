# Prepared production development-host dependencies

The factory consumes only `state/config.yaml` (defaults plus YAML, deliberately
not the legacy environment-merging/migrating config loader). Both the config
snapshot and credential service are read-only. The default constructor opens the
native keychain read-only only after coexistence preflight; tests use an
invocation-local opener returning `secrets.NewMemory()`.

The graph uses the prepared durable server's supplied store for the real agent,
ADK coordinator, inference runners, toolstack dispatch engine, capability registry,
and pinned prepared-worker runner. Diagnostics are separated into
`state/hosts/<host>/routing.jsonl` and `failures.jsonl`. No startup migrations,
retention sweeper, config watchers, or legacy default database are installed.

## Provider/runtime matrix

- Open: `llama_server` and `mistralrs`, through an **already-running,
  coexistence-protected shared modelhost** only. Attachment is bounded; absence,
  incompatible protection, or an attachment error is fatal. No launch,
  replacement, in-process runtime, or Ollama fallback.
- Cloud: configured profiles through the existing provider service and
  `cloudfactory.BuildCloudProvider`: `messages`, `chat_completions`, `responses`,
  and `bedrock`; the existing subscription routes use the read-only credential
  service's token sources. Static-key profiles read by profile name from the
  guarded store. The legacy router callback resolves against those same frozen
  profiles, not its loose API-key/base-URL arguments.
- Cloud-only still requires protected modelhost attachment in this graph; it does
  not replace the open dependencies with fixtures or nil providers. Modes other
  than `open_only` require an active prepared cloud profile. Configured profile
  build failures are fatal rather than silently degrading to the open lane.
- Legacy-only cloud configuration is not migrated. Unsupported runtime, invalid
  locus mode, and missing required active cloud profile fail clearly.

## Focused verification and limits

Run from `source/server`:

```
go test ./cmd/cercano -run TestDevelopmentProduction -count=1
```

The focused graph test injects only memory credentials and an inert runtime
manager, constructs real open/cloud graphs, checks the real `Read` registry
entry, executes that filesystem tool inside durable foreground admission, and
checks cleanup/config preservation. Refusal tests cover preflight-before-opener,
unsupported snapshots, opener failure exactly once, and runtime failure without
fallback. No native opener, provider inference, child process, or external
credential flow is exercised.

Permission decisions are strict and session-local (no legacy permissions-file
watcher or persisted allowlist).

Current verification (2026-10-26): the focused command compiles and constructs
both graphs, including their real tool registries and prepared worker runners,
but both graph cases fail at `Retain` with `coexistence contract mismatch`.
`internal/conversation/ownership.go:checkOwnershipContract` checks
`CurrentCoexistenceDeclaration()` rather than the development declaration used by
the prepared database. This separately owned ownership-layer mismatch blocks
foreground execution; the test intentionally remains failing rather than
substituting a legacy database or skipping the assertion. The foreground tool
execution assertions therefore remain unverified. The prepared-worker bridge is
now present and its dependency setup includes the required permission broker.

Protected modelhost attachment and actual cloud/open inference require separate
authorized integration evidence; these unit tests make no claim of native
credential access, successful inference, subscription refresh, or Bedrock
authentication.
