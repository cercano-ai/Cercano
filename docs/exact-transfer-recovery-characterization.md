# Exact-operation lower-level recovery characterization

Scope: offline fixture investigation only, based on `56a0de89bd4e` on
`test/transfer-recovery-characterization`. No production change or recovery helper.
Old running binary compatibility remains unverified; nothing was deployed or
attempted against a live host.

## Conclusion

**No existing call sequence meets the full requested recovery-only contract.**
A read-only preflight cannot make the following source RPC conditional on an
already-existing source reservation. Both `LifecycleRelinquish` and
`LifecycleForceRelinquish` can start the original reservation for an accepted
move. That is different from requiring a retry of a preexisting reservation.
Neither exposes an atomic “existing reservation only” condition. Moreover,
rejection after an acceptance disappears can leave a newly fenced source.
Consequently these tests deliberately do not supply a usable recovery helper.

## Conditional sequence demonstrated, not a strict recovery recipe

With fixture-created acceptance, its original nonempty receipt, exact identities,
conversation, source epoch and handover ID:

1. Destination `LifecycleValidateReceipt` checks the existing acceptance without
   creating it. This is advisory preflight, not a reservation or atomic guarantee.
2. Source `LifecycleForceRelinquish` with the unchanged original request can retry
   an existing forced closure after accepted persistence work completes. A source
   ordinary relinquish is a separate path; force refuses a non-forced closure.
   Preserve a nonempty original receipt to select receipt validation, not Prepare.
3. Destination `LifecycleClaim`, targeting the exact destination identity, consumes
   only the matching existing durable reservation, or returns its recorded result.
4. Repeating the completed source and destination calls returns the original
   receipt/result without changing the snapshotted ownership/lifecycle state.
   A historical claim result is **not** evidence of current ownership: it remains
   returnable after a clean release and does not reacquire ownership.

The source RPC does not atomically enforce step 2's preexisting-state assumption.
Do not substitute Resume/Acquire or Prepare/AcceptIncoming on missing state.

## Guarantees and gaps observed

- Source: both APIs start a source reservation when absent. Both can reject a
  receipt whose acceptance disappeared after preflight while still creating a
  closure (durable ownership remains owned). Changed source ownership after
  preflight rejects without changing snapshotted state.
- Existing forced operation: wrong operation, destination host/build, source
  host/build/epoch, conversation, target or receipt rejects without changing
  snapshotted state. Timeout leaves source fenced; exact retry after natural
  completion of held fixture work succeeds. No persistence-failure resolution
  is used by the new tests.
- Missing source receipt is not required to reject: an existing forced retry with
  an empty receipt enters Prepare and reuses the existing acceptance/receipt.
  This is a limitation probe only; it does not delete that acceptance or create
  a replacement. Prepare is otherwise used only to establish fixture state.
- Destination: missing durable reservation, receipt or in-memory acceptance, and
  wrong source build/host/epoch, conversation, operation or target reject without
  changing snapshotted state. Acceptance deletion between successful preflight
  and actual claim is rejected. `ClaimIncoming` checks acceptance identity again
  under its lock, not just the earlier receipt lookup.
- Destination gap: Claim does not compare request `Destination` with `Target`.
  An unrelated Destination field succeeds against the exact authenticated Target
  and original acceptance. Receipt lookup binds the effective destination to the
  local host, but does not enforce the complete caller-supplied destination field.
- Neither lower-level path requires a resume journal. Claims succeed with no
  journal; no journal is copied, retargeted or fabricated. Thus journal presence
  or correspondence is not an RPC-enforced recovery precondition.
- Store claims transactionally require reserved state, conversation, destination
  and move ID. Store `HandoverResult` is a read-only historical receipt lookup.
  The store does not independently validate source build, source epoch or the
  opaque acceptance receipt; those checks live in the lifecycle/acceptance layer.
- Authentication is not replaced by request identity. Production controls require
  ControlAuthorize, loopback and exact Target. Production peer dialing reselects
  identity/endpoint and loads the selected private token. The new fixture uses
  actual per-fixture token interceptors and authenticated clients over bufconn;
  only the loopback peer address is synthetic. This does not test deployed peer
  records, credentials or old binaries.

Implementation references: `internal/server/lifecycle_controls.go` (`control`,
`receiptLocked`, `controlRelinquish`, `LifecycleClaim`), `forced_cutover.go`,
`coexistence.go` (`ClaimIncoming`), `internal/conversation/ownership.go`,
`cmd/cercano/development_host.go` (`developmentPeerDial`), and
`pkg/devhostauth/auth.go` (paths relative to `source/server`).

## Offline evidence

New permanent tests: `source/server/internal/server/exact_recovery_test.go`.
The smallest reservation-gap probe passed first for both source RPCs. The selected
integration suite then passed normally and under the race detector:

```text
go test ./internal/server -run '^(TestExactRecovery|TestResumeNewDestinationInterruptedTransfer)' -count=1 -v
go test -race ./internal/server -run '^(TestExactRecovery|TestResumeNewDestinationInterruptedTransfer)' -count=1 -v
```

Each selected run: **7 top-level tests, 20 subtests (22 leaf cases)**. Of these,
6 top-level tests / 18 subtests are new exact-recovery characterization; the
remaining existing interrupted-transfer test has two subtests. The existing
Resume characterization is regression coverage, not the recovery sequence above.
No full-suite or live compatibility claim is made.

Runs used Go 1.26.3, isolated fixture HOME/XDG config, the existing module cache,
and `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. No live configuration, sockets,
network or agents were used. Unrelated fixture HOME/build artifacts are retained
and excluded from the checkpoint.
