# Bounded selected-candidate health acknowledgement

Scope: an already-owned candidate only. No launch orchestration, process stop,
rollback, cleanup, live installation access, schema migration, or lifecycle
completion. Attached terminal reconnection is not a health input.

## Boundary diagnosis

The existing `ObserveCandidateHealth` predicate accepts operation/journal inputs
from its caller and documents selection, exclusion and revision revalidation as
caller obligations. Its focused predicate tests exercise observations without
installation state: this gate alone cannot authorize durable acknowledgement.
The pre-implementation focused gate test passed; the new guarded tests exercise
the missing authority boundary with disposable SQLite, selection and inert image
fixtures. No claim is made that the predicate itself was broken.

## Implementation

`AcknowledgeCandidateHealth` holds the same directory-bound exclusive guard used
by Switch. It loads the current operation and selected journal, checks the exact
selection, verifies images against the persisted manifest using existing helpers,
and binds the owned lifetime to the selected agent path. Candidate launch binding
now retains an image identity/size/mtime receipt; `SelectedIdentity` rejects a
changed receipt. Content checks remain separately mandatory. After observation,
selection, image bytes and lifetime are checked again.

The state transaction checks current operation ID, revision, health-check state,
recovery flags and target together with journal revision, immutable facts and the
selected -> health-verified transition. Only journal checkpoint/revision changes.
No independent EventHealthSuccess occurs. A retry at health-verified returns
ErrHealthAlreadyRecorded, never fresh success for a stale or unrelated candidate.

## Verification (macOS host)

Passed:

- `go test -race ./internal/updatecoord/state ./internal/updatecoord/activationtxn -count=1`
- `go test -race ./internal/updatecoord/candidate -run '^TestSelectedImageReceipt$' -count=1`

Coverage includes wrong candidate/selection, closed or wrong-directory guard,
wrong manifest policy, operation/journal revisions, recovery, selected-byte and
lifetime changes during observation, uncertain health, write failure, immutable
journal fact mismatch, unchanged operation lifecycle, and database reopen.
File-receipt tests use inert files only. Existing state/transaction suites use
disposable installations; no production candidate is launched.

## Limits

Sequential native observations are not authenticated TCP peer identity or a lease
on future survival. Trusted packaging policy and cooperative installation
exclusion remain required. This is not end-to-end update acceptance or native
Windows/Linux validation. Lifecycle reconciliation of health-verified remains
future work. Startup safety remains unresolved for future launch orchestration;
this primitive neither launches a candidate nor chooses a startup strategy.
Read-only delegated review was unavailable under the bounded sub-agent rules;
review was local. No merge or push.
