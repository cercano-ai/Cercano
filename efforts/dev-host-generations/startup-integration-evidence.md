# Explicit development-host startup fixture

`cercano dev-host` now enters before ordinary CLI/agent startup. This is an
actual compiled-agent startup/identity fixture, **not native A/B acceptance or a
conversation execution harness**. Ordinary `agent` startup is unchanged.

## Deliberate boundary

The command validates `CERCANO_DISABLE_CREDENTIALS=1` before reading inputs or
entering any backend, requires fixture-local HOME/config/temp roots, explicitly
preflights an already prepared store, and constructs `server.NewPreparedServer`
with a frozen config snapshot, the same store in the agent, and memory-only
secrets. It does not call a credential backend opener, config loader/watcher,
ordinary worker reaper, discovery launcher, provider, or installed executable.

Private-file `devhostauth` unary and stream interceptors run first, including on
identity and lifecycle queries. `ControlAuthorize` uses the same auth object;
`ControlDial` only accepts explicitly enumerated loopback peers and uses each
peer's own host/token. No source token forwarding.

The running executable must be the exact resolved `GENERATION/cercano` and match
the manifest SHA256. `worker.NewSpawnPolicy` validates/reserves an immutable
per-generation/per-host namespace (reusing a host instance fails). No worker is
spawned. The protected modelhost record is explicitly selected inside the fixture
and copied to a private pinned directory. `modelhost.NewRemote` with
`CoexistenceProtection` verifies/attaches only to that record: no replacement,
launch, or normal discovery fallback. Close only releases this participant.
The fixture operator must provision a **synthetic protected modelhost**, never
copy a real runtime record into this fixture.

Only `GetDevelopmentHostIdentity`, `GetCredentialPolicy`, `LifecycleList`, and
`LifecycleRetire` are enabled. Other authenticated unary/stream RPCs fail with
FailedPrecondition; missing auth fails first with Unauthenticated. This prevents
partially wired providers, legacy worker spawning, or in-process fallback.

### Concrete remaining execution dependencies

`server.PreparedServerOptions` currently has no worker-policy/runner injection
field. `worker.NewWorkerRunnerWithPolicy` exists, but `server.Server.workerRunner`
and `runnerDeps` are private; installing the legacy runner is not safe. This
command reserves/validates the pinned policy but does **not** claim to integrate
worker execution. `server.CoexistenceOptions.Reload` also requires a durable
pending-outcome reload bridge before work acquisition can be enabled; the fixture
callback explicitly refuses instead of pretending a no-op reload is complete.
The concurrent admission task's changes are neither owned nor qualified here.
These are execution-harness dependencies, not excuses to use ordinary startup.

## Fixture contract and exact invocation

Prepare a mode-0700 absolute, non-symlink `FIXTURE` directory containing:

- `conversations.db`, explicitly created with
  `conversation.PrepareCoexistence(ctx, path)` before startup (no automatic
  adoption/migration by this command);
- `config.json`, an explicit JSON `config.Config` snapshot (`{}` is sufficient
  for this no-inference fixture); worker idle reaping is forced off;
- `token`, created with `devhostauth.Create(path)` (private 0600 file);
- `peers.json`, `[]` for a single fixture, or explicit objects with `Endpoint`,
  `Host`, `TokenFile`; do not list the source host;
- existing `home/`, `config/`, `tmp/` directories;
- `modelhost/`, explicitly provisioned by a synthetic protected participant via
  `modelhost.WriteRecord`. No real runtime backend is needed.

`GENERATION` is an absolute resolved immutable directory containing the dedicated
compiled `cercano` executable and `manifest.json`:

```json
{"Build":"fixture-build-A","SHA256":"<lowercase SHA256 of cercano>"}
```

Build with `go build -o "$GENERATION/cercano" ./cmd/cercano` from `source/server`,
write the manifest, then freeze the generation (never rebuild in place). Exact
startup invocation for the subsequent harness (choose fresh host IDs and ports):

```sh
env -i PATH=/usr/bin:/bin \
  CERCANO_DISABLE_CREDENTIALS=1 \
  HOME="$FIXTURE/home" XDG_CONFIG_HOME="$FIXTURE/config" TMPDIR="$FIXTURE/tmp" \
  "$GENERATION/cercano" dev-host \
  --fixture "$FIXTURE" --generation "$GENERATION" \
  --modelhost "$FIXTURE/modelhost" \
  --host fixture-A-unique-lifetime --listen 127.0.0.1:50471
```

Startup prints `dev-host ready ADDRESS build=BUILD host=HOST` only after successful
prepared construction and protected modelhost attachment. Use the token and exact
host to query identity. SIGINT/SIGTERM closes this process's participation. This
invocation qualifies startup only; it cannot yet perform conversation A/B moves.

## Executed verification

From `source/server`:

```sh
go test ./cmd/cercano -run '^TestDevHost' -count=1 -v
go test -race ./cmd/cercano -run '^TestDevHost' -count=1
go test ./pkg/devhostauth ./pkg/agentclient -run 'TestAuthenticationBeforeUnaryAndStreamEffects|TestPrivateTokenFile|TestAuthenticatedPinnedReconnect' -count=1
```

All passed. The race run instruments the test harness; its separately compiled
child binary is a normal build.

The integration test builds a dedicated real CLI executable in a temporary
generation, launches it with a sanitized fixture-only environment, supplies an
empty protected synthetic runtime manager (no provider registered), verifies
exact authenticated identity and disabled-credential attestation, and rejects
missing unary and stream authentication. Malformed/absent/enabled credential
settings in actual child processes are rejected before any synthetic modelhost
RPC entry. Fixture HOME/config directories stay untouched. No real runtime,
current agent, installed launcher, paid provider or Keychain is used. This is not
native A/B acceptance.
