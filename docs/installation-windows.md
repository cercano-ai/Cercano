# Installing Cercano on Windows

Windows x64 remains **experimental, unsigned, and not a supported release**.
Windows may show SmartScreen/unknown-publisher warnings. Package submission
is not code signing or a claim of full Windows support. No local llama/model
runtime is bundled. See the [artifact and testing guide](windows-artifact.md)
for limitations and the [README ZIP example](../README.md#windows-x64-experimental-unsigned)
for a checksum-verified alternative without package-manager installation.

## Chocolatey availability

All three **v0.27.0** packages were submitted successfully to the Chocolatey
Community Repository. The last known component status was **Pending automated
review** for `cercano-agent` and `cercano-cli`; the `cercano` convenience package
was waiting on its dependencies. Submission is **not approval**.

A read-only public recheck on **2026-11-15** returned HTTP 403 for all three
package pages, so current approval and default-source availability could not
be independently confirmed. Check the public pages before installing:

- [cercano](https://community.chocolatey.org/packages/cercano/0.27.0)
- [cercano-agent](https://community.chocolatey.org/packages/cercano-agent/0.27.0)
- [cercano-cli](https://community.chocolatey.org/packages/cercano-cli/0.27.0)

If moderation or dependency availability blocks installation, wait for approval
or use the verified ZIP. Do not bypass moderation or checksum validation.

## Install (once approved and available)

With Chocolatey already installed, run in an elevated PowerShell:

```powershell
choco install cercano -y
```

Then launch from a normal terminal:

```powershell
cercano-cli
```

The client starts the local agent when needed. The dependency-only `cercano`
package owns no binaries; its components supply the commands:

| Package | Command | Dependencies |
| --- | --- | --- |
| `cercano-agent` | `cercano` (agent) | none |
| `cercano-cli` | `cercano-cli` (terminal client) | none |
| `cercano` | none | both components, minimum version 0.27.0 |

For individual installs instead of the convenience package:

```powershell
choco install cercano-agent -y
choco install cercano-cli -y
```

Install only the component you need. A CLI-only installation can connect to a
remote agent with `cercano-cli --address HOST:PORT`; it does not pull in a local
agent. For local interactive use, both components are needed.

## Upgrade

Save work and close Cercano before upgrading. Quitting the terminal client can
leave the agent and workers running: check for those processes too. The package
guards refuse changes while their processes are running; they do not stop or
restart them for you. If an upgrade is refused, resolve the reported running
processes before retrying; do not force replacement.

```powershell
choco upgrade cercano -y
```

The convenience package uses **minimum dependency versions**, not exact locks
or an atomic release set. When it upgrades, Chocolatey resolves dependencies
from available source versions; that does not guarantee identical component
versions. If the convenience package is already current, do not assume the
command refreshed independently updated components. To explicitly upgrade both
(or only the component you installed):

```powershell
choco upgrade cercano-agent cercano-cli -y
```

Do not use `--ignore-dependencies` for the convenience package. Moderation or
repository lag can leave dependencies unavailable; wait rather than bypassing
those checks. After a successful upgrade, launch `cercano-cli` again and check:

```powershell
cercano --version
cercano-cli --version
```

Keep configuration and conversations intact during upgrades. If a prior ZIP,
source build, or other installation is also on PATH, use `Get-Command cercano,cercano-cli -All`
to inspect command resolution; upgrading Chocolatey packages does not replace
those other copies. No automatic migration from other installation methods is
prescribed here.
