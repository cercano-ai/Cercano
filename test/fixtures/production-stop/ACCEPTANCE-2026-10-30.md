> Historical evidence, superseded for upgrade acceptance by HOST-ONLY-2026-10-31.md.
> Descendant death, writer-tool tick cessation, descendant freeze/STOP, and private
> stdio shutdown are NOT current upgrade requirements. Workers/tools may survive;
> only independently verified DB-writing hosts are stopped. Independent writer
> uncertainty still refuses. Historical results below are not current assertions.

# Fresh bounded ordinary-launcher/native-stop acceptance — 2026-10-30

## Fresh execution

Both requested acceptance runs completed in this execution from checkpoint `0697bb8ab977bd8729d371242daac85d9427fe98`; neither result below is retained historical success. Only this document was changed in tracked source.

Working directory for all commands:
`/Users/bryancostanich/git_repos/cercano-ai/Cercano-upgrade-fixes`

Exact build and acceptance commands (all exit **0**):

```sh
python3 test/fixtures/production-stop/build.py
python3 test/fixtures/production-stop/run.py combined
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo /Users/bryancostanich/git_repos/cercano-ai/Cercano-upgrade-fixes --evidence /private/tmp/fresh-acceptance-20261030-m0apb27h
```

The three environment assignments on the last command were supplied as process environment overrides. Durations: build 0.771s, combined 37.734s, compiled A/B 115.117s. The existing build driver rebuilt all five required artifacts from current source using offline Go dependencies: untagged production host, ordinary-upgrade helper, native fixture upgrade helper, client, and standalone upgrade fixture. Go build cache was allowed; old binaries were not used without running the build commands. Full Go argument lists/output are in `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/upgrade-build-4zx8sfmu/output.log`.

Expectations computed before execution: combined confirmation → one native shutdown → owned writer exits → one backup → one migration to schema 2 → generation/history checks; compatible A/B zero shutdown/migration calls and zero upgrade backups; both owned cleanup sets empty. The private `expectations.json` was persisted after combined and before A/B; the expectations themselves were stated before either run.

## Combined evidence and assertions

- Result: `/private/tmp/cstop-zgipsem9/result.json`
- Native fence: `/private/tmp/cstop-zgipsem9/native-stop.log`
- Launcher trace: `/private/tmp/cstop-zgipsem9/upgrade-calls.jsonl`
- PTY: `/private/tmp/cstop-zgipsem9/generation.pty`
- Backup: `/private/tmp/cstop-zgipsem9/state/conversations.before-upgrade-mwy6e4ce.db`

`fixture_combined_acceptance: true`. A live tool writer, held provider request, and later-created production worker were observed. Before confirmation, assertions required no native stop record and no backup. After confirmation the two helper calls were check-only with `--stop-hosts-config`, then migration without those flags. Native stop recorded exactly `stop-start`, `verified-stop-before-backup`; the native hook verified the explicit host and writer descendants were exited (or zombies) before backup. Exactly one backup retained schema 0 and the running task; migration reached schema 2 with SQLite integrity `ok`.

The replacement generation selected the same store, preserved all three historical turn rows, displayed `BEFORE_STOP_REPLY` on resume, persisted a fresh `FIXTURE_OK` reply in the original conversation, and changed the fixture task to `interrupted`. Writer tick count remained unchanged across the post-upgrade wait. Host exit was **-9**, not graceful shutdown.

## Compatible compiled A/B evidence

- Report: `/private/tmp/fresh-acceptance-20261030-m0apb27h/compiled.json`
- Private runtime/source root: `/private/tmp/ca-azjde0k6`
- Additional verification: `/private/tmp/fresh-acceptance-20261030-m0apb27h/verification.json`

`passed: true`; `shutdown_calls: 0`, `migration_calls: 0`, `upgrade_calls: []`, and assertion of no upgrade backups passed. Fresh bootstrap and A/B generation builds ran inside the disposable source copy. Snapshot `9d028265fd0feabd927649bcad110fa373584840`; source tree `3930cf8b634e50b5181b578db973bf2972a5e5fd`.

Assertions also covered failed B build/startup retaining A, B publication without stopping A, new sessions choosing B, real fixture MCP execution, authenticated settings mutation affecting model requests, a newly born pinned A worker after B, independent task completion, original history resumed on B after client restart, automatic drained A retirement, and continued durable B replies.

## Cleanup and scope

Combined `cleanup_remaining: {}` and `generation_cleanup.remaining: []`; A/B `cleanup.remaining: []`. An additional exit-0 verification loaded both fresh JSON reports, checked native fence/counts, and queried only the 18 recorded owned PIDs (including shell writer descendants) with `ps -p <pid> -o stat=,command=`. Every PID was absent; exact observations are retained in `verification.json`. No ambient process discovery or stop was performed.

Fresh binary SHA256 values:

- Production host: `5b98c29b3c0e6c2d7a8c1098a14f8eb2c3efa9f82ca30330ce465d3aae59b655`
- Native upgrade helper: `3b985e188912ea210f3088e16b84d779a47aef68490dbb09f1cab42a56fb7bfe`

Native shutdown used only the existing explicitly owned fixture-host hook. Configurations, databases, providers, and runtime state were disposable; no live configuration/database access or live process signals were performed. The compiled harness created its existing private launcher shim under `/private/tmp/ca-azjde0k6/bin`, not a production installation. No production source changes, activation, land, or push. This is bounded synthetic-provider acceptance, not external-provider or ambient production-discovery acceptance.
