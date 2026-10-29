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

## Public Busy propagation correction — 2026-10-28

The earlier statement that contention returns `ErrBusy`, not an authentication
error, described the coordinator but overstated the public contract at
`da25095a7750b81e49d75ec8b80ccedec09d2817`. Memory-only regressions reproduced
Responses/OpenAI Chat and Stream returning Unknown, Anthropic returning Network,
and login Commit returning terminal CredentialError. The coordinator sentinel
alone did not establish consistent public Busy classification.

`ErrBusy` now uses the existing `llm.CredentialError` seam with class `llm.ErrBusy`
and safe `identity_busy` diagnostics. Sentinel identity survives normalization
and SDK wrapping (`errors.Is`). OpenAI recognizes that seam before network
classification; Commit preserves Busy before its terminal store-error wrapper.
BeginLogin, snapshots, admission, and refresh inherit the same temporary class.
No generic Busy retry/failover policy was changed. No stale-login rebase, logout
bypass, automatic issuer retry, or ambiguity recovery was added.

Audit scope is only the new `internal/llm/credential_busy_integration_test.go`
tests. They call `credentialtest.EnableHermetic` before construction, use
constructor-injected non-dialing HTTP wires with synthetic `.invalid` URLs,
in-memory record storage, and fake inline issuers. Only exclusion files use
`t.TempDir`; no native backend, real SDK endpoint, DNS, sockets, or loopback
server is used. Responses gained the same explicit `BaseTransport` injection
seam already available on the other providers. It remains inside the existing
non-replay wrapper and redirect refusal; the default production transport is
unchanged.

From `source/server`, with the outer environment shown verbatim:

```text
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./internal/llm -run '^TestCredentialBusy' -count=1 -timeout=30s
# baseline error-path implementation: FAIL (exit 1), all 12 provider cases;
# BeginLogin/snapshot/admission/refresh Unknown; Commit CredentialError.

CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./internal/llm -run '^TestCredential(Busy|CachedClientReplacement|TransportGuards)' -count=1 -timeout=30s
# fixed implementation: ok cercano/source/server/internal/llm 1.382s (exit 0)

CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./internal/llm -run '^TestCredential(Busy|CachedClientReplacement|TransportGuards)' -count=10 -timeout=60s
# ok cercano/source/server/internal/llm 1.658s (exit 0)
```

The initial failing regressions preceded the fix. The baseline was additionally
rechecked by temporarily loading the original `coordinated.go`, `login.go`, and
OpenAI `normalize.go` with `git show da25095a7750b81e49d75ec8b80ccedec09d2817:<path>`;
a Python `try/finally` restored the fixed files. The Responses injection-only
seam remained so no original native/default transport was needed.

Coverage: snapshot/admission contention through all three public providers'
Chat and Stream, no wire dispatch or record mutation on Busy, sentinel identity,
safe diagnostics, unchanged generic Busy resilience membership, public
BeginLogin/Commit and refresh contention (issuer not invoked), subsequent
stale-revision Commit refusal and no logout bypass. The same cached clients use
replacement credentials on the next independent attempt and do not dispatch
after logout. Transport checks assert stripped `GetBody`, one wire call for
synthetic 503 (no SDK retries), and one for 307 (no redirect following), across
all three providers and both APIs. An initial guard-test expectation that every
SDK must *error* on 307 failed for Anthropic; it was narrowed to the existing
no-follow contract, not fixed by changing provider behavior. These tests do not
claim uniform 3xx decoding behavior. No whole provider suite was run.

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
