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

## Follow-up diagnosis — supported opt-in blocked by fixture identity

Follow-up to checkpoint `43a7e13bc30e` (2026-11-15), commands records 75–80:

- Copied Homebrew `help install` lists no `--no-sandbox` formula-install option. Its `Library/Homebrew/env_config.rb:129–135` **does** document `HOMEBREW_AVOID_NESTED_SANDBOXING`: skip only Homebrew's own sandbox when already sandboxed, outside the default prefix, and running as an unprivileged user. `sandbox.rb:107–129` enforces these conditions; `extend/os/mac/sandbox.rb:103–112` detects the outer sandbox through `sandbox_check`. No Homebrew source was changed. The copied checkout lacks `.git`, so its upstream revision could not be established; source hashes are captured in record 77, not claimed as upstream provenance.
- Minimal reproduction, with the existing profile unchanged: `sudo -u test env -i HOME=/opt/cercano-migration-20261115/home PATH=/usr/bin:/bin:/usr/sbin:/sbin sandbox-exec -f /opt/cercano-migration-20261115/safety.sb sandbox-exec -p '(version 1)(allow default)' /usr/bin/true`. Result: `sandbox-exec: sandbox_apply: Operation not permitted`, status **71**. No historical code is involved in that failure.
- Retried the historical install with `HOMEBREW_AVOID_NESTED_SANDBOXING=1` added **only to the fresh environment of the inner fixture brew invocation**, retaining the outer `sandbox-exec`, all previous environment/cleanup restrictions, and the original historical formula/hook. Brew verified the local archive and then exited **1**: `Refusing to skip the sandbox: ... you are in the privileged \`admin\` group; this is only supported for an unprivileged user.` The enclosing diagnostic script exits 0 because it prints individual statuses; that is **not** an install success. Exact argv/output/statuses are in record 78. This direct diagnostic invocation did not use the wrapper's process preflight; no keg/hook was reached, and final process inspection found no matching Cercano process.
- `sudo -u test id` confirms UID501, primary group `staff`, supplemental `admin` and others. Homebrew's privileged-group set is **admin, staff, root, wheel** (`sandbox.rb:20`), so merely removing admin membership would not suffice. No group/account/credential manipulation, alternate-identity execution, source monkeypatch, sandbox removal or isolation relaxation was attempted. The opt-in was not persisted in the wrapper or brew.env.
- Final profile SHA256 remains `5b4ab1be2e69df68181065aefb54397a402382cf24ae2e58ceb931723d12c966`, matching `safety-final.sb`. Installed-tap historical formula remains `4e55bcc37db70a6a8983963c3551a118d23a197b0a3ffd18afc2449462b1037f`, matching the saved fixture formula with its restart hook intact. All three synthetic state hashes remain as recorded above. No Cercano keg, command link or matching executable process was found. Snapshot `2f6bace3-6acf-4fcf-8e39-359c1efe3af0` remains listed; fixture and snapshot were not removed/restored. No host brew invocation, mutation of existing guest `/opt/homebrew` or `/Users/test`, publisher enablement, push or merge occurred.

**Disposition: still blocked before historical install.** A scoped setting exists, but Homebrew deliberately refuses it for the present fixture identity. Manual migration remains NOT RUN; proceeding from a failed historical install would invalidate its preconditions.

**Alternatives for review, not executed:** authorize a new disposable fixture using a dedicated genuinely unprivileged account (neither primary nor supplementary membership in any of the four rejected groups), an isolated custom prefix/HOME and at least the same validated outer containment, then retry the documented opt-in. Validate the replacement identity, ownership and sandbox denials before any hook execution; do not repurpose or modify `/Users/test`, this preserved fixture or its snapshot. Alternatively design and independently validate another containment mechanism supporting Homebrew's normal sandbox in a separate disposable environment. Neither alternative authorizes dropping the outer sandbox or bypassing Homebrew's identity guard here.

## Remaining gates

Historical native install/hook result, explicit manual migration, retained old-keg checks, exclusive command ownership, component/convenience brew tests, independent partial/missing-dependency recovery baselines, post-convenience snapshot recovery and A/B lag cases remain BLOCKED/NOT RUN. Installed-user applicability, real runtime/inference, human acceptance, notarization and clean-Mac signing acceptance remain pending. No publisher enabled; no push/merge performed.

## Retained fixture / cleanup inventory

- VM `08196962-608d-4fde-8284-49a5f020fea6` remains running; preboot snapshot above retained.
- Guest `/opt/cercano-migration-20261115` (~1.8GiB): copied brew checkout/portable Ruby and taps, local historical tap, downloaded archives, extracted signature-inspection binaries, cache/logs/tmp, synthetic HOME/state, scripts/base64 transfers and transport probe. No installed historical Cercano keg or fixture command links found. No old keg existed to remove or retain after install failed.
- Guest package invocations use explicit absolute fixture brew, fresh HOME, no auto-update/analytics/API installs/install cleanup and `HOMEBREW_NO_CLEANUP_FORMULAE=cercano,cercano-agent`; fixture persistent brew.env retains both exclusions. Policy was not extended to original prefix or another user.
- Worktree evidence is this directory; public archives remain local untracked files (excluded from checkpoint), with exact hashes. Scripts, formulas, logs, source excerpt and report are checkpointed. No spec.md/plan.md created. Nothing was deleted for cleanup; teardown requires separately chosen fixture-only removal and must not touch original user data or snapshot.
