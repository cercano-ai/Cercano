> Historical evidence, superseded for upgrade acceptance by HOST-ONLY-2026-10-31.md.
> Descendant death, writer-tool tick cessation, descendant freeze/STOP, and private
> stdio shutdown are NOT current upgrade requirements. Workers/tools may survive;
> only independently verified DB-writing hosts are stopped. Independent writer
> uncertainty still refuses. Historical results below are not current assertions.

# Private stdio child stop — review checkpoint (2026-10-31)

Resumes the quota-exhausted implementation; does not repeat ambient diagnosis.
The delegated read-only finding was host 38883 with external stdio children
38888/38889 (dotnet) and 38890 (lunie-mcp), with reciprocal native pipe evidence.
Those PIDs were not inspected or signaled during this resume.

## Implementation boundary

`upgrade_stdio_darwin.go` reads Darwin `PROC_PIDLISTFDS` and
`PROC_PIDFDPIPEINFO`. Ownership requires child FD 0 read / FD 1 write, two
disjoint pipes, reciprocal nonzero kernel endpoint handles and opposite host
endpoint directions. UID, direct parent, lifetime ordering, stable identities
and unchanged repeated pipe inventories are required. A recorded witness is
checked again immediately before and after freezing each private root.
Verified model-host exclusion remains first in classification; unknown direct
children without either existing worker authority or private stdio evidence
remain refused. Existing descendant freeze/rollback and verified exit gates
are reused. This is host-private stdio ownership, not proof of MCP membership
or a new executable-name allowlist. The existing manager launches these
commands via `mcp.CommandTransport` (`internal/mcp_host/manager.go:98-108`).

Tests cover reciprocal pairs, duplicated host endpoints, missing/ambiguous/
wrong-direction/zero/single-pipe evidence, mismatched UID/parent/lifetime,
changed witnesses, native external root and descendant freeze, two root
revalidations, external-image stop, and direct dotnet helper refusal without
private pipes. Existing model-host, unknown-role, cancellation, fork and
rollback coverage remains enabled.

## Executed verification

All commands below ran from the worktree root via the existing isolated,
credentials-disabled, offline fixture build driver. Logs are retained locally
under `source/server/artifacts/`.

- `python3 test/fixtures/production-stop/build.py test ./internal/brewrestart ./cmd/cercano -run 'Test(Upgrade|PrivateStdio)' -count=1 -v`: PASS.
- `python3 test/fixtures/production-stop/build.py`: PASS (all five fixture builds).
- `python3 test/fixtures/production-stop/build.py test ./internal/brewrestart ./cmd/cercano -count=1`: PASS; brewrestart 37.057s, cmd/cercano 51.789s. This isolated run is distinct from the previously documented baseline failures in `TestMissingExecutableZeroFDAndConfirmedExit` and `TestSurvivingAliasConcreteWriterRisks`; no policy implementation or expectations were changed or suppressed. This does not establish why those earlier runs failed.
- After strengthening test-only assertions: `python3 test/fixtures/production-stop/build.py test ./internal/brewrestart -run TestPrivateStdio -count=1 -v`: PASS (0.381s).
- Final `python3 test/fixtures/production-stop/run.py combined`: PASS, disposable root `/private/tmp/cstop-waw0jn0o`. External stdio root PID 59567 and attached sleep PID 59569 were explicitly included in the native pre-backup exit inventory. All recorded external fixtures are asserted members of that inventory before confirmation, then asserted absent after acceptance. `external_stdio_exited_before_backup=true`; one shutdown/backup/migration, migration version 2, four preserved history rows, replacement started and fresh `FIXTURE_OK` reply. Unidentified fixture and sibling worker survived; sibling still served. Generation cleanup remaining `[]`, fixture cleanup remaining `{}`.
- `git diff --check`: PASS.

The earlier `private-stdio-combined.log` failed on an inspection race (`no such
process`); it is not a passing acceptance. The inherited resume log and two
fresh combined executions passed; the final execution above includes the
explicit external-PID pre-backup inventory assertion. No retry/permissive
policy was added to production to hide that failure.

### Local log SHA-256

| Log | SHA-256 |
| --- | --- |
| private-stdio-resume-build.log | 072c9afe08628b79c6182de4ed562db1b066111debb2b66cbbb3c6352b5bb9d7 |
| private-stdio-resume-tests.log | 367f0568b426c51734c1cb0275580e430082901a76dca229ef98b28c8e96acc3 |
| private-stdio-resume-full.log | 42907c8f1e2de9ebe4ee0b9b2235050bdb040225c761f2b9fb5ebbd3395d1bbc |
| private-stdio-resume-pairs.log | 31240d4e8e3e861100b95954dc7727bbb2454df174fc4cea5dcc6c3b66e3dc12 |
| private-stdio-resume-combined-final.log | 97b44749ea3a394d36dc3738c04f022e61532f6c9761ebea8a248f510562e4a9 |

## Limits and review status

Only disposable fixtures were signaled. No ambient shutdown, live launcher or
upgrade, live configuration/database/credential reads, or push. Acceptance
uses fixture-scoped candidate inventory and a synthetic generation backend /
provider, not ambient discovery or production model acceptance. The full
repository suite was not run. Pipe snapshots are necessarily kernel snapshots;
any unavailable/changing evidence refuses private ownership rather than
falling back to parentage alone.

The unrelated change to `efforts/dev-host-generations/everyday-launcher-pty.py`
is left untouched and excluded from this checkpoint. Existing local logs and
untracked baseline binary are also left untouched, not committed. Independent
parent review is required before landing; this checkpoint is not landed.
