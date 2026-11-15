# Bounded native packaging failure/recovery continuation — 2026-11-15

## Scope and containment

Only macOS VM `08196962-608d-4fde-8284-49a5f020fea6` was used. Windows was not touched. Separate disposable prefixes `/opt/cercano-failure-20261115-{conflict,recovery,lag}` preserve the successful `/opt/cercano-migration-unpriv-20261115` fixture. Conflict/recovery start from independently installed historical monolith 0.25.2; lag starts empty. No host brew, real credentials, live publication, unrelated install changes or guard edits.

All brew actions use the root-owned mode555 wrapper, real UID/GID5501, fresh environment, outer sandbox, supported avoid-nested setting, disabled auto-update/analytics/API/cleanup and preserved old kegs. Profiles are root-owned mode444 under root-owned mode755 roots. The copied profile additionally denies successful-fixture access. Control-plane root only prepares/inspects these disposable fixtures; brew itself is unprivileged. Records105–110 contain identity/config/profile/artifact hashes. Record135 confirms profile hashes and both Homebrew guard sources identical to the successful fixture. Existing nonfatal sandbox `ps` EPERM warnings remain; no suppression or weakening. Offline version/tests do not prove agent/runtime continuity.

Exact artifacts remain the verified release archives, no invented versions:
- 0.25.2 archive SHA256 `e57b4ff87d23241588df64e6f106b087775bb6c8939611fc9c540e907efef664`.
- 0.27.0 archive SHA256 `18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0`.

The agent-only 0.25.2 definition below is explicitly a **packaging fixture over genuine historical monolith bytes**, NOT a published historical split package. Mixed-set checks are offline topology/command evidence, not protocol/runtime compatibility or a real A/B split-release acceptance claim.

## Exact case outcomes

| Case | Result | Exact evidence (1-based commands.jsonl record) |
|---|---|---|
| Ordinary dependency-first `brew upgrade --formula migration/cercano/cercano`, no overwrite | **PASS observed migration/topology; obsolete fixture assertion failed** | 124: brew exits0, installs agent/CLI dependencies and convenience 0.27.0. 125: exact links select `../Cellar/cercano-agent/0.27.0/bin/cercano` and `../Cellar/cercano-cli/0.27.0/bin/cercano-cli`; historical 0.25.2 keg retains original executable hashes. The recorded script exits1 only because it incorrectly expected brew failure. No ordinary-upgrade conflict was reproduced. Corrected `case-conflict.sh` expects success and captures before/after hashes and all postconditions even if brew fails; it has NOT been rerun. |
| Direct link against still-linked historical owner, no overwrite | **PASS**, distinct from ordinary upgrade | 134: recovery fixture `brew link .../cercano-agent` exits1, `Could not symlink bin/cercano`, target belongs to cercano. Historical link remains. Suggested overwrite command was NOT executed. |
| Missing CLI artifact after successful agent install, before convenience; safe rollback | **PASS** | 126, `case-recovery.sh`: only CLI file URL changed to absent fixture file, original version/hash retained. CLI install exits1; no CLI/convenience 0.27.0 keg. Unlink agent then link preserved monolith restores both links and original binary hashes. Formula restored. |
| Repository lag: convenience/agent present, CLI formula absent | **PASS** | 128, `case-lag-partial.sh`: convenience install exits1 `No available formula ... cercano-cli`; no convenience keg. CLI formula restored. |
| Independent component installs and mixed package set | **PASS bounded packaging** | 128: historical-byte agent-only fixture 0.25.2 installed independently; CLI0.27.0 independently installed; both tests pass. Convenience0.27.0 installs/tests with agent0.25.2 still selected, demonstrating no forced exact version match. Convenience has no bin. Not genuine split-release A/B compatibility. |
| Independent agent upgrade and CLI/convenience removal/reinstall | **PASS bounded packaging** | 133, `case-independent-uninstall.sh`: agent0.25.2→0.27.0 upgrades while CLI/convenience remain0.27.0. Agent test passes. Uninstall convenience leaves both components executable; uninstall/reinstall CLI leaves agent executable; reinstall convenience/test passes, no bin. |
| Uninstall preserves synthetic config, conversation and inert credential marker | **PASS** | 133: uninstall convenience, CLI and latest agent; hashes all OK. Homebrew retains agent0.25.2 (normal retained keg, not falsely described as complete uninstall). 134: second ordinary agent uninstall removes remaining0.25.2; Cellar agent directory absent, formula list empty, all three hashes still OK. No force/cleanup and no original-fixture uninstalls. |
| Post-convenience injected failure and snapshot rollback | **NOT RUN** | 104: attempted dedicated linked clone refused while source VM running. No dedicated clone/disposable VM snapshot obtained. Did NOT inject this failure, stop/reset source VM or restore its unrelated snapshots. Prefix separation is not claimed as VM snapshot rollback evidence. |
| Real A/B split-release repository lag/protocol/runtime compatibility | **NOT RUN** | Only exact archives0.25.2/0.27.0 available; historical0.25.2 was monolithic. Packaging-only mixed set is explicitly limited above. |

Initial record122 failed at tap trust before the upgrade under test. Disposable tap trust was configured (123), then the actual ordinary-upgrade case124 ran. Transport `PrlJob_GetResult: Invalid argument` records112/129 were not passes; individual staging was retried and succeeded. No formula guard change.

## Preservation / end state

Record135 samples the conflict fixture synthetic config `02a18e7a…`, conversation `9a9ec9ed…` and inert credential `e583c114…`; this is not a before/after ordinary-upgrade assertion or live continuity evidence. The corrected script adds that before/after check for a future authorized fresh fixture only.

Record135: successful fixture historical binary hashes, selected split links, config `41605ae5…`, conversation `9a9ec9ed…`, inert credential `e583c114…` and profile `5c7f8de1…` match prior continuation. No matching Cercano executable process found. This is sampled preservation, not a whole-guest unchanged attestation. Record136 lists BOTH original snapshots `2f6bace3-6acf-4fcf-8e39-359c1efe3af0` and `9b24ada1-0594-4f31-9418-c19197eb225a`; neither restored/deleted. Successful fixture untouched; new conflict/recovery prefixes retain old monolith kegs. Lag prefix retains synthetic data, logs, cache and definitions after its authorized uninstall case.

## Reproduction / evidence

`failure-cases.py setup` refuses existing roots. Then `stage conflict`, `stage recovery`, `stage lag`; trust the disposable migration tap before cases. Run guest scripts via `python3 failure-cases.py guest < case-*.sh` from this directory. Lag order: `lag-agent`, `case-lag-partial.sh`, `stage lag`, `case-independent-uninstall.sh`; final second ordinary agent uninstall is recorded134. Cases are stateful, not idempotent. Do not rerun setup over preserved fixtures.

`commands.jsonl` is authoritative full argv/status/stdout/stderr; `commands-readable.txt` is regenerated including decoded hex guest scripts. No release archives or guest data checked in. The obsolete ordinary-conflict assertion is a fixture defect, not a failed brew migration or shipping gate. Shipping remains blocked by unrun safe snapshot rollback, real A/B runtime acceptance and prior live-session/signing/human/publication gates. This correction performed no guest actions or native rerun; recorded topology and sampled preservation do not establish live session continuity.
