# Native migration rehearsal — BLOCKED before historical install

## Result

The bounded package rehearsal was attempted, not passed. Actual guest Homebrew reached `Installing cercano from migration/cercano` and verified the local historical archive, but could not execute its build sandbox inside the protective outer sandbox: `sandbox-exec: sandbox_apply: Operation not permitted`. Exit status 1. No historical keg exists, so explicit unlink, component installs, convenience upgrade, ownership acceptance and brew tests were **not run**. No failure/recovery baseline was manufactured from an uninstalled monolith.

The concrete blocker is macOS nested sandbox application, not missing authorization or an artifact checksum failure. The outer sandbox is required here to prohibit signals, TCP traffic, reads of real user contents/keychains and writes to `/Users/test` and `/opt/homebrew` while preserving the unmodified historical restart hook. Removing it without another validated containment mechanism would weaken hook isolation. Execution stopped rather than bypass it or mutate Homebrew source. The port setting is synthetic; no endpoint/agent was started and there were no conversations to drain. This does not prove successful hook execution or agent continuity.

## Completed checks

- Read root AGENTS.md and release/homebrew/README.md, including explicit migration and failure recovery guidance.
- Retrieved historical formula from canonical `cercano-ai/homebrew-cercano` base `caffd060597d6ca9706c9fa4db0c7c042722d7ef`. Original formula SHA256: `50f68b11106b7ca865bd931a0e945fc454c4275283d9e93ade230c2f548be5be`. Local fixture changes only archive URL to guest file URL; restart hook retained.
- Historical archive SHA256 `e57b4ff87d23241588df64e6f106b087775bb6c8939611fc9c540e907efef664` matches formula; v0.27.0 SHA256 `18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0` matches supplied trust anchor. Rechecked guest bytes.
- Both executables from both archives passed guest `codesign --verify --strict --verbose=2`. All identify Developer ID Application bryan costanich, TeamIdentifier `4HPM47RCM4`, Developer ID Certification Authority / Apple Root CA. This is not notarization/clean-Mac acceptance.
- Rendered all three canonical v0.27.0 formulas; SHA256SUMS records exact formula and artifact hashes. Canonical rendered files were not published or installed.
- Isolated copied brew checkout and synthetic HOME exist owned by UID501:GID20; guest brew config ran in that context. Existing `/opt/homebrew` was copied/read as source, not modified by package commands. No host brew invoked.
- Synthetic config port 49381, conversation and inert credential markers provisioned. End hashes match initial hashes in log: config `3152a352aa84d73093251920dc8f170997d4bf2811c86b8cdde167f110e7c70b`, conversation `9a9ec9ed1854cc95148fbeb12db10a011503255f7b7408aac9e77826dbb703b3`, credential `e583c11459158be2080b528872ea2a5595afd886c1e1509d6dd61d2d5bf3e902`.
- Snapshot listing confirms existing rollback snapshot `2f6bace3-6acf-4fcf-8e39-359c1efe3af0`; no snapshot restore/delete performed. No matching Cercano executable processes or fixture command links were observed at end.

## Attempts and limitations

`commands.jsonl` records exact argv, captured stdout/stderr and status. `commands-readable.txt` decodes transport hex for review. Transport failures (`PrlJob_GetResult` / `PrlJob_GetRetCode`, status255) are indeterminate guest results, not install successes. Transfer retries and direct checks established later fixture state.

Homebrew initially refused a prefix under its temporary directory; fixture was moved to `/opt/cercano-migration-20261115`, where brew config works. Local tap clone from an uncommitted working tree contained no formula; copied formula into the isolated installed tap, leaving unrelated copied taps untouched. Homebrew warns that prefix is nonstandard/Tier3 and copied `native/migration` tap is untrusted; it does not fetch that tap.

Installation attempts then failed in order:
1. Realpath metadata lookup of `/Users/test` denied by outer profile.
2. After changing read denial to file contents rather than metadata, Homebrew UNIX-domain build socket bind denied.
3. After permitting UNIX-domain sockets (TCP remains denied), Homebrew's nested build `sandbox-exec` failed `sandbox_apply` before a keg was installed.

The final guest profile is saved as `safety-final.sb`; setup.sh deliberately records the original stricter profile, not the final retry state. UNIX sockets were allowed for Homebrew IPC, not exercised as credential access. Process preflight rejects matching Cercano commands, and outer profile denies signals/TCP. Source inspection of historical dispatch/restart coordinator informed isolation, but source identity is not an independent binary reproducibility proof. Never run the historical hook uncontained against another HOME.

## Remaining gates

Historical native install/hook result, explicit manual migration, retained old-keg checks, exclusive command ownership, component/convenience brew tests, independent partial/missing-dependency recovery baselines, post-convenience snapshot recovery and A/B lag cases remain BLOCKED/NOT RUN. Installed-user applicability, real runtime/inference, human acceptance, notarization and clean-Mac signing acceptance remain pending. No publisher enabled; no push/merge performed.

## Retained fixture / cleanup inventory

- VM `08196962-608d-4fde-8284-49a5f020fea6` remains running; preboot snapshot above retained.
- Guest `/opt/cercano-migration-20261115` (~1.8GiB): copied brew checkout/portable Ruby and taps, local historical tap, downloaded archives, extracted signature-inspection binaries, cache/logs/tmp, synthetic HOME/state, scripts/base64 transfers and transport probe. No installed historical Cercano keg or fixture command links found. No old keg existed to remove or retain after install failed.
- Guest package invocations use explicit absolute fixture brew, fresh HOME, no auto-update/analytics/API installs/install cleanup and `HOMEBREW_NO_CLEANUP_FORMULAE=cercano,cercano-agent`; fixture persistent brew.env retains both exclusions. Policy was not extended to original prefix or another user.
- Worktree evidence is this directory; public archives remain local untracked files (excluded from checkpoint), with exact hashes. Scripts, formulas, logs, source excerpt and report are checkpointed. No spec.md/plan.md created. Nothing was deleted for cleanup; teardown requires separately chosen fixture-only removal and must not touch original user data or snapshot.
