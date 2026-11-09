> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Disposable Windows packaging runner

## Handoff status — 2026-11-09 (no runner access)

Current runner readiness is **unknown**; this maintenance did not query or change any VM, guest or host configuration. The historical authorized disposable clone is `{SANITIZED_VM_2}` only; the original guest must not be touched. The bootstrap-blocked report below is historical: later authorized process-only execution-policy handling installed Chocolatey 2.7.5 and enabled the synthetic probes recorded in `windows-phase1-handoff.md` and `native-go-next-results.md`. Those ARM64 probes are not x64 shipping-ZIP or product acceptance. Do not reuse commands below as current authorization or rerun the provider-blocked concurrent-launch probe. See the handoff for residuals, unresolved validation and the branch-push privacy blocker.

## Historical result — full clone running; Chocolatey bootstrap blocked (2026-11-06)

Resumed after the user independently shut down the original. Read `AGENTS.md`, the approved Windows amendment in `spec.md`, `plan.md`, and this report. `prlctl list --all` verified source `{SANITIZED_VM_1}` **stopped** before cloning and again after guest provisioning. No source start, stop, configuration, guest command, or UUID regeneration was performed. Other existing VMs were not accessed or changed.

### Clone and access evidence

Parallels CLI **27.1.0 (58756)** supports `--force-deep-copy`; the following direct-argv full clone succeeded (exit 0), without `--linked` or `--regenerate-src-uuid`:

```text
prlctl clone {SANITIZED_VM_1} --name cercano-windows-packaging-20261106-1117-full --force-deep-copy
prlctl list --all
prlctl start {SANITIZED_VM_2}
prlctl exec {SANITIZED_VM_2} C:\Windows\System32\whoami.exe
```

- Exact clone UUID: **`{SANITIZED_VM_2}`**.
- Name: `cercano-windows-packaging-20261106-1117-full` (name is an identifier, not a timestamp assertion).
- Only this clone was started; final inventory reports it **running**, original **stopped**.
- First guest probe failed while booting with `Unable to open new session ...`; retry after 20 seconds succeeded (exit 0).
- Direct guest identity: **`nt authority\system`**. No credentials, login, license purchase, or account-policy changes were used.

### Sanitized guest inventory

Bounded CIM/registry/command-discovery queries, without user-file inspection:

| Fact | Observed |
| --- | --- |
| OS | Microsoft Windows 11 Pro |
| Version / build | `10.0.26300` / `26300` |
| OS / process architecture | ARM 64-bit Processor / ARM64 |
| PowerShell | `5.1.26100.9711` |
| .NET Framework v4 Full release | `534296` |
| Command discovery | `choco.exe`, `git.exe`, `go.exe`, `python.exe`, `dotnet.exe` absent from this execution environment's PATH |
| Fixed Chocolatey executable | `C:\ProgramData\chocolatey\bin\choco.exe` absent before and after attempt |

Absence from PATH is not an exhaustive software inventory. SYSTEM access is usable for isolated machine-tool provisioning, not evidence of standard-user, interactive, cross-account, or Cercano lifecycle acceptance.

### Official Chocolatey bootstrap attempt and blocker

Created only the new isolated staging directory `C:\CercanoRunner\provision-20261106-1117`; saved the downloaded bootstrap as `install-chocolatey.ps1` and provenance as `provenance.json` there. The official bootstrap also staged its own package under `C:\Windows\Temp\chocolatey\chocoInstall`. No inherited user profiles/files or credentials were inspected.

- Official installation reference: <https://chocolatey.org/install>.
- Requested and final bootstrap URL: **`https://community.chocolatey.org/install.ps1`**, downloaded in the clone over HTTPS using `Invoke-WebRequest -UseBasicParsing`, with no certificate-verification override.
- Retrieval UTC: **`2026-11-06T19:03:01.2385378Z`**.
- Saved bootstrap SHA-256: **`44E045ED5350758616D664C5AF631E7F2CD10165F5BF2BD82CBF3A0BB8F63462`**.
- Official bootstrap selected/downloaded **Chocolatey 2.7.5** from `https://community.chocolatey.org/api/v2/package/chocolatey/2.7.5`.
- Downloaded package SHA-256: **`F86F99A18D236C2F99E6F67A20BE7ADC65C6C7756BF6CF3B43759E7EC0AA5839`** (provenance evidence, not independent publisher authentication).
- **Installed Chocolatey version: none.** Bootstrap invocation exited 1 when loading its packaged `tools\chocolateyInstall.ps1`:

```text
File C:\WINDOWS\TEMP\chocolatey\chocoInstall\tools\chocolateyInstall.ps1 cannot be loaded because running scripts is disabled on this system.
CategoryInfo: SecurityError ... PSSecurityException
FullyQualifiedErrorId: UnauthorizedAccess
```

Read-only `Get-ExecutionPolicy -List` reported `Undefined` at MachinePolicy, UserPolicy, Process, CurrentUser, and LocalMachine. No execution-policy changes or bypass switches were applied. This is an exact guest script-execution permission/policy blocker, not a missing authorization for routine package installation. Provisioning stopped rather than bypassing that restriction; a policy-permitted installation context is needed before Chocolatey/native fixtures are ready. No user login or paid-license blocker was encountered, but neither interactive licensing nor standard-user access was qualified.

### Verified command transport recipe

Use direct program argv, never host-shell compound commands. `whoami.exe` above verifies identity in the clone. For bounded PowerShell operations, the successful transport used an absolute executable and a UTF-16LE Base64 payload, avoiding inline quoting:

```python
import base64, subprocess
clone = '{SANITIZED_VM_2}'
# Example harmless inventory payload; no shell and no policy override.
script = '[Security.Principal.WindowsIdentity]::GetCurrent().Name'
subprocess.run([
    'prlctl', 'exec', clone,
    r'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe',
    '-NoProfile', '-NonInteractive', '-EncodedCommand',
    base64.b64encode(script.encode('utf-16le')).decode()
], check=True)
```

The bootstrap transport used this same recipe to retrieve the official script, save/hash it, and invoke its content with `& ([scriptblock]::Create([IO.File]::ReadAllText($path)))`; its nested packaged-script invocation was blocked as recorded above. Do not repeat that bootstrap or substitute a bypass as a readiness probe. For future fixtures prefer absolute `-File` scripts once script execution is legitimately available; that route is not yet verified.

**Native runner readiness: partial / blocked.** Full clone and SYSTEM guest command access are established; Chocolatey installation and all native package-safety/acceptance probes remain blocked/not run. No additional tooling, fixture package, production Cercano, or package-safety probe was installed/run. No host package/security/configuration changes, credential access, account-policy changes, plan edits, git operations, or checkpoints occurred. The historical failure below is retained as evidence of the earlier suspended-source attempt, not current source state.

## Historical result — blocked while source suspended (2026-11-06)

No new runner was provisioned. Parallels refuses the supported full-clone operation while the authorized source VM is suspended. The source was not resumed, stopped, reconfigured, or otherwise deliberately changed. A second inventory confirms it remains suspended. No guest commands, package installs, credential access, personal-file inspection, or Cercano installation occurred.

Read `AGENTS.md`, this effort's `spec.md`, and `plan.md`. This report does not change the plan or establish native packaging acceptance.

## Host tool and sanitized evidence

`prlctl help`, `prlctl clone --help`, and `prlctl exec --help` report Parallels CLI **27.1.0 (58756)**. Clone help documents normal cloning with `--name`, optional `--force-deep-copy`, and separate `--linked` and `--regenerate-src-uuid` options. Neither linked cloning nor source UUID regeneration was requested.

Initial `prlctl list --all` identified the authorized source:

- Source UUID: `{SANITIZED_VM_1}`
- Source name: `Windows 11`
- Source state: `suspended`

Two other VMs were listed (one running, one suspended); neither was accessed or modified. Their existence does not authorize reuse.

Attempted full clone (no shell interpretation):

```text
prlctl clone {SANITIZED_VM_1} --name cercano-windows-packaging-20261106-1045 --force-deep-copy
```

Result: exit **255**, with:

```text
Failed to clone the VM: Unable to complete the operation. This operation cannot be completed because "Windows 11" is in the "suspended" state.
```

A subsequent `prlctl list --all` returned the same three UUIDs/states and no new clone. Consequently:

| Requested detail | Observed result |
| --- | --- |
| Clone name | Attempted `cercano-windows-packaging-20261106-1045`; not created |
| Exact clone UUID | **None assigned/listed** |
| Guest command access | Not established |
| Guest OS version/build | Not queried; source display name is not OS evidence |
| Guest architecture | Not queried |
| Chocolatey presence/version | Not queried |
| Chocolatey download provenance/version | No download or install attempted |
| Native fixture readiness | **Blocked** |

## Blocker and boundary

The supported clone attempt cannot proceed from this suspended source state. Resuming/stopping the source to enable cloning would violate the explicit requirement that it remain untouched and suspended. No bypass, manual VM-file copy, alternate existing VM reuse, UI interaction, or license/credential operation was attempted. Provisioning needs an independently authorized disposable runner or an independently prepared cloneable source; this run cannot create one within the present constraints.

## Follow-up: fresh installation route

A separate read-only investigation checked Parallels' supported alternatives. [KB 125375](https://kb.parallels.com/125375) documents Control Center → **+ → Get Windows 11 from Microsoft → Continue → Install Windows**, with automatic download/install and a Microsoft EULA completion step. This can create a fresh disposable guest without resuming the original. User interaction is still needed for the wizard and any license/account prompts; no installation was initiated.

[Microsoft's ARM64 ISO page](https://www.microsoft.com/en-us/software-download/windows11arm64) is an alternative official source with published checksum verification. No Windows ISO was found in the bounded known Parallels directories, excluding VM/disk bundles; no ISO was downloaded. Installed CLI `create --help` exposes VM configuration creation but no Windows download/unattended installation switch. [KB 114118](https://kb.parallels.com/114118) requires shutdown before copying a VM, so manually copying suspended state is not treated as a supported workaround.

The user authorized disposable provisioning and guest test tooling. The remaining blocker is interactive provisioning, not a request for repeated authorization of routine implementation. The original remains suspended.

## Guest invocation recipe for a future authorized clone

The following is a **help-derived recipe, not guest-verified**. Use only an exact newly authorized clone UUID, never the source UUID. Invoke `prlctl` as a direct argv program; do not wrap guest commands in host shell strings or assume complex argument quoting survives. Prefer one absolute guest executable with no arguments for the first access probe, for example:

```text
prlctl exec <CLONE-UUID> C:\Windows\System32\hostname.exe
```

For later multi-step work, prefer an absolute path to a purpose-built guest test script (or the absolute PowerShell executable with `-NoProfile -NonInteractive -File` and a simple absolute script path), first validating argument delivery using harmless synthetic inputs. Do not use inline `-Command` strings with nested quoting as the primary harness. Do not provide passwords on the command line, inspect inherited credentials, or read inherited user profiles. If access needs credentials/UI/license input, stop and report the blocker.

Once access exists, gather only bounded OS/build/architecture and package-tool version facts. If Chocolatey is absent, use its official documented installation source in that clone only, and record the documentation URL, actual download URL, retrieval time, script SHA-256, and installed version. None of these installation steps was reached here. No host packages/configuration/security settings were changed, no plan was edited, and no git plumbing, commit, or push was performed.
