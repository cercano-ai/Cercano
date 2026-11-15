# Authorized continuation: historical install and manual split PASS (bounded fixture)

This supersedes the execution blockers in RESULTS.md, not its historical record or pending shipping gates. Read AGENTS.md, the migration recipe and checkpoint d042c318cb9a before execution. New transport records follow record88 in commands.jsonl; commands-readable.txt decodes their scripts.

## Diagnosis and containment

- Actual process UID/GID5501 groups are `[5501,12,61,100,701]`. None is GID0/wheel,20/staff,80/admin. Unmodified Homebrew's guard names admin/staff/root/wheel; missing root directory group is handled by Homebrew itself. `_lpoperator` is not that guard. The successful real install with `HOMEBREW_AVOID_NESTED_SANDBOXING=1` demonstrates acceptance, not merely an inferred permission.
- Relevant authority is scoped: the identity owns its copied prefix and synthetic state, not root, real HOME or original brew. `_lpoperator` inherits localaccounts; it is not generic system administration. Guest CUPS config names `SystemGroup admin`, system.print.admin, and separate operator-auth job operations. Its UNIX socket is world-accessible and the existing profile permits UNIX sockets; this is NOT an assertion that all local IPC or print authority is absent. No CUPS request, authorization challenge, credential query or print action was performed. Print-specific membership alone does not confer Homebrew's four-group privilege or invalidate the demonstrated real-state/signal/TCP denials. There is no general sandbox-escape proof here.
- Corrected fresh-probes.rb: self `kill(0)` is an expected successful existence probe, not destructive signaling. An unsandboxed parent running as UID5501 forks an inert sleeping helper; its sandboxed child attempts TERM only on that owned helper. TERM returns EPERM, and the parent confirms helper existence, then terminates/reaps only its helper outside the sandbox. No unrelated process is signaled. Pre-install and final probes both exit0.
- Positive owned-prefix read/write and UNIX bind pass. Real HOME/keychain/original brew/previous fixture open-only probes deny access (EACCES or EPERM); synthetic loopback TCP denies EPERM. No protected data bytes read or written. EACCES may also reflect DAC. Profile remains byte-identical, SHA256 `5c7f8de194d70965d1299eaf6c2c9e900fe2e5c9c461bbc4056b843f7f916dd6`, root-owned mode444 beneath root-owned mode755 prefix. Both Homebrew guard sources compare identical to prior copies. No identity/source guard patch, sandbox relaxation or group edit.
- New root-owned unpriv-brew-fixture.sh uses fresh env, explicit copied brew, actual account, outer sandbox, supported avoid-nested setting, no auto-update/analytics/API install/automatic cleanup, and persistent retention exclusions for cercano AND cercano-agent. Process preflight runs outside sandbox and rejects matching executable names.

## Executed historical baseline

`brew config` records custom prefix and supported opt-in in actual context. Real `brew install --formula migration/cercano/cercano` succeeds, installs v0.25.2, and invokes the unchanged historical `restart-after-upgrade` post-install hook. Hook log: **No running agent owned by this installation; nothing started.** No matching agent executable was found before/after. There are only inert conversation markers, no live conversations to drain; this is no-agent hook evidence, not active drain/restart continuity.

Historical ownership was both commands under Cellar/cercano/0.25.2. Baseline executable SHA256:
- cercano: `effc0cb3c0ec4785b0f2244b872d75f0621249247ca067363c3d49b980c5b24e`
- cercano-cli: `c94534e942967207dacfa715ad534a3abddf75eb453d3ca75c501d71feee42d2`

Added pre-transition VM snapshot `9b24ada1-0594-4f31-9418-c19197eb225a` after historical installation/no-agent baseline and before staging split formulas. Original snapshot `2f6bace3-6acf-4fcf-8e39-359c1efe3af0` remains. Neither restored or deleted.

## Manual migration and checks

1. Staged all three v0.27.0 canonical rendered definitions together in isolated installed tap; changed only archive URLs to same local fixture file. Both archives retain trusted recorded hashes.
2. Asserted historical links target v0.25.2 and both components absent. Explicit `brew unlink migration/cercano/cercano` removed exactly two links. Old keg executable hashes unchanged.
3. Installed agent alone; only cercano link exists and no CLI link yet. Installed CLI separately; inspected its files and both canonical link targets.
4. Only then ran `brew upgrade --formula migration/cercano/cercano`, successfully 0.25.2 -> 0.27.0.
5. `brew test` agent, CLI and convenience all exit0. Both commands report v0.27.0 under actual identity/outer sandbox. Convenience keg has metadata/license/share README only and no bin directory. Current links exclusively select component kegs; saved historical binaries remain unlinked, not a second active owner.
6. `brew list --versions`: cercano 0.25.2 0.27.0; cercano-agent 0.27.0; cercano-cli 0.27.0. Old executable hashes unchanged after convenience upgrade. Persistent brew.env retains cercano,cercano-agent exclusions and disabled automatic cleanup. Component files owned5501:5501.
7. Config hash remains `41605ae5178735b19b7f16419bc6edb4ed89a1e495988a1040828c04709f9029`; conversation `9a9ec9ed1854cc95148fbeb12db10a011503255f7b7408aac9e77826dbb703b3`; inert credential `e583c11459158be2080b528872ea2a5595afd886c1e1509d6dd61d2d5bf3e902`. Prior fixture sampled state/profile hashes also unchanged. No full protected-tree unchanged attestation.

## Diagnosed non-successful diagnostics / warnings

- An optional historical `brew test` was issued after staging new definitions. Homebrew correctly refused because installed historical version was no longer latest. No migration/package step failed; historical hook result comes from real install/log, not that test. All three final split tests passed.
- Bare `brew list migration/cercano/cercano` after convenience upgrade incorrectly attempted missing Caskroom in this copied fixture. Repeated with explicit `--formula` succeeds and enumerates the convenience keg. No Caskroom or source patch needed.
- Homebrew emits nonfatal `brew: exec failed (EPERM): ps` during its post-action checks under outer sandbox, while enclosing installs/upgrades exit0. Historical hook log separately establishes no-agent result; external process preflight/final inspection provides process evidence. This warning was not removed by weakening containment.
- Custom prefix is Tier3; copied brew has no Git revision. Codesign/archive checks remain prior bounded evidence, not notarization or reproducibility.

## Concrete remaining shipping gates

The happy-path manual topology transition is now demonstrated. It is not shipping acceptance. Independent snapshot-separated ordinary dependency-first conflict behavior, missing/failed component recovery before convenience, snapshot restore recovery after convenience, and independent A/B component/convenience repository-lag cases remain NOT RUN. No A/B archive was supplied/constructed in this continuation. These require distinct baselines; the successful path must not be relabeled as recovery evidence.

Actual installed-user applicability, active-session drain/restart continuity, full external llama/model runtime behavior, human acceptance, notarization/clean-Mac signing and publishing integration remain pending. No agent/runtime endpoint, real credential, host brew or live tap publication was used. No push/merge/publisher enablement.

Retain both snapshots, UID/GID5501 and entire new fixture including old/new kegs, state, caches/logs, helper scripts/probes. The helper processes were reaped; unique probe socket paths removed by their creating process. Preserve `/Users/test`, `/opt/homebrew` and prior fixture. No package cleanup/uninstall or other teardown performed. Checkpoint contains scripts/evidence only, never archives or guest state.

Local validation: `python3 -m unittest discover -s release/homebrew -p 'test_*.py'` passed70 tests; `ruby release/homebrew/test_formula.rb` passed5 runs/40 assertions. These do not replace native acceptance gates.
