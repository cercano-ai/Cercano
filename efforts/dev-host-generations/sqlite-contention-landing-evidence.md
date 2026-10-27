# Corrective local landing evidence — 2026-10-27

## Provenance and original failure

Authorized local landing: `fix/launcher-sqlite-contention` at
`e5e94471133a55a139bb5a04222311bff1f20d43` onto main previously at
`9bf0a6b016cabc2fa90a541b54189e5061cf54ec`.
The original landing test failure was reciprocal source-first ownership transfer:
`FailedPrecondition: database is locked (517)` (SQLite BUSY_SNAPSHOT).
The retained `/private/tmp/csql-45534/repro.log` independently records that failure.
The fix admits coexistence writers with immediate transactions before ownership
snapshots, including replacement connections; legacy/read-only behavior remains
unchanged. It preserves context errors and rolls back late admission without retries.
The fix commit records reciprocal count=200 and race count=10; retained logs were
inspected and show PASS. Those historical stress counts were not rerun here.

## Explicit pre-land gate (all PASS)

From the exact clean fix worktree, before calling git_land:
- Server: `go test ./... -count=1` and `go build ./...`.
- CLI: `go test ./... -count=1` and `go build -o <private-temp>/cli ./`.
- Server: `go test ./cmd/cercano -run ^TestEverydayLauncher -count=1 -v`,
  with `CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1`; both compiled PTY and
  entrypoint refusal tests explicitly passed (not skipped).

All commands used `CERCANO_DISABLE_CREDENTIALS=1`, `CERCANO_OUTER_ENABLED=0`,
`GOPROXY=off`, `GOSUMDB=off`, and mode-0700 TMPDIR `/private/tmp/cl-xcia7huv`.
The compiled harness uses synthetic fixture providers, private HOME/state, and
explicit developmentfixture,cercano_development_acceptance tags. No native
credentials, native model, or installed processes were exercised.

## Landing and post-land gate (all PASS)

Only after all explicit gates passed, git_land performed the authorized local
land. It reported no configured tool test gate; that is why the explicit gate
above preceded it. No push was performed.
Main immediately after landing exactly equaled the fix SHA above; ancestry of
both the branch tip and fix worktree HEAD in main was verified.

A clean tracked-only local clone at `/private/tmp/cl-xcia7huv/landed` was checked
out detached at that exact SHA, excluding main's unrelated untracked files.
On this landed snapshot:
- `go test ./internal/server -run ^TestSourceFirstReciprocalNoMutexAcrossRPC$ -count=20`: PASS.
- Opt-in `TestEverydayLauncher*` compiled actual-shell/publisher/host/CLI PTY gate
  and entrypoint refusals: PASS, no skips.
- Both compiled reports record source tree `1e3ba7945abffd04b08866ba117788bfccfe0456`
  and checkout `e5e94471133a55a139bb5a04222311bff1f20d43`.

Private logs/reports: `/private/tmp/cl-xcia7huv/{server-suite,cli-suite,server-build,cli-build,launcher-gate,post-reciprocal,post-launcher}.log`
and `pre-compiled/compiled.json`, `post-compiled/compiled.json`.
Harness artifact roots retained: `/private/tmp/ca-ya1jupfg` and `/private/tmp/ca-8rw3_skh`.
Only this sanitized summary is committed, not raw runtime identifiers or transcripts.

## Launcher preservation and safe cleanup

Installed `/Users/bryancostanich/bin/cercano` SHA-256 verified before and after gates:
`d37ee44856fd8ef34aaec5091a5c9ad265bfc67378d847edde0cdc67459256fc` (unchanged).
Candidate `/private/tmp/el-candidate-sji6yred/cercano` SHA-256:
`99f14bdf3815401086a283e23aaf6e344991968d95c31fcd0a77640a48d4b4ce`.
Candidate `bash -n` passed; its default repository is the main checkout. It was
not executed against real state, installed, or modified.

Fix worktree `/Users/bryancostanich/git_repos/cercano-ai/Cercano-launcher-sqlite-fix`
is retained: identity and containment verified, status including untracked files
empty, ignored-file inventory empty, no worktree lock shown, but lsof found four
process working directories under its source/server/cmd/cercano. In-use safety
therefore prohibits removal. No processes were killed to permit cleanup.
The everyday launcher worktree remains retained as requested (previously in use
with ignored pycache); no other worktree was cleaned. Feature branch retained.
Main's unrelated untracked files were neither staged nor altered. This explicit
main checkpoint changes only this evidence document; tested source is unchanged.
