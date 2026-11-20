# Windows offline upgrade — durable evidence summary

Bookkeeping audit: 2026-11-20. This summarizes retained 2026-11-19 evidence, not newly executed tests. Implementation checkpoints: `088ea6cd` (production upgrade), `d73b699e` (Windows NTFS backup), `eeef482f` (startup-binding fixture isolation), `69c3d877` (guarded manual Windows entrypoint). All are ancestors of the audited branch HEAD. No tracked implementation/test edits remained uncommitted.

## Native Windows ARM64 final selection

Inspected local `followup.ps1` and `followup-native-final.log` under `.windows-review-evidence/`. The log identifies `ARCH=ARM64`. Process-local TEMP/TMP used the canonical long path under the user profile, avoiding the default short alias without changing system configuration. Compiled package test binaries ran from separate `cmd/cercano` and `internal/conversation` working directories, with the frozen `testdata/production-v2.sql` available to relative fixture reads:

```powershell
$env:TEMP=Join-Path $env:USERPROFILE 'AppData\Local\Temp'
$env:TMP=$env:TEMP
# From the scratch tree's cmd/cercano directory:
& "$d\cmd.exe" '-test.run=Upgrade|upgrade|Backup|Binding|Windows' '-test.v' '-test.count=1'
# From the scratch tree's internal/conversation directory:
& "$d\conversation.exe" '-test.run=Upgrade|upgrade|Backup|Binding|Windows' '-test.v' '-test.count=1'
```

Both report `PASS`, `CMD_EXIT=0` and `CONVERSATION_EXIT=0`. Exactly two existing subtests skip:

- `TestWindowsOwnershipShortAndReparseAliases/reparse`: symlink privilege unavailable; no privilege change was made. Unprivileged junction inventory coverage passed.
- `TestOfflineUpgradeSyncFailureBeforeMutation/directory`: Windows uses write-through publication rather than Unix directory sync; Windows publication-failure tests cover that boundary.

Coverage includes actual private fixture database upgrade, absent/empty inventory, private locking cancellation/release, malformed/semantic/unreadable/hardlink/directory/junction/noncanonical inventory refusal, retained renamed-host refusal, known-process liveness/exit, automatic-stop no-effects and continued development-lifecycle refusal. Earlier attempts are not substituted for this final passing selection: a default short TEMP alias correctly failed path validation, and review reproduced a `retained.JSON` acceptance gap before the final case-sensitive entry policy fixed it.

## Darwin regression evidence

The committed plan and checkpoint history record this four-package regression command from `source/server` as PASS after the Windows backup/manual work:

```sh
go test ./internal/conversation ./cmd/cercano ./internal/brewrestart ./internal/server -count=1 -timeout=120s
```

The four-package claim is retained from that committed record; the local scratch logs do not independently contain its entire output. The plan also records this focused passing command:

```sh
go test ./cmd/cercano ./internal/conversation -run 'Upgrade|Development|Ownership' -count=1
```

Its recorded results are 4.947s / 0.830s. Local `darwin.log` separately contains passing two-package output (4.691s / 0.834s) but no command header, so is not treated as an exact reproduction of those timings. `followup-darwin.log` contains subsequent passing cmd/conversation output (1.271s / 1.008s), also without an exact invocation header. No new suite was run during this documentation audit.

## Completed scope and limits

Only the approved offline-upgrade prerequisite is complete. Existing conversations remain host-mode with no binding backfill. Windows manual `upgrade-store` supports absent/empty retained-host inventory, not host lifecycle management. Private validated paths/ACLs and local NTFS are required for Windows backup publication. Retained hosts, unknown/noncanonical entries (including ancillary token files), uncertain known-process inspection and `--stop-hosts-config` refuse; inventory is not deleted or bypassed. Operators must stop all writers and keep them stopped; process checks are not a universal fence against renamed/custom or newly launched writers.

No user database migration, physical power-loss qualification, Linux runtime qualification or full repository suite is claimed. Barrier ordering/error tests and documented write-through OS contracts are not proof of hardware crash durability. Broader routing inventory, authentication/transport, shared workspace extraction, client execution/reconciliation and end-to-end acceptance remain pending in `plan.md`; approved spec scope is unchanged.

Raw binaries, logs, SQL copies, helper scripts and scratch remain locally in `.windows-review-evidence/`, intentionally uncommitted and undeleted. This concise summary is the durable repository evidence; no VM commands, new implementation, merge, landing or cleanup occurred during bookkeeping.
