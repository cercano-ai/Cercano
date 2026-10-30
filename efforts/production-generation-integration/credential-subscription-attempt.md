# Subscription attempt feasibility checkpoint — 2026-10-28

## Outcome: blocked at the existing Service refresh contract

No production integration or workaround implemented. Coordinated Responses/ChatGPT remains fail closed. This is a reproduction and interface-blocker report, not a completed subscription slice.

Exact blockers:

- `internal/hostsvc/credentials/service.go:224` rejects coordinated stores before resolving even a fresh token. `token`/`flight` carry access/account, not the revision of an atomic active record. Simply removing this guard is unsafe: `run` publishes staged refresh via ordinary `Set` after issuer work, with no durable pending marker before consuming the refresh grant. Service generation alone does not fence another coordinator's logout.
- `internal/hostsvc/credentials/coordinated.go:215` provides the safe low-level pending rotation algorithm, with issuer outside commit exclusion, but returns only `error`. It is not a Service acquisition primitive or a capability forwarded by wrappers. Its final `replace` discards the active record it writes. A later revision read paired with the issuer's previously returned token would be unsafe across replacement. A later *whole atomic snapshot*, parsed anew and admitted against its own revision, could be safe, but that is additional integration, not an existing Service return contract.
- `internal/secrets/coordination.go:41` exposes atomic snapshot/admission only, not rotation. Readonly wrappers forward these reads/admissions and reject mutations; asserting a concrete coordinator or unwrapping them to refresh would bypass that boundary.
- `internal/chatgptauth/source.go:96` invokes refresh before `Save`; the staged transaction does not have a pre-issuer pending hook. Its raw Token result cannot be paired with a separately fetched revision. `internal/hostsvc/credentials/facade.go:67` returns an atomic active payload/revision but does not perform provider refresh.

Therefore the existing Service refresh primitive cannot safely expose a refreshed active snapshot as currently defined. This is not a claim that safe extension is impossible. No new start/await abstraction, raw-token fallback, or speculative extension was introduced.

## Smallest reproduction and verification

Added `internal/hostsvc/credentials/subscription_attempt_test.go`, `TestSubscriptionAttemptServiceBlocker/{fresh,expired}`. A canonical memory record, temporary lock root, Service ChatGPT source, and injected rejecting issuer transport reproduce `ErrProviderIntegration` for two acquisitions. Both return no access/account, leave the entire atomic snapshot unchanged, and make zero issuer calls. This passing characterization captures the missing integration before any fix; no fix was attempted.

Commands from `source/server`, each with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
go test ./internal/hostsvc/credentials -run '^TestSubscriptionAttemptServiceBlocker$' -count=1
go test -race ./internal/hostsvc/credentials -run '^Test(SubscriptionAttemptServiceBlocker|CoordinatedLogoutFencesRefreshAndAdmission|CoordinatedUncertainRefreshNoReplay|CoordinatedRefreshSerialization)$' -count=10
```

Both passed. Credential enablement is confined to injected memory scopes. Existing selected tests exercise independent-owner logout fencing, pending uncertainty/no replay, and refresh serialization. No native credentials, real network, child processes, full suite, subscription HTTP success, worker integration, or durability qualification was exercised.

## Supported entry paths and unchanged gaps

The supported coordinated request entry remains static Responses only: host providers -> `cloudfactory.BuildCloudProvider` with `Options.Credentials`, Responses flavor, empty route, nonempty profile, no token sources -> `responses.NewClient` with `Config.Credentials`/`CredentialProfile` -> `Chat` or `StreamChat` -> `do`. Explicit direct construction with that same coordinated static configuration is also supported. `cloudfactory/factory.go:88` and `llm/responses/client.go:190` refuse subscription configurations rather than fall back to a supplied raw key.

Every supported constructed POST obtains an atomic snapshot and fresh admission, including temperature fallback. Logout invalidates later admission; already admitted requests may transmit afterward. The existing qualified HTTP/1, no-reuse, no-redirect, replay-stripping path is unchanged. No subscription success entry path is added. Readonly mutation refusal, wrapper coordination, and pending no-replay behavior are unchanged, not broadened by these tests.

Worker raw-credential transfer, worker-local constructors/diagnostics, direct agent/development factory calls without capabilities, legacy SDK constructors, and previously exported raw material remain outside the coordinated admission graph. A factory cannot infer authority from a raw string. Other providers, Bedrock, discovery, and diagnostics remain unsupported for coordinated use. No activation, adoption, native backend, install, spec/plan, push, or merge changes.
