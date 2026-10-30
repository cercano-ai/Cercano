# Production worker shutdown probe (2026-10-30)

Status: bounded descendant correction; **not qualified ordinary-upgrade acceptance**.
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
not a deterministic fork-in-progress race. That race reproduction and qualification
remain outstanding; no migration authorization is inferred from this snapshot.

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
and set version 2. **Replacement startup failed**: ordinary `agent` used the legacy
opener, which refuses the marked store (`marked store requires OpenCoexistence with
explicit ownership`). This is the intended admission boundary, not permission to
weaken it. Canonical generation activation additionally remains gated on service
parity in `production_startup.go`. Full combined acceptance remains blocked there.

## Reproduction

```
python3 test/fixtures/production-stop/build.py
python3 test/fixtures/production-stop/run.py native
python3 test/fixtures/production-stop/run.py combined
python3 test/fixtures/production-stop/build.py test ./internal/brewrestart ./cmd/cercano ./internal/conversation -run 'Test(Upgrade|.*Discovery|.*Cancellation|Offline|Ordinary)' -count=1
```

`combined` must currently return nonzero at replacement startup, not silently pass.
The build driver uses `/opt/homebrew/bin/go`, local existing caches, GOPROXY=off,
GOSUMDB=off and GOTOOLCHAIN=local. No installation, live database migration, push,
or landing is performed. Fixture migration uses the existing acceptance-tagged
empty discovery shim only after scoped native shutdown assertions; this is **not**
unmodified global discovery acceptance or the guarded launcher path. A clean
macOS account/VM and qualified startup path are still required for full-environment
acceptance. No current ambient-process diagnostic was run.
