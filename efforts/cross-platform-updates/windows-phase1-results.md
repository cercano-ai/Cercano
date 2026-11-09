> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Windows Phase 1 — synthetic transaction checkpoint

2026-11-06. This is a bounded safety-evidence unit, **not completion of the native acceptance matrix**. No production packaging implementation or release operation is included. Spec and plan are unchanged.

## Scope and setup

Only disposable clone `{SANITIZED_VM_2}` was targeted. Original `{SANITIZED_VM_1}` remained stopped in recorded VM listings, including the final check. Clone remains running for evidence retention. Commands used absolute guest PowerShell/Chocolatey paths, not shell-dependent quoting. Execution identity was SYSTEM; this does **not** establish standard-user or alternate-admin acceptance.

The previous bootstrap failure left downloaded/extracted Chocolatey under `C:\Windows\Temp\chocolatey\chocoInstall`, not a working destination installation. `provision.ps1` inspected those locations and refused an existing destination/binary rather than deleting partial installation state blindly. It reran the already downloaded official `https://community.chocolatey.org/install.ps1` with process-only `-ExecutionPolicy Bypass`. Official installer completed and installed Chocolatey **2.7.5**. See `windows-provision-output.txt` (also includes later fixture diagnostic output; not a pristine provisioning-only stream).

Observed SHA-256:

- Official installer script: `44E045ED5350758616D664C5AF631E7F2CD10165F5BF2BD82CBF3A0BB8F63462`.
- Downloaded Chocolatey 2.7.5 package: `F86F99A18D236C2F99E6F67A20BE7ADC65C6C7756BF6CF3B43759E7EC0AA5839`.
- Installed `C:\ProgramData\chocolatey\bin\choco.exe`: `E8DB87C40BAD54ECFDFDE1F7097090C457A9DB1E533F88AD56E274AD0DA9FA29`.
- Synthetic running A payload (transactions-02): `FCE500C9D66B33C8E83FF0E90C5588C385908490FCE10167ABD367BA4DB9EBDB`.

During scripts, Process policy was Bypass and other scopes Undefined. A fresh direct PowerShell check without bypass reported **all five scopes Undefined**, Chocolatey 2.7.5 and only `chocolatey|2.7.5` installed (`windows-policy-final-output.txt`; also rechecked at checkpoint). No persistent policy change was needed.

Guest: Windows 11 Pro build 26300, ARM64; Windows PowerShell 5.1.26100.9711. `IsWow64Process2` on live Chocolatey/compiler processes reported process machine `0x0000`, native `0xAA64` (ARM64). PE headers were `0x014C` for managed Chocolatey/AnyCPU fixture and `0x8664` for compiler; **PE header alone is not emulation evidence**. Measured processes reported native ARM64, not x86/x64 emulation. This is not a native x64 Windows matrix, nor a test of compiled Go production executable locking. See architecture/final output and helper scripts.

## Reusable harness and exact operation shapes

`release/chocolatey/tests/guest.py` hardcodes the clone UUID, uploads only a supplied script to `C:\CercanoRunner\phase1\entry.ps1`, then directly invokes:

```text
prlctl exec {SANITIZED_VM_2} C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File C:\CercanoRunner\phase1\entry.ps1 [script arguments]
```

The historical transaction set was `transactions-02`, extensions `transactions-03`, inconclusive barrier `transactions-04`, closed smoke `transactions-05`. The current consolidated harness is parameterized, requires a fresh run directory and refuses existing synthetic package/shim ownership. Do not rerun provisioning on the installed clone. Do not rerun barrier as if it were a passing regression.

```text
python3 release/chocolatey/tests/guest.py release/chocolatey/tests/transactions.ps1 efforts/cross-platform-updates/windows-consolidated-output.txt -RunName transactions-06
python3 release/chocolatey/tests/guest.py release/chocolatey/tests/collect.ps1 efforts/cross-platform-updates/windows-consolidated-evidence.json -RunName transactions-06
python3 release/chocolatey/tests/assert_evidence.py
```

Use a fresh `-RunName` for future runs; `-CaseSet closed` gives a smaller smoke test. Historical standalone `extensions.ps1`/`barrier.ps1` are retained to explain captured evidence, not production templates. Collection reads only fixture-owned files. Historical package hashes are in companion transaction stdout (local-feed `.nupkg` files; no consolidated stdout file is stored), individual command argument arrays in collected `commands.json`, and before/after/cleaned package/shim hashes in each `*-files.json`. The collector excludes `transcript.txt`: none of the stored JSON collections contains a transcript. In particular, `windows-consolidated-output.txt` in the historical command example is absent from this checkpoint. No consolidated PID-survival or fresh-launch observation can be reconstructed from the stored collection. Current helpers are not a byte-for-byte attestation of historical guest scripts. The commands above document operation shapes only; they were not rerun during review finalization.

For each `cercano-phase1-CASE`, the absolute manager is `C:\ProgramData\chocolatey\bin\choco.exe`. Exact command pattern (expanded per-case arguments are preserved in evidence):

```text
choco.exe pack RUN\CASE\VERSION\cercano-phase1-CASE.nuspec --outputdirectory=RUN\feed --limit-output
choco.exe install cercano-phase1-CASE --version=1.0.0 --source=RUN\feed -y --no-progress --limit-output
choco.exe upgrade cercano-phase1-CASE --version=2.0.0 --source=RUN\feed -y --no-progress --limit-output --execution-timeout=60
choco.exe uninstall cercano-phase1-CASE -y --no-progress --limit-output --execution-timeout=60
```

`RUN` is the absolute guest run directory. Only synthetic local-feed packages were installed. Locks are fixture-owned `FileStream`s, released with Dispose. Running fixture has a 120-second maximum and a graceful stop-file protocol; no forced termination. Synthetic config/conversation/fake-credential sentinels live outside package ownership under the run directory, not inherited profiles. Their hashes are unchanged. A path containing spaces/Unicode for these sentinels does not establish installed executable-path coverage.

## Observed transaction boundary

The table below describes **exact offline assertions** in `assert_evidence.py`, across phase1, extensions, smoke and consolidated captures. Metadata B means the manager lists 2.0.0, not that expected B bytes or safe activation have been proven. A changed executable hash means only `after != before`; no stored expected-B artifact hash is compared. Shim presence/hash does not prove command functionality.

| Probe | Asserted operation exit | Asserted after-state |
|---|---:|---|
| Closed A→B | 0 | Metadata 2.0.0; executable hash differs from A; shim present. |
| BeforeModify throw | -1 | Metadata 2.0.0; executable hash differs from A; shim present despite failure. |
| BeforeModify nonzero | 19 | Metadata 2.0.0; executable hash differs from A; shim present despite failure. |
| Running fixture | 0 | Metadata 2.0.0; executable hash differs from A; shim present. No PID-survival assertion. |
| Stopped locked resource | 1 | Metadata 1.0.0; every before-snapshot path/hash except shim preserved; shim present (hash not compared). |
| Locked resource + BeforeModify throw | -1 | Same preservation/presence checks as locked resource; metadata 1.0.0. |
| Locked shim | 1 | Metadata 2.0.0; executable hash differs from A; shim has original hash. |
| Uninstall script throw | -1 | Metadata 1.0.0; before-snapshot non-shim hashes preserved; shim absent; hook contains `uninstall:shim=False`. |
| New install script throw | -1 | Metadata 1.0.0; before-snapshot non-shim hashes preserved; shim present (hash not compared). |

For every checked case, the checker also asserts cleanup exit 0, absence of the executable and shim from cleaned snapshots, absence of package metadata, and byte-identical synthetic data hash CSVs before/after the run. It does not assert that all resources or backup artifacts are gone. Equality of synthetic sentinels is not a test of real user data.

**Historical observations, not offline assertions:** `windows-transactions-output.txt:83-88` records owned PID 9520 alive before/after and a direct fresh payload printing 2.0.0. Closed direct-payload output is at line 26 (also `windows-harness-smoke-output.txt:26`). These are not public-shim launches, do not prove PID identity beyond the helper's observation, and do not extend to the consolidated run. Retry command records and snapshots are retained in JSON, and historical stdout records zero retry exits; `assert_evidence.py` does not check retries or their resulting bytes. No new process/launch claims are inferred here.

**Explicit unasserted gaps:** expected-B executable identity; resource-B identity or change in B-metadata cases; fresh public-shim version/launch; PID survival/ownership; retry exit/metadata/payload/resource correctness; shim hash preservation except `shimlocked`; actionable failure-message quality; complete resource/backup cleanup; atomic intermediate state. No stored assertions establish running-process safety, real user-data preservation or full native acceptance.

`windows-phase1-evidence.json`, `windows-extensions-evidence.json`, `windows-smoke-evidence.json`, `windows-consolidated-evidence.json` and available companion outputs retain logs, hook chronology, inventories, metadata and synthetic data checks. Resource-lock after-snapshots preserve recorded original non-shim paths/hashes; they **do not demonstrate persistent partial payload deletion**. Source-possible intermediate deletion must not be represented as an observed final-state loss. Historical backup/bad-package artifacts in the install-failure case are conditional-recovery observations, not atomicity proof.

Barrier relaunch extension: before-modify launched owned A and recorded ready PID 6744, operation returned 0 and metadata became B, but ownership/path validation failed. Later that PID no longer identified the fixture; no process was killed. Manager uninstall cleaned this fixture. **Inconclusive:** no assertion about survival, race exclusion or safety is supported. The standalone helper remains a failed experiment, not a regression pass.

## Safety feasibility and amendment scope

The captured metadata/hash transitions and uninstall hook/shim observation support the source audit's **BeforeModify non-veto and uninstall shim-before-script boundaries**. Refusal implemented only by those hooks cannot be relied on to prevent mutation or infer activation from an exit code. Documentation asking users to close the app does not supply the missing enforcement.

The amendment requires **actionable failure for running/locked-file conditions, no forced process killing, no user-data loss, and no false activation**. It does **not** require atomic preservation of every shim/payload on every error. Consequently shim disappearance or intermediate file mutation alone is not proof that every Chocolatey portable approach violates the amendment. This evidence rejects reliance on hook veto, not all portable designs. The managed running fixture did not produce a refusal; production running/locked-file safety remains unproven. Actionability and activation truth require separate evidence, not just manager exit codes or metadata.

A Chocolatey wrapper around Windows Installer (MSI) component packages is **one untested candidate**, not a minimum necessary solution or an approved architecture. No MSI transaction was tested here. Other native-manager-compatible portable approaches are not excluded by this checkpoint; any proposal must demonstrate the amendment's actual failure/no-kill/data/activation requirements, including launch races and manager-owned recovery where needed. No production packaging implementation is authorized by these results.

## Teardown and remaining gaps

Fixture processes exit cooperatively or by their bounded lifetime; file locks are disposed. No existing/unowned process was forcibly terminated. Only local synthetic packages were installed/uninstalled. Final package listing contains only Chocolatey 2.7.5. Clone retains tooling and fixture feeds/evidence/sentinels intentionally for inspection; bootstrap temp extraction is retained, not silently purged. Original VM was not started or targeted. No production install, service, signing, publication, persistent policy change, spec/plan change, git/commit, push or merge.

Remaining: native x64 and production executable/resource closure; successful barrier/relaunch/inaccessible-process coverage; real agent/workers and reconnect coordination; stopped locked executable and uninstall-lock matrix; interrupted/power-loss/permission/disk failure; alternate-user/data boundaries; standalone/convenience/partial component lifecycle; installed paths with spaces/Unicode; signed releases/runtime delivery. Current tests neither load a real model nor establish production artifact or inference compatibility.

## Review finalization

This review used stored files only; no guest scripts were rerun. Offline assertions pass for all 19 captured cases, and both Python helpers pass syntax compilation. See `windows-phase1-checkpoint.md` for finding resolutions, exact scoped file allowlist, hashes and raw-evidence privacy risks. No binary archives are included in that allowlist.
