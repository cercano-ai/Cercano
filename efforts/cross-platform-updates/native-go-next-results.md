> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Corrected native fixture verification and single synthetic guard experiment

## Scope and evidence

Disposable clone `{SANITIZED_VM_2}` only. Four-case run `native-go-20261106211304`; single additional synthetic guard run `native-go-20261106211614`. Prior evidence retained. No production/MSI/spec/plan changes, forced reinstall, custom restoration, fixture kill, or broader account/relaunch matrix.

Both runners verified the source SHA256 and reused existing clone executables only after exact SHA256 checks (`native-go-next-run.json`, `native-go-next-setup.json`, guard counterparts). No rebuild/download. The matched build manifest records Windows ARM64 / GOARM64=v8.0 for both binaries (`native-go-build.json`).

Known bytes:

| Item | SHA256 |
|---|---|
| Source | bea12a2ac5b86f237aa3abfb374f96ab74bed62073959ac39ce609eedf595438 |
| A executable, 1.0.0 | DECEF429AEE4CCAB0233D35DB619A85515CED6EC8E2CA689CECF9080DDD41E1E |
| B executable, 2.0.0 | 71EB50461447D4B654FF9FE6EEAF98D35D214C0ED8CFD806C99B82961A7E5B45 |
| A resource | E6FA40966629E22774B8E79E12EDAF0593DE521EC56F986B5C55CCE2B41CA5FE |
| B resource | F5718B4F1CC2902DEAE1DC0376488528C00869B400446812F39E9E0269E4F849 |

Sanitized manager stdout, numeric exits, snapshots, source manifests, installed hashes, direct/shim outputs, sentinels, and transcripts are retained inside `native-go-next-evidence.json` and `native-go-guard-evidence.json` under `files`. `native-go-commands.json` is the command/stdout/exit ledger. Nested snapshot `knownSourceMatches` contains nulls from nonmatching candidates; actual non-null matches and SHA256, not array truthiness, establish A/B identity.

## Fresh four-case run — COMPLETE

| Case | Held upgrade numeric exit | Actual manager report | Ordinary closed retry | Fresh direct + shim after retry |
|---|---:|---|---|---|
| Closed | 0 | upgraded 1/1, successful | 0; upgraded 0/1 (already B) | exit 0; B/resource-B |
| Running A | 0 | upgraded 1/1, successful; mapped A backup deletion warning | 0; upgraded 0/1 (already B) | exit 0; B/resource-B |
| Executable read/share-read lock | 1 | upgraded 0/1, 1 failed; unable to remove existing package, sharing violation (32) | 0; upgraded 1/1 | exit 0; B/resource-B |
| Resource read/share-read lock | 1 | upgraded 0/1, 1 failed; unable to remove existing package, sharing violation (32) | 0; upgraded 1/1 | exit 0; B/resource-B |

All initial A direct/shim launches returned exit 0 and exact `native-go-fixture version=1.0.0 resource=resource-1.0.0`. Every final B direct/shim returned exit 0 and exact `native-go-fixture version=2.0.0 resource=resource-2.0.0`. Closed held B also launched successfully before the no-op retry. Pack/install-A/ordinary uninstall numeric exits were all 0. Shim diagnostic logs are retained.

Running identity: PID 2428, exact run-specific executable path under `C:\ProgramData\chocolatey\lib\cercano-native-go-20261106211304-running\tools`, alive before and after held upgrade, unchanged creation identity (full timestamp in results). Ready record established A/resource-A. Installed files and resource became B while this same A process survived. Manager warned that it could not delete the mapped A executable under lib-bkp but still reported success. **Ordinary replacement alone therefore fails the running-refusal requirement.** The cooperative stop produced `cooperative-stop` and numeric exit 0 using the retained query handle.

Held-state hashes: closed had B payload/resource. Running had B payload/resource in lib and A executable retained in lib-bkp. Both lock failures left A payload/resource in lib, metadata 1.0.0; lib-bad held the conflicting A executable or A resource respectively under the attempted 2.0.0 directory. These are observations, not an atomic-preservation requirement. After releasing the owned locks, ordinary upgrade succeeded with B hashes and B fresh launches, without force or custom repair.

## Exactly one synthetic guard experiment — COMPLETE

Only the running case was exercised. Both versions contain full-path process checks in `chocolateyBeforeModify.ps1` and a new `chocolateyInstall.ps1` reporting backstop. The synthetic guard enumerates CIM processes, identifies the run-specific executable name or paths under this package's lib/lib-bkp/lib-bad roots, requires executable path/creation/command line for candidates, canonicalizes the path and requires root membership; unknown candidate information throws rather than allowing modification. Enumeration failure also throws. No process is signalled by these scripts. This is not production discovery or a race-free veto guarantee.

`running/native-go-guard-events.jsonl` records both stages detecting PID 7908 at the exact package executable path, creation `2026-11-06T21:16:27.4074200Z`, matching command line, `fullPathInPackageRoots:true`.

Actual held manager numeric exit: **-1**. BeforeModify threw `SYNTHETIC RUNNING REFUSAL at BeforeModify : exit the owned fixture and retry ordinary upgrade`; Chocolatey treated that as a warning and **continued** to the new install script. The InstallBackstop detected the same process and threw the corresponding actionable message. Final stdout: `The upgrade ... was NOT successful`, `Attempting rollback`, `Chocolatey upgraded 0/1 packages. 1 packages failed.`, `Exiting with -1`. Debug logging also contains a formatting-string diagnostic; the exact guard-event records and both executed error messages establish that both checks actually ran.

Before release, metadata and lib payload/resource were A; lib-bad contained B payload/resource. The A process remained alive with identical PID/path/creation identity. Cooperative stop returned exit 0 with A/resource-A then `cooperative-stop`. An ordinary closed retry returned **0**, upgraded **1/1**; both fresh direct and shim launches returned exit 0 with B/resource-B and installed B hashes. Ordinary uninstall returned 0. No custom restoration or force was used: rollback was Chocolatey's own behavior.

This demonstrates one bounded supported failure/ordinary-retry path using the reporting backstop, NOT that BeforeModify vetoes replacement, nor that a package install script prevents extraction, races, or all side effects. Unknown-path failure branches were implemented fail-closed but were not separately injected/tested.

## Synthetic data, cleanup, and residuals

Both runs' synthetic before/after Path/Hash sentinel arrays are exactly equal. Both completed with no failure file. Collected owned process lists are empty after cooperative teardown and manager completion.

All five new package IDs were ordinarily uninstalled; cleaned snapshots show no payload/shim or lib/lib-bkp/lib-bad files. Per-ID `.chocolatey` bookkeeping remains: A `.arguments`, `.files`, `.sourceInstalledFrom`; B `.arguments`, `.files`. Run directories, feeds, packages, scripts, logs, synthetic sentinels, and control files remain as owned evidence. No attempt was made to erase them or touch historical leftovers.

Historical installed packages `cercano-native-go-20261106201208-closed|2.0.0` and `cercano-native-go-20261106210423-running|2.0.0` remain unchanged in the final metadata collection. No claim of VM-wide cleanup or user-data coverage.

## Boundary

Stop here: the requested basics and the single synthetic experiment are complete. Ordinary replacement is not a running-refusal solution; BeforeModify alone is not a veto. The new install-script backstop yielded an actionable nonzero final status followed by successful ordinary retry in this fixture only. No product/MSI implementation or acceptance claim follows from this result.
