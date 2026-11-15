# Authorized clone-local post-convenience recovery — PASS (bounded packaging)

## Evidence and scope

This reconciles the persisted execution evidence in `commands.jsonl` records137–220 (1-based lines), not a claim that all operations were freshly rerun during documentation. Final inventory was freshly queried at record220. It supersedes the earlier NOT RUN post-convenience rollback entry only. Full runtime/installed-user acceptance is **not established**. No model runtime, live session drain/restart, real credentials, tap publication, push or merge was exercised.

## Source inspection, identity, safe clone route

Records137–144 identify source `08196962-608d-4fde-8284-49a5f020fea6`, originally running, and inspect workloads before shutdown: no active Cercano agent/brew workload requiring preservation was identified. Selected links already pointed to 0.27.0 components; historical binaries and synthetic marker hashes were captured. Record143 verifies snapshot `9b24ada1-0594-4f31-9418-c19197eb225a`: `cercano-historical-installed-20261115`, `2026-11-15 00:54:25`, state `poweron`, description `Isolated UID5501 v0.25.2 real historical formula installed; no active agent; before manual split.` This identity check preceded clone operations.

Exact control-plane sequence (full flags, output and decoded guest scripts in the log/recovery transcript):

1. `prlctl stop 08196962-608d-4fde-8284-49a5f020fea6` failed `Operation canceled` (145). Guest graceful shutdown (146) lost transport (`PRL_ERR_IO_STOPPED`); status147/148 confirmed stopped. No force stop/reset.
2. `prlctl clone 08196962-608d-4fde-8284-49a5f020fea6 --name cercano-acceptance-rollback-20261115-1129 --linked --id 9b24ada1-0594-4f31-9418-c19197eb225a` was rejected (149): linked clones require a snapshot created while shut down, whereas this snapshot is powered on.
3. Documented `clone --force-deep-copy` route succeeded (150), creating separate VM `5df50e96-bb17-4248-aa6e-50ae559cd765`, name `cercano-acceptance-rollback-20261115-1129`, including snapshot tree. Source was successfully started (151), restoring its prior running state. All subsequent restores target only that clone.
4. Plain clone `snapshot-switch --id 9b24ada1-0594-4f31-9418-c19197eb225a` failed restoring saved VM state (154). Initial rebuild experiment used cold snapshot `f0b49e47-7145-43c3-b5ba-3b18bb564a0d`, `acceptance-historical-flushed-20261115` (164–185). That experiment alone is not evidence of restoring the requested source snapshot.
5. `prlctl snapshot-switch --help` documents `--skip-resume` (190). While clone stopped (191), `prlctl snapshot-switch 5df50e96-bb17-4248-aa6e-50ae559cd765 --id 9b24ada1-0594-4f31-9418-c19197eb225a --skip-resume` succeeded (192), then clone start succeeded (193). This safe route restores snapshot disks without resuming the incompatible saved powered-on execution state; source was never reverted. Initial guest call194 failed before Tools readiness. Probe195 failed because the earlier recovery prefix was correctly absent after restore; inspection196 found historical source fixture material. Do not claim preservation/resumption of the original live memory state.

## Definitive post-restore test

After the successful requested-snapshot restore, record197 ran `python3 snapshot-recovery.py guest < case-snapshot-setup.sh`. Setup refuses an existing recovery root, copies empty historical brew and verified release archives into **clone-only** `/opt/cercano-snapshot-recovery-20261115`, installs real 0.25.2 with unchanged restart hook, and creates synthetic config/conversation/inert-credential markers. Historical bytes were freshly installed in this separate prefix; it is not the original prefix's runtime state.

The root-owned mode444 outer profile retains network/signal denials and protected `/Users/test`, `/Library/Keychains`, `/opt/homebrew`, prior fixture and successful fixture restrictions. Its SHA256 is `06ba86297f7adf399e6aa634fce0773bdb67df8682e58ae4ae064f0c6154cdae`. UID/GID5501 executes brew with supported avoid-nested opt-in; Homebrew guard files compare unchanged. Narrow loopback permission was authorized but **not needed or enabled** for this offline recovery test; no agent/CLI endpoint exchange or runtime acceptance is claimed. No sandbox was broadly disabled. Nonfatal Homebrew `ps` EPERM warnings remain, as do Tier3/custom-prefix warnings.

Guest shutdown198 returned255 as transport closed; status199 confirmed stopped. Record200 creates clone-local cold snapshot `7c251a7a-8c29-4ff4-aab3-635ae38954e9`, name `acceptance-historical-cold-20261115-1215`, description explicitly distinguishing newly installed historical bytes from original snapshot runtime state. Start201 succeeds.

`python3 case-snapshot-migrate.py` (202) stages canonical local-artifact 0.27.0 definitions, unlinks monolith, installs agent/CLI, upgrades convenience formula and verifies convenience has no bin directory. Selected commands point to agent/CLI component kegs. It deliberately renames only the synthetic CLI executable to `cercano-cli.INJECTED-FAILURE`; the CLI symlink becomes dangling **after convenience replacement**. It prints `INJECTED POST-CONVENIENCE CLI UNAVAILABLE; no repair performed`, preserves markers, and syncs. No brew relink, cleanup or manual repair precedes rollback.

Shutdown203/status204, then `prlctl snapshot-switch 5df50e96-bb17-4248-aa6e-50ae559cd765 --id 7c251a7a-8c29-4ff4-aab3-635ae38954e9` (205) and start206 succeed. `python3 snapshot-recovery.py guest < case-snapshot-verify.sh` (207) exits0 and verifies:

| Restored file | Exact SHA256 |
|---|---|
| `brew/Cellar/cercano/0.25.2/bin/cercano` | `effc0cb3c0ec4785b0f2244b872d75f0621249247ca067363c3d49b980c5b24e` |
| `brew/Cellar/cercano/0.25.2/bin/cercano-cli` | `c94534e942967207dacfa715ad534a3abddf75eb453d3ca75c501d71feee42d2` |
| `home/.config/cercano/config.yaml` | `ee4cd98070adf3eb294279793c7fa9415e3f1e7743939fd894de9b2326033157` |
| `home/.cercano/conversation-marker` | `9a9ec9ed1854cc95148fbeb12db10a011503255f7b7408aac9e77826dbb703b3` |
| `home/.cercano/credential-marker` | `e583c11459158be2080b528872ea2a5595afd886c1e1509d6dd61d2d5bf3e902` |

Both `brew/bin` symlinks resolve exclusively to `Cellar/cercano/0.25.2/bin`, with symlink and executable ownership UID5501. Only monolith0.25.2 remains in Cellar, historical tap restored, component formula absent; brew lists `cercano 0.25.2`. Both `--help` commands execute under outer sandbox; no matching executable process is reported. This verifies whole-VM snapshot recovery of the tested tap/Cellar/links/synthetic state, not merely a prefix copy or `brew link` fallback.

Earlier setup attempted a custom remote on a non-Git tap and failed; changed to supported `trust migration/cercano`. Earlier migration experiment's unqualified `list --versions` hit missing Caskroom; script now uses `list --formula --versions`. These failed diagnostics are retained, not labeled passes.

## Cleanup and residual inventory

Shutdown208 failed `PrlJob_GetRetCode: Invalid argument`; retry209 prints `Shutdown NOW!`, status210 confirms clone stopped. No VM/snapshot deleted. No source snapshot restored/deleted. No fixture uninstall or cache/keg cleanup in this slice. Retention is deliberate evidence preservation.

- **Source running** (211/216 and fresh220), successful split fixture retained at `/opt/cercano-migration-unpriv-20261115`; original `/opt/cercano-migration-20261115` and failure prefixes `/opt/cercano-failure-20261115-{conflict,recovery,lag}` retained (215). Source snapshot tree remains only `2f6bace3-6acf-4fcf-8e39-359c1efe3af0` → `9b24ada1-0594-4f31-9418-c19197eb225a` (212–214/218). Source selected split links, exact old binary hashes, synthetic config/markers and original safety hash match pre-stop samples (215). This is sampled preservation, not whole-guest unchanged attestation.
- **Clone stopped** (210/216 and fresh220); `/opt/cercano-snapshot-recovery-20261115` retained at restored historical baseline, along with inherited original/unprivileged fixture paths (209). Clone snapshot tree retains inherited two snapshots plus `f0b49e47-7145-43c3-b5ba-3b18bb564a0d` and current `7c251a7a-8c29-4ff4-aab3-635ae38954e9` (217). No teardown/deletion of these retained artifacts.
- Other inventory220: Windows11 stopped; native acceptance `d5633dbc-c8a1-49fc-a420-df6d5d60d952` suspended; Windows packaging `8f18b5fd-37bf-476e-8b2b-a14ff7784a0b` suspended. None operated on.

Remaining gates: genuine split-release A/B protocol/runtime compatibility, installed-user applicability, live drain/restart continuity, external llama/model runtime, human acceptance, notarization/clean-Mac signing and publishing integration. Packaging recovery PASS does not authorize shipping/publication.

Scripts are stateful, not general-purpose rerunnable automation: setup requires missing recovery root; transport is pinned to the acceptance clone. Do not rerun over retained evidence. `recovery-commands-readable.txt` decodes records137 onward; original JSONL remains authoritative.

## Final checkpoint validation

Record221 freshly confirms source running and acceptance clone stopped before checkpoint. No stateful fixture operation was rerun over retained evidence. `bash -n case-snapshot-{setup,verify}.sh`, Python AST parsing of `snapshot-recovery.py` and `case-snapshot-migrate.py`, and `git diff --check` passed. These syntax/document checks do not expand native runtime acceptance.
