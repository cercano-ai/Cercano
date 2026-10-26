# Development authentication integration — partial, 2026-10-26

Branch inspected: `feat/dev-host-generations`; starting HEAD
`325c1b36acf62c239f5074a3a8d63afd4e730ab2`.

This is a solved authentication prerequisite, **not native A/B acceptance**.
No installed launcher, agent, runtime, user data, Keychain or paid provider was
invoked. No spec changes, merge or push. Other-task edits to the plan and server
admission files are deliberately excluded from this checkpoint.

## Implemented

`pkg/devhostauth`: exclusive-create random 256-bit tokens in 0600 files; rejects
exposed, malformed or symlink token files. Immutable loaded credentials, redacted
formatting, no token argv/environment interface. Unary and stream interceptors
reject missing/wrong token or wrong exact host before handlers. All services and
methods are guarded, with no identity/lifecycle exemption. Install these options
before any other effectful interceptor. Intended only for same-user development
on loopback; not hostile same-user security or an alternative transport.

`pkg/agentclient`: `DialAuthenticatedPinnedSession(ctx, intent, tokenFile)`
requires literal loopback TCP, loads once, authenticates even credential-policy
and identity queries, and preserves that auth on explicit reconnect alongside
existing exact build/host/contract/epoch checks. No discovery or launch added.
Legacy unprotected APIs remain unchanged; protected servers reject them.

## Exact executed verification

Working directory: `source/server`.

```
go test ./pkg/devhostauth ./pkg/agentclient -count=1
```

Passed (0.293s / 33.171s), before the additional authenticated pinned-client test.

```
go test ./pkg/devhostauth ./pkg/agentclient -run 'TestAuthenticationBeforeUnaryAndStreamEffects|TestPrivateTokenFile|TestAuthenticatedPinnedReconnect' -count=1 -v
```

Passed (0.387s / 0.447s). Synthetic bufconn unary + server-stream cases assert
zero handler effects for missing token, wrong token and wrong host, with valid
positive controls. A dedicated ephemeral loopback fake Agent exercises strict
disabled-credential attestation, authenticated pinned reconnect, released-session
refusal, zero process-launch calls, and lifecycle authentication before effects.
Temporary tokens live only in Go test fixture directories and are cleaned up.
These are synthetic tests, not real executable/native application qualification.

The same targeted command with `-race` also passed (1.300s / 1.577s):

```
go test -race ./pkg/devhostauth ./pkg/agentclient -run 'TestAuthenticationBeforeUnaryAndStreamEffects|TestPrivateTokenFile|TestAuthenticatedPinnedReconnect' -count=1
```

## Unresolved launch gate / limitations

Actual `cmd/cercano` startup is NOT integrated by this checkpoint. Do not launch
A/B using ordinary `agent`: main.go still loads the default config (1045), opens
the ordinary store (295–307), starts retention (394–402), reaps workers (411),
constructs `server.NewServer` (430), and starts ordinary config watchers (511).
Those are not dedicated isolated prepared-host paths. A safe fixture command and
native harness still need to be implemented, wiring `NewPreparedServer`, fixed
read-only credential/config options and protected modelhost remote. No claims
are made that package tests establish backend exclusion in native children.

Admission work is concurrently modifying `internal/server`; it was not completed
or verified by this task. `prepared.go:20` still declares normal admission disabled
pending foreground/context bridging at the time of inspection. That work must be
qualified, but it is not the only blocker: the startup wiring and native harness
above are themselves unfinished. No native host or model backend was launched;
there are no A/B acceptance artifacts or successful move/retire claims.

Server wiring required next: load token before protected side effects, install
`auth.ServerOptions()` first on the existing gRPC server, supply `auth.Authorize`
as lifecycle `ControlAuthorize`, and use the selected destination's token and
exact host with `auth.DialOptions()` in `ControlDial`. Never forward the source
host's token to its destination. No service/discovery redesign is needed.
