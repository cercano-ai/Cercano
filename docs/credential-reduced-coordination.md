# Reduced credential coordination evidence — 2026-10-28

## Implemented contract

- One nonblocking per-identity file exclusion covers fresh record read, bounded
  refresh issuance, and publication. Contending logout, replacement, refresh,
  snapshot, and admission return `ErrBusy`, not an authentication error.
- The existing identity holds only canonical version-1 `active` / `logged-out`
  revision envelopes. Pending records are refused without migration. Logout
  replaces the secret with a revision marker. No pending journal, quarantine,
  rollback, recovery, backend CAS, or stronger durability contract remains.
- Refresh receives a context deadline capped at 30 seconds and checks cancellation
  before publication. Issuers must honor cancellation; no issuer goroutine is
  abandoned. An issuer/backend error is returned without automatic replay. A later
  independent attempt is not durably quarantined; this is intentionally weaker
  than the superseded protocol, including after ambiguous rotation/write failure.
- Attempt admission and captured-revision login CAS remain. The service releases
  its local profile mutex before issuer work so conflicts reach identity exclusion.
  Existing read-only/disabled wrappers and provider retry/redirect protections stay.
  A cached Responses client resolves a fresh active snapshot on a new attempt after
  replacement, and cannot dispatch after logout. No stale-login retry was added.

## Reproduction and checks

All Go commands used `go` from PATH with
`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, from `source/server`.
Enabled service tests use explicit memory stores/fake issuers and hermetic
credentialtest isolation (default HTTP denies all destinations, including loopback).
Process tests directly exercise the coordinator with a test-only file backend;
children require disabled/offline policy. No native credentials or live HTTP used.

A temporary memory-only `TestProbe` package copied the coordinator from HEAD and
then from the worktree (without replacing tracked files), running
`go test ./internal/reduced_probe_<temporary> -run '^TestProbe$' -count=1 -timeout=20s`.
HEAD failed both smallest cases: logout succeeded inside refresh issuance, and a
pending record was accepted as ordinary absence. Worktree passed. Probe directory
was removed. Permanent reduced tests additionally assert distinguishable busy,
service guards, bounded cancellation, and no pending migration.

Passed:

```text
go test -race ./internal/hostsvc/credentials -run '^(TestReduced.*|TestCoordinated.*|TestCredentialProcess.*|TestSubscriptionSnapshot|TestAnthropicSubscriptionSnapshot|TestNestedServiceStaleLogin|TestGuardedCoordinatedAcquisitionRefused|TestFacade.*|TestLegacyFacadeStillUsesLegacyAcquisition|TestAttemptReadonlySnapshotLogout)$' -count=1 -timeout=90s
# ok (13.643s); includes separate-process fake-backend exclusion/login tests

go test -race ./internal/llm/responses -run '^TestCoordinated(SnapshotMutationRefusesHTTP|AdmittedAttemptOutlivesLogout|CachedClientFreshActiveRevision)$' -count=1 -timeout=60s
# ok (1.227s)
```

## Limits / remaining prerequisites

No broad suite, native qualification, live-provider call, migration, activation,
merge, or push. The separate-process fake uses same-directory rename, not native
storage, fsync/crash-durability qualification, or recovery. Production remains
inactive: separately authorize and establish an appropriate existing-identity
adapter, stable private shared exclusion root, and exclusion of legacy participants
before activation; no native adapter or initial-record provisioning is supplied.
The stronger requirements in `native-credential-contract-audit.md` are historical,
not prerequisites restored by this change. Full production integration/acceptance
is not claimed. No unresolved choice within the approved reduced design.
