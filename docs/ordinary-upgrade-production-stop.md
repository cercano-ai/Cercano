# Production worker shutdown probe (2026-10-30)

Status: bounded isolated combined acceptance passes; **not global ordinary-upgrade qualification**.
The production discovery implementation and `--stop-hosts-config` safety refusal
remain unchanged. Only the `upgradefixture` build exposes scoped native discovery.

## Expectations and implementation

The fixture writes expectations before launching anything, uses compiled untagged
`cmd/cercano` agents/workers, isolated HOME/XDG/TMPDIR, disabled credentials, and a
loopback synthetic Ollama provider. A real RunCommand shell writes every 100ms,
ignores TERM, and self-expires after 120 iterations. A second worker is held in a
provider request after the initial worker inventory. Native acceptance requires
host/worker exit, stable writer ticks after stop returns, and shared model-host
survival. No ambient process is selected or signalled.

Shutdown freezes verified workers before recursively enumerating and freezing their
attached same-user descendants. Each parent is frozen before its children are
inventoried. Executable/start identities are checked at signalling; process groups
are never signalled. Unknown host children refuse. The tree is bounded at 4096
processes. Pre-TERM refusal resumes recorded identities; after TERM, bounded
cancellation cleanup remains irreversible. The cancellation test now observes
actual TERM delivery, since frozen workers no longer exit at TERM.

This does not contain detached/reparented tools or prove every kernel fork/exec
interleaving. The second-worker assertion demonstrates late inventory coverage,
not a deterministic fork-in-progress race. The bounded published-child/pre-exec reproduction below is now covered; kernel-internal
fork publication qualification remains outstanding. No migration authorization is inferred.

## Observations

With the descendant-freeze call temporarily bypassed and restored in `finally`,
the compiled native probe failed as expected: `/private/tmp/cstop-1nl36hjc` recorded
3 ticks at stop, 25 after shutdown, and 44 after waiting. Native stop itself returned
success and only the model host appeared among tracked Cercano survivors. This
establishes actual surviving tool work, not merely a surviving PID.

With the correction, combined probe `/private/tmp/cstop-nhsg0lem` recorded 3 ticks
at stop and 6 both after shutdown and after waiting. Native stop succeeded; the
shared model host survived and fixture cleanup reported no remaining tracked PIDs.
The disposable SQLite backup preserved the running task and version 0; migration
returned success, preserved turn rows, interrupted the task, passed integrity_check,
and set version 2. The original replacement `agent` failed at the intended legacy
admission boundary. This failure was reproduced before edits in
`/private/tmp/cstop-14_yt1m6` (exit 1, migration version 2, replacement refused,
no tracked survivors after cleanup).

The corrected fixture uses the **existing** source launcher, real generation
publisher, `development-launch`, ordinary generation host and compiled CLI PTY.
It does not install a launcher or alter the legacy opener. The same upgraded
`state/conversations.db` is selected explicitly; `/resume` displays pre-upgrade
history, and a fresh user turn and synthetic `FIXTURE_OK` assistant reply persist
in that original conversation. Tagged backend substitution follows
`everyday-launcher-compiled.py`; native shutdown still uses untagged production
hosts/workers and scoped `upgradefixture` discovery. Old Ollama environment
selection is removed only for the replacement synthetic backend. An intermediate
probe caught that stale selection (fresh work invoked the old synthetic tool),
so the final assertion also requires writer ticks to remain unchanged across
replacement startup and fresh reply.

Passing combined evidence before checkpoint: `/private/tmp/cstop-nld04v7w`:
3 ticks at shutdown and after waiting, version 2, 3 original turn rows preserved,
`FIXTURE_OK` persisted, original task still interrupted, generation and legacy
tracked cleanup sets empty. No production discovery or guard implementation was
changed.

### Remaining freeze limit

A deterministic kernel fork-in-progress regression is **not completed**. The
implementation stops a parent, observes stopped state, and inventories its children
once. This run cannot prove that Darwin's stopped-state observation fences every
already-in-progress fork's child publication. A synthetic late-inventory test would
not establish that kernel property. The observed second production worker is only
late-inventory evidence, not this guarantee. Detached/reparented processes remain
outside the bounded claim; there is no universal containment or authorization to
remove the production guard.

## Reproduction

```
python3 test/fixtures/production-stop/build.py
python3 test/fixtures/production-stop/run.py native
python3 test/fixtures/production-stop/run.py combined
python3 test/fixtures/production-stop/build.py test ./internal/brewrestart ./cmd/cercano ./internal/conversation -run 'Test(Upgrade|.*Discovery|.*Cancellation|Offline|Ordinary)' -count=1
```

`combined` must return zero only after same-store CLI resume and durable fresh reply assertions.
The build driver uses `/opt/homebrew/bin/go`, local existing caches, GOPROXY=off,
GOSUMDB=off and GOTOOLCHAIN=local. No installation, live database migration, push,
or landing is performed. Fixture migration uses the existing acceptance-tagged
empty discovery shim only after scoped native shutdown assertions; this is **not**
unmodified global discovery acceptance or automatic guarded ordinary-upgrade authorization. A clean
macOS account/VM and qualified startup path are still required for full-environment
acceptance. No current ambient-process diagnostic was run.

## Review corrections: exec rollback and different-version model hosts

Rollback now records whether this operation successfully sent STOP, preserving a
previously observed stopped state. CONT rollback checks PID, UID and kernel start
time, not the pre-exec executable; TERM/KILL still require strict executable
identity. Cleanup errors are joined into the refusal instead of discarded.
A deterministic disposable shell-to-sleep exec at the inspect/STOP seam reproduces
the stale strict-identity failure. Native state checks assert resumed state after
rollback and preserved STOP for a previously stopped process.

Attached descendant model-host exclusion now uses the same kernel executable-name
and revalidated role checks as direct children, regardless of version path. An
arbitrary non-Cercano executable does not gain exclusion from argv alone. This is
the existing bounded identity convention, not cryptographic executable attestation.
Production discovery and the production stop-hosts guard are unchanged.

Negative controls (temporary mutations restored in finally):
- P1 strict executable rollback: `upgrade-build-k2vs1y16/output.log`, expected
  failure `rollback lifetime changed` in `TestUpgradeExecSTOPRollback`.
- P2 old equal-executable exclusion: `upgrade-build-2cdqptaj/output.log`, expected
  failure `different-version model host frozen`.
These build roots are under `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/`.
An initial test run (`upgrade-build-gderg4v4`) exposed a fixture path canonicalization
error (/var versus /private/var); fixed by resolving the copied executable path.
Three repeated affected native test runs passed in `upgrade-build-51n5sgqv`.

`TestUpgradeInFlightForkObservable` compiles a disposable C fixture with a real
fork and pipe handshake. The child has been published but waits before exec.
After its parent is natively stopped, releasing the child proves it can still
write and exec; the subsequent bounded descendant scan finds and freezes it.
This establishes observable in-flight spawn semantics, **not** a deterministic
kernel-internal fork/publication interleaving or a STOP publication fence. The
kernel-internal question above remains unproven; detached/reparented descendants,
concurrent external STOP/CONT and the inspect-to-signal PID reuse window are not
universally contained. No production guard relaxation follows.

Final-commit verification uses the reproduction commands above with `-v` on the
affected tests. The external build roots retain exact allowlisted environments and
output logs; the combined fixture records source_commit and its disposable store,
generation, CLI transcript, writer ticks, and cleanup evidence in its result files.
